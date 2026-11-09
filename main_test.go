package main

import (
	"encoding/json"
	"errors"
	"syscall"
	"testing"
)

func TestConfigShowConsoleDefaultsToTrue(t *testing.T) {
	var cfg config
	if err := json.Unmarshal([]byte(`{"auto_start":false}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.ShowConsole {
		t.Fatal("show_console should default to true when omitted")
	}
}

func TestConfigShowConsoleCanBeDisabled(t *testing.T) {
	var cfg config
	if err := json.Unmarshal([]byte(`{"show_console":false}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.ShowConsole {
		t.Fatal("show_console should respect an explicit false value")
	}
}

func TestIsTraceConsoleInvocation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "trace console", args: []string{"AltCmd.exe", traceConsoleArg, "AltCmd.log"}, want: true},
		{name: "normal startup", args: []string{"AltCmd.exe"}, want: false},
		{name: "missing log path", args: []string{"AltCmd.exe", traceConsoleArg}, want: false},
		{name: "other mode", args: []string{"AltCmd.exe", "--other", "AltCmd.log"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTraceConsoleInvocation(tt.args); got != tt.want {
				t.Fatalf("isTraceConsoleInvocation(%q) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestAltKeyDownCapturesMappedKeyImmediately(t *testing.T) {
	a := &app{
		altDownPending: true,
		altDownVK:      vkLMenu,
	}
	a.altComboTable['Z'] = 7

	if !a.handleHookKeyDown('Z') {
		t.Fatal("Alt+Z key-down should be swallowed")
	}
	if a.pendingByKey['Z'] != 7 {
		t.Fatalf("pending key ID = %d, want 7", a.pendingByKey['Z'])
	}
}

func TestMappedKeyRepeatIsSwallowedUntilKeyUp(t *testing.T) {
	a := &app{
		altDownPending: true,
	}
	a.altComboTable['Z'] = 9

	if !a.handleHookKeyDown('Z') || !a.handleHookKeyDown('Z') {
		t.Fatal("mapped key and its repeats should be swallowed")
	}
	if !a.handleHookKeyUp('Z') {
		t.Fatal("mapped key-up should be swallowed")
	}
	if a.pendingByKey['Z'] != 0 {
		t.Fatal("mapped key should no longer be pending after key-up")
	}
}

func TestAltComboIsNotDroppedAfterLongAltHold(t *testing.T) {
	a := &app{
		altDownPending: true,
		altDownVK:      vkLMenu,
	}
	a.altComboTable['Z'] = 11

	if !a.handleHookKeyDown('Z') {
		t.Fatal("Alt+Z should remain captured while Alt is still held")
	}
	if a.pendingByKey['Z'] != 11 {
		t.Fatalf("pending key ID = %d, want 11", a.pendingByKey['Z'])
	}
}

func TestNewAltPressClearsAbandonedChord(t *testing.T) {
	a := &app{
		pendingByKey: [256]int{'Z': 9},
	}

	if !a.handleHookKeyDown(vkLMenu) {
		t.Fatal("Alt key-down should be swallowed")
	}
	if a.pendingByKey['Z'] != 0 {
		t.Fatal("a new Alt press should not inherit an abandoned chord")
	}
}

func TestAltKeyUpDoesNotLeakMappedKeyRelease(t *testing.T) {
	a := &app{
		altDownPending: true,
		pendingByKey:   [256]int{'Z': 9},
	}

	if !a.handleHookKeyUp(vkLMenu) {
		t.Fatal("Alt key-up should be swallowed")
	}
	if !a.handleHookKeyUp('Z') {
		t.Fatal("mapped key-up should remain swallowed after Alt is released")
	}
}

func TestInspectHotkeyConflictsMarksWindowsAndDuplicateConflicts(t *testing.T) {
	hotkeys := []hotkeyConfig{
		{Combo: "Alt+Z", Action: hotkeyActionUndo, Enabled: true},
		{Combo: "Alt+S", Action: hotkeyActionSave, Enabled: true},
		{Combo: "Alt+Z", Action: hotkeyActionCopy, Enabled: true},
		{Combo: "Alt+X", Action: hotkeyActionCut, Enabled: false},
	}
	conflicts, messages := inspectHotkeyConflicts(hotkeys, func(_ int, _ uint, key uint) (bool, error) {
		return key == 'S', nil
	})

	if conflicts[0] != "配置重复" || conflicts[2] != "配置重复" {
		t.Fatalf("duplicate conflict statuses = %q, %q", conflicts[0], conflicts[2])
	}
	if conflicts[1] != "系统热键冲突" {
		t.Fatalf("Alt+S status = %q, want system conflict", conflicts[1])
	}
	if conflicts[3] != "未启用" {
		t.Fatalf("disabled hotkey status = %q, want disabled", conflicts[3])
	}
	if len(messages) != 2 {
		t.Fatalf("got %d conflict messages, want 2", len(messages))
	}
}

func TestInspectHotkeyConflictsReportsProbeErrors(t *testing.T) {
	conflicts, messages := inspectHotkeyConflicts(
		[]hotkeyConfig{{Combo: "Alt+C", Action: hotkeyActionCopy, Enabled: true}},
		func(_ int, _ uint, _ uint) (bool, error) {
			return false, errors.New("access denied")
		},
	)
	if conflicts[0] != "无法探测" {
		t.Fatalf("status = %q, want unavailable", conflicts[0])
	}
	if len(messages) != 1 || messages[0] != "Alt+C：暂时无法确认是否冲突" {
		t.Fatalf("messages = %#v, want a friendly unavailable message", messages)
	}
}

func TestHotkeyAlreadyRegisteredErrorIsRecognized(t *testing.T) {
	if !isHotkeyAlreadyRegistered(syscall.Errno(1409)) {
		t.Fatal("Windows hotkey-already-registered error should be recognized")
	}
}

func TestAddGameProcessEnablesGameModeAndDeduplicates(t *testing.T) {
	cfg := config{
		GameProcesses:   []string{"existinggame.exe"},
		GameModeEnabled: false,
	}

	added, process := addGameProcess(&cfg, `C:\Games\SampleGame\Game.exe`)
	if !added || process != "game.exe" {
		t.Fatalf("addGameProcess() = (%v, %q), want (true, %q)", added, process, "game.exe")
	}
	if !cfg.GameModeEnabled {
		t.Fatal("adding a game should enable game mode")
	}
	if len(cfg.GameProcesses) != 2 || cfg.GameProcesses[1] != "game.exe" {
		t.Fatalf("game process list = %#v", cfg.GameProcesses)
	}

	added, process = addGameProcess(&cfg, `D:\AnotherInstall\GAME.EXE`)
	if added || process != "game.exe" || len(cfg.GameProcesses) != 2 {
		t.Fatalf("duplicate add = (%v, %q), list %#v", added, process, cfg.GameProcesses)
	}
}

func TestAddGameProcessRejectsNonExecutable(t *testing.T) {
	cfg := config{}
	if added, _ := addGameProcess(&cfg, `C:\Games\readme.txt`); added {
		t.Fatal("non-executable file should not be added")
	}
}
