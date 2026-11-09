package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const (
	traceConsoleArg  = "--trace-console"
	traceTailMaxSize = 64 * 1024
)

var (
	consoleKernel32      = windows.NewLazySystemDLL("kernel32.dll")
	procGetConsoleWindow = consoleKernel32.NewProc("GetConsoleWindow")
	procFreeConsole      = consoleKernel32.NewProc("FreeConsole")
	procAllocConsole     = consoleKernel32.NewProc("AllocConsole")
	consoleLogFile       *os.File
	traceConsoleCmd      *exec.Cmd
	traceConsoleDone     chan error
	consoleDetached      bool
)

func setConsoleVisible(visible bool) error {
	if err := initializeTraceLogging(); err != nil {
		return err
	}
	if err := detachApplicationConsole(); err != nil {
		return err
	}

	if visible {
		return startTraceConsole()
	}
	return stopTraceConsole()
}

func initializeTraceLogging() error {
	if consoleLogFile != nil {
		return nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("get user config directory failed: %w", err)
	}
	logDir := filepath.Join(configDir, "AltCmd")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return fmt.Errorf("create trace log directory failed: %w", err)
	}
	output, err := os.OpenFile(filepath.Join(logDir, "AltCmd.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open trace log failed: %w", err)
	}
	consoleLogFile = output
	log.SetOutput(output)
	return nil
}

func detachApplicationConsole() error {
	if consoleDetached {
		return nil
	}
	consoleWindow, _, _ := procGetConsoleWindow.Call()
	if consoleWindow != 0 {
		result, _, callErr := procFreeConsole.Call()
		if result == 0 {
			return fmt.Errorf("detach application from console failed: %w", callErr)
		}
	}
	consoleDetached = true
	return nil
}

func startTraceConsole() error {
	if traceConsoleCmd != nil {
		select {
		case <-traceConsoleDone:
			traceConsoleCmd = nil
			traceConsoleDone = nil
		default:
			return nil
		}
	}

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get application executable failed: %w", err)
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("get user config directory failed: %w", err)
	}
	logPath := filepath.Join(configDir, "AltCmd", "AltCmd.log")

	cmd := exec.Command(executable, traceConsoleArg, logPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start trace console failed: %w", err)
	}
	done := make(chan error, 1)
	traceConsoleCmd = cmd
	traceConsoleDone = done
	go func() {
		done <- cmd.Wait()
	}()
	return nil
}

func stopTraceConsole() error {
	if traceConsoleCmd == nil {
		return nil
	}
	select {
	case <-traceConsoleDone:
		traceConsoleCmd = nil
		traceConsoleDone = nil
		return nil
	default:
	}

	if err := traceConsoleCmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("close trace console failed: %w", err)
	}
	<-traceConsoleDone
	traceConsoleCmd = nil
	traceConsoleDone = nil
	return nil
}

func runTraceConsole(logPath string) error {
	consoleWindow, _, _ := procGetConsoleWindow.Call()
	if consoleWindow == 0 {
		result, _, callErr := procAllocConsole.Call()
		if result == 0 {
			return fmt.Errorf("allocate trace console failed: %w", callErr)
		}
	}

	output, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open trace console output failed: %w", err)
	}
	defer output.Close()

	logFile, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("open trace log failed: %w", err)
	}
	defer logFile.Close()

	info, err := logFile.Stat()
	if err != nil {
		return fmt.Errorf("read trace log information failed: %w", err)
	}
	offset := info.Size() - traceTailMaxSize
	if offset < 0 {
		offset = 0
	}
	if _, err := logFile.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("seek trace log failed: %w", err)
	}
	reader := bufio.NewReader(logFile)
	if offset > 0 {
		if _, err := reader.ReadString('\n'); err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read trace log failed: %w", err)
		}
	}
	if _, err := io.WriteString(output, "AltCmd trace console; closing this window will not stop AltCmd.\r\n"); err != nil {
		return fmt.Errorf("write trace console header failed: %w", err)
	}

	for {
		if _, err := io.Copy(output, reader); err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("write trace log to console failed: %w", err)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func isTraceConsoleInvocation(args []string) bool {
	return len(args) == 3 && strings.EqualFold(args[1], traceConsoleArg)
}
