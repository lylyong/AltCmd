package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	applicationVersion = "1.1.0"

	wmHotkey  = 0x0312
	wmCommand = 0x0111
	wmDestroy = 0x0002
	wmClose   = 0x0010
	wmApp     = 0x8000
	wmExecute = wmApp + 2

	wmCopy  = 0x0301
	wmPaste = 0x0302

	wmRButtonUp     = 0x0205
	wmLButtonUp     = 0x0202
	wmContextMenu   = 0x007B
	wmLButtonDblClk = 0x0203
	wmNull          = 0x0000
	wmKeyDown       = 0x0100
	wmKeyUp         = 0x0101
	wmSysKeyDown    = 0x0104
	wmSysKeyUp      = 0x0105

	modAlt     = 0x0001
	modControl = 0x0002
	modShift   = 0x0004
	modWin     = 0x0008

	inputKeyboard  = 1
	imageIcon      = 1
	keyeventfKeyup = 0x0002
	whKeyboardLL   = 13
	hcAction       = 0
	llkhfInjected  = 0x00000010
	lrDefaultSize  = 0x0040
	lrLoadFromFile = 0x0010
	gwlExstyle     = -20

	vkControl  = 0x11
	vkLControl = 0xA2
	vkRControl = 0xA3
	vkMenu     = 0x12
	vkShift    = 0x10
	vkLShift   = 0xA0
	vkRShift   = 0xA1
	vkLWin     = 0x5B
	vkRWin     = 0x5C
	vkLMenu    = 0xA4
	vkRMenu    = 0xA5
	vkEscape   = 0x1B
	vkF4       = 0x73
	vkOem3     = 0xC0

	sendInputMaxAttempts = 3

	processQueryLimited = 0x1000
	smtoAbortIfHung     = 0x0002
	wsExToolWindow      = 0x00000080

	mfString    = 0x0000
	mfSeparator = 0x0800
	mfChecked   = 0x0008

	tpmLeftAlign   = 0x0000
	tpmBottomAlign = 0x0020

	nimAdd        = 0x00000000
	nimDelete     = 0x00000002
	nimSetVersion = 0x00000004

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	notifyIconVersion4 = 4
	ninSelect          = 0x0400
	ninKeySelect       = 0x0401

	idiApplication = 32512

	idMenuToggleGameMode = 1001
	idMenuToggleStartup  = 1002
	idMenuReloadConfig   = 1003
	idMenuExit           = 1004
	idMenuSettings       = 1005
	idMenuToggleConsole  = 1006
	idMenuAbout          = 1007

	trayUID = 1

	wmTrayIcon = wmApp + 1

	hotkeyActionCopy         = "copy"
	hotkeyActionPaste        = "paste"
	hotkeyActionCut          = "cut"
	hotkeyActionUndo         = "undo"
	hotkeyActionSave         = "save"
	hotkeyActionSelectAll    = "select_all"
	hotkeyActionNewTab       = "new_tab"
	hotkeyActionCloseTab     = "close_tab"
	hotkeyActionCloseWindow  = "close_window"
	hotkeyActionSwitchWindow = "switch_window"
	hotkeyActionFind         = "find"

	currentConfigVersion = 1
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procSendInput           = user32.NewProc("SendInput")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procSendMessageTimeoutW = user32.NewProc("SendMessageTimeoutW")
	procGetClipboardSeqNo   = user32.NewProc("GetClipboardSequenceNumber")
	procGetWindowThreadPID  = user32.NewProc("GetWindowThreadProcessId")
	procGetGUIThreadInfo    = user32.NewProc("GetGUIThreadInfo")
	procEnumWindows         = user32.NewProc("EnumWindows")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procIsIconic            = user32.NewProc("IsIconic")
	procGetWindowLongPtrW   = user32.NewProc("GetWindowLongPtrW")
	procLoadIconW           = user32.NewProc("LoadIconW")
	procLoadImageW          = user32.NewProc("LoadImageW")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenuW         = user32.NewProc("AppendMenuW")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procShellNotifyIconW    = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")

	globalWndProc = windows.NewCallback(wndProc)
	keyboardHook  = windows.NewCallback(lowLevelKeyboardProc)
	appState      *app
	appIconHandle windows.Handle
)

//go:embed assets/altcmd.ico
var embeddedIcon []byte

type app struct {
	hwnd                 windows.Handle
	configPath           string
	cfg                  config
	registeredByID       map[int]registeredHotkey
	altComboTable        [256]int // vk -> hotkey ID; for Alt+key combos (zero = not registered)
	pendingByKey         [256]int // vk -> hotkey ID; tracks swallowed combo key-downs awaiting key-up
	altDownPending       bool
	altDownVK            uint
	altComboHandled      bool
	syntheticAltInjected bool
	sendImmediate        func(...input) bool
	nextHotkeyID         int
	hookHandle           windows.Handle
	settingsHwnd         windows.Handle
	settingsList         windows.Handle
	settingsCombo        windows.Handle
	settingsAction       windows.Handle
	settingsEnabled      windows.Handle
	settingsDraft        []hotkeyConfig
	settingsSelected     int
	settingsConflicts    map[int]string
	settingsGameStatus   windows.Handle
}

type config struct {
	ConfigVersion   int            `json:"config_version"`
	GameModeEnabled bool           `json:"game_mode_enabled"`
	AutoStart       bool           `json:"auto_start"`
	ShowConsole     bool           `json:"show_console"`
	GameProcesses   []string       `json:"game_processes"`
	Hotkeys         []hotkeyConfig `json:"hotkeys"`
}

func (cfg *config) UnmarshalJSON(data []byte) error {
	type configAlias config
	decoded := configAlias{AutoStart: true}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*cfg = config(decoded)
	return nil
}

type hotkeyConfig struct {
	Combo   string `json:"combo"`
	Action  string `json:"action"`
	Enabled bool   `json:"enabled"`
}

type registeredHotkey struct {
	ID         int
	Modifiers  uint
	Key        uint
	Enabled    bool
	ActionName string
	Label      string
}

type point struct {
	X int32
	Y int32
}

type msg struct {
	Hwnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type keybdinput struct {
	WVK         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type input struct {
	Type uint32
	_    uint32
	Ki   keybdinput
	_    [8]byte
}

type guithreadinfo struct {
	CbSize        uint32
	Flags         uint32
	HwndActive    windows.Handle
	HwndFocus     windows.Handle
	HwndCapture   windows.Handle
	HwndMenuOwner windows.Handle
	HwndMoveSize  windows.Handle
	HwndCaret     windows.Handle
	RcCaret       struct {
		Left   int32
		Top    int32
		Right  int32
		Bottom int32
	}
}

type wndclassex struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         windows.Handle
	HCursor       windows.Handle
	HbrBackground windows.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       windows.Handle
}

type notifyIconData struct {
	CbSize           uint32
	HWnd             windows.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            windows.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UTimeoutOrVer    uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     windows.Handle
}

type kbdllhookstruct struct {
	VKCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(applicationVersion)
		return
	}

	if isTraceConsoleInvocation(os.Args) {
		if err := runTraceConsole(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	configPath, cfg, err := loadOrCreateConfig()
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	installed, err := installAndLaunch(configPath, &cfg)
	if err != nil {
		log.Fatalf("install AltCmd failed: %v", err)
	}
	if installed {
		return
	}

	a := &app{
		configPath:     configPath,
		cfg:            cfg,
		registeredByID: make(map[int]registeredHotkey),
		nextHotkeyID:   1,
	}
	appState = a
	if err := setConsoleVisible(a.cfg.ShowConsole); err != nil {
		log.Printf("apply console visibility failed: %v", err)
	}
	defer func() {
		if err := setConsoleVisible(false); err != nil {
			log.Printf("close trace console failed: %v", err)
		}
	}()

	hwnd, err := createMessageWindow()
	if err != nil {
		log.Fatalf("create message window failed: %v", err)
	}
	a.hwnd = hwnd

	if err := addTrayIcon(hwnd); err != nil {
		log.Fatalf("add tray icon failed: %v", err)
	}
	defer removeTrayIcon(hwnd)
	defer destroyWindow(hwnd)

	if err := applyAutoStart(a.cfg.AutoStart); err != nil {
		log.Printf("apply auto-start failed: %v", err)
	}

	a.registerHotkeys()
	if err := a.installKeyboardHook(); err != nil {
		log.Fatalf("install keyboard hook failed: %v", err)
	}
	defer a.uninstallKeyboardHook()

	log.Printf("AltCmd %s running in tray", applicationVersion)
	log.Printf("config path: %s", configPath)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		postMessage(hwnd, wmDestroy, 0, 0)
	}()

	runMessageLoop()
	log.Println("AltCmd exited")
}

func loadOrCreateConfig() (string, config, error) {
	path, err := defaultConfigPath()
	if err != nil {
		return "", config{}, err
	}

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		cfg := defaultConfig()
		if err := saveConfig(path, cfg); err != nil {
			return "", config{}, err
		}
		return path, cfg, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return "", config{}, err
	}

	var cfg config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", config{}, fmt.Errorf("invalid config json: %w", err)
	}

	normalizeConfig(&cfg)
	if migrateConfig(&cfg) {
		if err := saveConfig(path, cfg); err != nil {
			return "", config{}, fmt.Errorf("migrate config failed: %w", err)
		}
	}
	return path, cfg, nil
}

func defaultConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "AltCmd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func installedExecutablePath() (string, error) {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return "", errors.New("LOCALAPPDATA is not set")
	}
	return filepath.Join(localAppData, "Programs", "AltCmd", "AltCmd.exe"), nil
}

func installAndLaunch(configPath string, cfg *config) (bool, error) {
	executable, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("get current executable path failed: %w", err)
	}
	target, err := installedExecutablePath()
	if err != nil {
		return false, err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return false, fmt.Errorf("resolve current executable path failed: %w", err)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return false, fmt.Errorf("resolve install path failed: %w", err)
	}
	if strings.EqualFold(filepath.Clean(executable), filepath.Clean(target)) {
		return false, nil
	}

	if err := copyExecutable(executable, target); err != nil {
		return false, err
	}
	cfg.AutoStart = true
	if err := saveConfig(configPath, *cfg); err != nil {
		return false, fmt.Errorf("enable auto-start in configuration failed: %w", err)
	}
	if err := setAutoStart(true, target); err != nil {
		return false, fmt.Errorf("configure auto-start failed: %w", err)
	}
	cmd := exec.Command(target)
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("launch installed AltCmd failed: %w", err)
	}
	return true, nil
}

func copyExecutable(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create install directory failed: %w", err)
	}
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open current executable failed: %w", err)
	}
	defer input.Close()

	output, err := os.CreateTemp(filepath.Dir(target), "AltCmd-*.tmp")
	if err != nil {
		return fmt.Errorf("create staged executable failed: %w", err)
	}
	tempPath := output.Name()
	defer os.Remove(tempPath)

	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return fmt.Errorf("copy executable failed: %w", err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close staged executable failed: %w", err)
	}
	if err := os.Rename(tempPath, target); err != nil {
		return fmt.Errorf("move executable into install directory failed: %w", err)
	}
	return nil
}

func defaultConfig() config {
	return config{
		ConfigVersion:   currentConfigVersion,
		GameModeEnabled: false,
		AutoStart:       true,
		ShowConsole:     false,
		GameProcesses: []string{
			"valorant.exe",
			"league of legends.exe",
			"dota2.exe",
			"cs2.exe",
			"overwatch.exe",
			"eldenring.exe",
		},
		Hotkeys: []hotkeyConfig{
			{Combo: "Alt+C", Action: hotkeyActionCopy, Enabled: true},
			{Combo: "Alt+F", Action: hotkeyActionFind, Enabled: true},
			{Combo: "Alt+V", Action: hotkeyActionPaste, Enabled: true},
			{Combo: "Alt+X", Action: hotkeyActionCut, Enabled: true},
			{Combo: "Alt+Z", Action: hotkeyActionUndo, Enabled: true},
			{Combo: "Alt+S", Action: hotkeyActionSave, Enabled: true},
			{Combo: "Alt+A", Action: hotkeyActionSelectAll, Enabled: true},
			{Combo: "Alt+T", Action: hotkeyActionNewTab, Enabled: true},
			{Combo: "Alt+W", Action: hotkeyActionCloseTab, Enabled: true},
			{Combo: "Alt+Q", Action: hotkeyActionCloseWindow, Enabled: true},
			{Combo: "Alt+BACKTICK", Action: hotkeyActionSwitchWindow, Enabled: true},
		},
	}
}

func migrateConfig(cfg *config) bool {
	if cfg.ConfigVersion >= currentConfigVersion {
		return false
	}

	findMods, findKey, _ := parseCombo("Alt+F")
	for _, item := range cfg.Hotkeys {
		mods, key, err := parseCombo(item.Combo)
		if err == nil && mods == findMods && key == findKey {
			cfg.ConfigVersion = currentConfigVersion
			return true
		}
	}

	cfg.Hotkeys = append(cfg.Hotkeys, hotkeyConfig{
		Combo:   "Alt+F",
		Action:  hotkeyActionFind,
		Enabled: true,
	})
	cfg.ConfigVersion = currentConfigVersion
	return true
}

func normalizeConfig(cfg *config) {
	if len(cfg.GameProcesses) == 0 {
		cfg.GameProcesses = defaultConfig().GameProcesses
	}
	if len(cfg.Hotkeys) == 0 {
		cfg.Hotkeys = defaultConfig().Hotkeys
	}
	for i := range cfg.GameProcesses {
		cfg.GameProcesses[i] = strings.ToLower(strings.TrimSpace(cfg.GameProcesses[i]))
	}
	for i := range cfg.Hotkeys {
		cfg.Hotkeys[i].Action = strings.ToLower(strings.TrimSpace(cfg.Hotkeys[i].Action))
		cfg.Hotkeys[i].Combo = strings.TrimSpace(cfg.Hotkeys[i].Combo)
	}
}

func saveConfig(path string, cfg config) error {
	normalizeConfig(&cfg)
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func createMessageWindow() (windows.Handle, error) {
	hInstance, _, _ := procGetModuleHandleW.Call(0)
	className, err := windows.UTF16PtrFromString("AltCmdMessageWindow")
	if err != nil {
		return 0, err
	}

	icon := loadAppIconHandle()

	wc := wndclassex{
		CbSize:        uint32(unsafe.Sizeof(wndclassex{})),
		LpfnWndProc:   globalWndProc,
		HInstance:     windows.Handle(hInstance),
		HIcon:         icon,
		HIconSm:       icon,
		LpszClassName: className,
	}

	r, _, regErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if r == 0 {
		const errClassAlreadyExists syscall.Errno = 1410
		if !errors.Is(regErr, errClassAlreadyExists) {
			return 0, fmt.Errorf("RegisterClassExW failed: %w", regErr)
		}
	}

	title, _ := windows.UTF16PtrFromString("AltCmd")
	hwnd, _, createErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		0, 0, 0, 0, 0,
		0, 0,
		hInstance,
		0,
	)
	if hwnd == 0 {
		return 0, fmt.Errorf("CreateWindowExW failed: %w", createErr)
	}

	return windows.Handle(hwnd), nil
}

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmHotkey:
		if appState != nil {
			appState.handleHotkeyID(int(wParam))
		}
		return 0
	case wmExecute:
		if appState != nil {
			appState.handleHotkeyID(int(wParam))
		}
		return 0
	case wmCommand:
		if appState != nil {
			menuID := int(uint16(wParam & 0xFFFF))
			appState.handleMenuCommand(menuID)
		}
		return 0
	case wmTrayIcon:
		eventCode := uint32(lParam & 0xFFFF)
		if eventCode == wmRButtonUp || eventCode == wmLButtonUp || eventCode == wmContextMenu || eventCode == wmLButtonDblClk || eventCode == ninSelect || eventCode == ninKeySelect {
			showTrayMenu(windows.Handle(hwnd))
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	default:
		r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
		return r
	}
}

func addTrayIcon(hwnd windows.Handle) error {
	var nid notifyIconData
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = hwnd
	nid.UID = trayUID
	nid.UFlags = nifMessage | nifIcon | nifTip
	nid.UCallbackMessage = wmTrayIcon

	nid.HIcon = loadAppIconHandle()
	copyUTF16(nid.SzTip[:], "AltCmd")

	r, _, err := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	if r == 0 {
		return fmt.Errorf("Shell_NotifyIconW(NIM_ADD) failed: %w", err)
	}

	nid.UTimeoutOrVer = notifyIconVersion4
	procShellNotifyIconW.Call(nimSetVersion, uintptr(unsafe.Pointer(&nid)))
	return nil
}

func removeTrayIcon(hwnd windows.Handle) {
	var nid notifyIconData
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = hwnd
	nid.UID = trayUID
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
}

func showTrayMenu(hwnd windows.Handle) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	flagsGame := uintptr(mfString)
	if appState != nil && appState.cfg.GameModeEnabled {
		flagsGame |= mfChecked
	}
	appendMenu(menu, flagsGame, idMenuToggleGameMode, "游戏模式")

	flagsStartup := uintptr(mfString)
	if appState != nil && appState.cfg.AutoStart {
		flagsStartup |= mfChecked
	}
	appendMenu(menu, flagsStartup, idMenuToggleStartup, "开机自启动")

	flagsConsole := uintptr(mfString)
	if appState != nil && appState.cfg.ShowConsole {
		flagsConsole |= mfChecked
	}
	appendMenu(menu, flagsConsole, idMenuToggleConsole, "显示追踪终端")

	appendMenu(menu, mfString, idMenuSettings, "快捷键设置...")
	appendMenu(menu, mfString, idMenuReloadConfig, "重载配置")
	appendMenu(menu, mfSeparator, 0, "")
	appendMenu(menu, mfString, idMenuAbout, "关于 AltCmd...")
	appendMenu(menu, mfSeparator, 0, "")
	appendMenu(menu, mfString, idMenuExit, "退出")

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(uintptr(hwnd))
	procTrackPopupMenu.Call(
		menu,
		tpmLeftAlign|tpmBottomAlign,
		uintptr(pt.X),
		uintptr(pt.Y),
		0,
		uintptr(hwnd),
		0,
	)
	postMessage(hwnd, wmNull, 0, 0)
}

func appendMenu(menu uintptr, flags uintptr, id int, text string) {
	var textPtr *uint16
	if text != "" {
		textPtr, _ = windows.UTF16PtrFromString(text)
	}
	procAppendMenuW.Call(menu, flags, uintptr(id), uintptr(unsafe.Pointer(textPtr)))
}

func (a *app) handleMenuCommand(id int) {
	switch id {
	case idMenuToggleGameMode:
		a.cfg.GameModeEnabled = !a.cfg.GameModeEnabled
		if err := saveConfig(a.configPath, a.cfg); err != nil {
			log.Printf("save config failed: %v", err)
		}
		log.Printf("game mode: %v", a.cfg.GameModeEnabled)
	case idMenuToggleStartup:
		a.cfg.AutoStart = !a.cfg.AutoStart
		if err := applyAutoStart(a.cfg.AutoStart); err != nil {
			log.Printf("toggle auto-start failed: %v", err)
			a.cfg.AutoStart = !a.cfg.AutoStart
			return
		}
		if err := saveConfig(a.configPath, a.cfg); err != nil {
			log.Printf("save config failed: %v", err)
		}
		log.Printf("auto-start: %v", a.cfg.AutoStart)
	case idMenuToggleConsole:
		oldValue := a.cfg.ShowConsole
		newValue := !oldValue
		if err := setConsoleVisible(newValue); err != nil {
			log.Printf("toggle console visibility failed: %v", err)
			return
		}
		a.cfg.ShowConsole = newValue
		if err := saveConfig(a.configPath, a.cfg); err != nil {
			a.cfg.ShowConsole = oldValue
			if restoreErr := setConsoleVisible(oldValue); restoreErr != nil {
				log.Printf("restore console visibility failed: %v", restoreErr)
			}
			log.Printf("save console visibility failed: %v", err)
			return
		}
		if a.cfg.ShowConsole {
			log.Printf("trace console shown")
		}
	case idMenuSettings:
		if err := a.showSettingsWindow(); err != nil {
			log.Printf("open settings window failed: %v", err)
			showSettingsMessage(a.hwnd, "无法打开快捷键设置窗口：\n"+err.Error(), true)
		}
	case idMenuReloadConfig:
		cfgPath, cfg, err := loadOrCreateConfig()
		if err != nil {
			log.Printf("reload config failed: %v", err)
			return
		}
		a.configPath = cfgPath
		a.cfg = cfg
		if err := applyAutoStart(a.cfg.AutoStart); err != nil {
			log.Printf("apply auto-start failed: %v", err)
		}
		a.registerHotkeys()
		log.Printf("config reloaded")
	case idMenuAbout:
		if err := showAboutDialog(a.hwnd); err != nil {
			log.Printf("show about dialog failed: %v", err)
		}
	case idMenuExit:
		postMessage(a.hwnd, wmDestroy, 0, 0)
	}
}

func applyAutoStart(enabled bool) error {
	executable := ""
	if enabled {
		var err error
		executable, err = os.Executable()
		if err != nil {
			return err
		}
	}
	return setAutoStart(enabled, executable)
}

func setAutoStart(enabled bool, executable string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	const valueName = "AltCmd"
	if !enabled {
		err := key.DeleteValue(valueName)
		if err != nil && !errors.Is(err, syscall.Errno(2)) {
			return err
		}
		return nil
	}

	if executable == "" {
		return errors.New("executable path is required to enable auto-start")
	}
	executable = filepath.Clean(executable)
	value := fmt.Sprintf("\"%s\"", executable)
	return key.SetStringValue(valueName, value)
}

func (a *app) registerHotkeys() {
	a.registeredByID = make(map[int]registeredHotkey)
	a.altComboTable = [256]int{}
	a.pendingByKey = [256]int{}
	a.altDownPending = false
	a.altDownVK = 0
	a.altComboHandled = false
	a.syntheticAltInjected = false
	a.nextHotkeyID = 1

	seen := make(map[string]struct{})
	registeredCount := 0
	for _, item := range a.cfg.Hotkeys {
		if !item.Enabled {
			continue
		}
		act := strings.ToLower(strings.TrimSpace(item.Action))
		if !isSupportedAction(act) {
			log.Printf("skip unsupported action: %s", item.Action)
			continue
		}

		mods, key, err := parseCombo(item.Combo)
		if err != nil {
			log.Printf("skip invalid combo %q: %v", item.Combo, err)
			continue
		}

		sig := fmt.Sprintf("%d:%d", mods, key)
		if _, exists := seen[sig]; exists {
			log.Printf("skip duplicate combo: %s", item.Combo)
			continue
		}
		seen[sig] = struct{}{}

		id := a.nextHotkeyID
		a.nextHotkeyID++

		hk := registeredHotkey{
			ID:         id,
			Modifiers:  mods,
			Key:        key,
			Enabled:    item.Enabled,
			ActionName: act,
			Label:      fmt.Sprintf("%s -> %s", item.Combo, act),
		}
		a.registeredByID[id] = hk

		// Fast lookup table for Alt+key combos (zero allocation in hook path).
		if mods == modAlt && key < 256 {
			a.altComboTable[key] = id
		}

		registeredCount++
		log.Printf("registered: %s -> %s", item.Combo, act)
	}

	if registeredCount == 0 {
		log.Printf("warning: no hotkey registered")
	}
}

func (a *app) installKeyboardHook() error {
	hInstance, _, _ := procGetModuleHandleW.Call(0)
	r, _, err := procSetWindowsHookExW.Call(
		whKeyboardLL,
		keyboardHook,
		hInstance,
		0,
	)
	if r == 0 {
		return fmt.Errorf("SetWindowsHookExW failed: %w", err)
	}
	a.hookHandle = windows.Handle(r)
	return nil
}

func (a *app) uninstallKeyboardHook() {
	if a.hookHandle == 0 {
		return
	}
	procUnhookWindowsHookEx.Call(uintptr(a.hookHandle))
	a.hookHandle = 0
}

func (a *app) handleHotkeyID(id int) {
	hk, ok := a.registeredByID[id]
	if !ok || !hk.Enabled {
		return
	}

	log.Printf("hotkey triggered: %s", hk.Label)

	if a.cfg.GameModeEnabled {
		if inGame, proc := isForegroundGame(a.cfg.GameProcesses); inGame {
			log.Printf("game mode blocked hotkey (%s), process=%s", hk.Label, proc)
			return
		}
	}

	switch hk.ActionName {
	case hotkeyActionCopy:
		sendCtrlCombo('C')
	case hotkeyActionFind:
		sendCtrlCombo('F')
	case hotkeyActionPaste:
		sendCtrlCombo('V')
	case hotkeyActionCut:
		sendCtrlCombo('X')
	case hotkeyActionUndo:
		sendCtrlCombo('Z')
	case hotkeyActionSave:
		sendCtrlCombo('S')
	case hotkeyActionSelectAll:
		sendCtrlCombo('A')
	case hotkeyActionNewTab:
		sendCtrlCombo('T')
	case hotkeyActionCloseTab:
		sendCtrlCombo('W')
	case hotkeyActionCloseWindow:
		sendAltF4()
	case hotkeyActionSwitchWindow:
		switchForegroundWindow()
	}
}

// handleHookKeyDown processes key-down events from the low-level keyboard hook.
// CRITICAL: Zero allocations. No fmt, no log, no map writes, no channel ops.
func (a *app) handleHookKeyDown(vk uint) bool {
	// Always swallow real Alt key-down.
	if vk == vkLMenu || vk == vkRMenu || vk == vkMenu {
		if !a.altDownPending {
			a.pendingByKey = [256]int{}
		}
		a.altDownPending = true
		a.altDownVK = vk
		a.altComboHandled = false
		return true
	}

	// If Alt is not pending, pass through.
	if !a.altDownPending {
		return false
	}

	// Escape cancels the pending Alt combo instead of replaying Alt into the system.
	if vk == vkEscape {
		a.cancelPendingAltCombo()
		return false
	}

	// Another modifier while Alt pending: flush Alt to system, pass modifier through.
	if isModifierKey(vk) {
		a.flushPendingAlt()
		return false
	}

	// Check registered Alt+key combo via fast table lookup (no allocation).
	if vk < 256 {
		if id := a.altComboTable[vk]; id != 0 {
			if a.pendingByKey[vk] != 0 {
				return true
			}
			a.pendingByKey[vk] = id
			a.altComboHandled = true
			postMessage(a.hwnd, wmExecute, uintptr(id), 0)
			return true
		}
	}

	// Not a registered combo: flush Alt, let key pass through.
	a.flushPendingAlt()
	return false
}

// handleHookKeyUp processes key-up events from the low-level keyboard hook.
// CRITICAL: Zero allocations.
func (a *app) handleHookKeyUp(vk uint) bool {
	// Swallow real Alt key-up; replay a complete tap only when no combo used it.
	if vk == vkLMenu || vk == vkRMenu || vk == vkMenu {
		if a.syntheticAltInjected {
			a.sendHookInput(keyEvent(uint16(vk), false))
			a.syntheticAltInjected = false
		} else if a.altDownPending && !a.altComboHandled {
			altVK := a.altDownVK
			if altVK == 0 {
				altVK = vk
			}
			a.sendHookInput(keyEvent(uint16(altVK), true), keyEvent(uint16(altVK), false))
		}
		a.altDownPending = false
		a.altDownVK = 0
		a.altComboHandled = false
		return true
	}

	if vk < 256 {
		if a.pendingByKey[vk] != 0 {
			a.pendingByKey[vk] = 0
			return true
		}
	}

	return false
}

func (a *app) flushPendingAlt() {
	if !a.altDownPending {
		return
	}
	vk := uint16(vkMenu)
	if a.altDownVK != 0 {
		vk = uint16(a.altDownVK)
	}
	a.syntheticAltInjected = a.sendHookInput(keyEvent(vk, true))
	a.altDownPending = false
	a.altDownVK = 0
}

func (a *app) sendHookInput(seq ...input) bool {
	if a.sendImmediate != nil {
		return a.sendImmediate(seq...)
	}
	return sendSequenceImmediate(seq...)
}

func (a *app) cancelPendingAltCombo() {
	a.altDownPending = false
	a.altDownVK = 0
	a.altComboHandled = false
	a.syntheticAltInjected = false
	a.pendingByKey = [256]int{}
}

func isModifierKey(vk uint) bool {
	switch vk {
	case vkControl, vkLControl, vkRControl,
		vkMenu, vkLMenu, vkRMenu,
		vkShift, vkLShift, vkRShift,
		vkLWin, vkRWin:
		return true
	}
	return false
}

func lowLevelKeyboardProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode != hcAction || appState == nil {
		return callNextHook(nCode, wParam, lParam)
	}
	if appState.settingsHwnd != 0 {
		foreground, _, _ := procGetForegroundWindow.Call()
		rootOwner, _, _ := procGetAncestor.Call(foreground, 3)
		if windows.Handle(rootOwner) == appState.settingsHwnd {
			return callNextHook(nCode, wParam, lParam)
		}
	}

	info := (*kbdllhookstruct)(unsafe.Pointer(lParam))
	if (info.Flags & llkhfInjected) != 0 {
		return callNextHook(nCode, wParam, lParam)
	}

	vk := uint(info.VKCode)
	switch uint32(wParam) {
	case wmKeyDown, wmSysKeyDown:
		if appState.handleHookKeyDown(vk) {
			return 1
		}
	case wmKeyUp, wmSysKeyUp:
		if appState.handleHookKeyUp(vk) {
			return 1
		}
	}

	return callNextHook(nCode, wParam, lParam)
}

func callNextHook(nCode int, wParam uintptr, lParam uintptr) uintptr {
	r, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return r
}

func isSupportedAction(action string) bool {
	switch action {
	case hotkeyActionCopy, hotkeyActionFind, hotkeyActionPaste, hotkeyActionCut, hotkeyActionUndo,
		hotkeyActionSave, hotkeyActionSelectAll, hotkeyActionNewTab,
		hotkeyActionCloseTab, hotkeyActionCloseWindow,
		hotkeyActionSwitchWindow:
		return true
	default:
		return false
	}
}

func parseCombo(combo string) (uint, uint, error) {
	tokens := strings.Split(strings.ToUpper(strings.TrimSpace(combo)), "+")
	if len(tokens) < 2 {
		return 0, 0, fmt.Errorf("combo needs at least one modifier and one key")
	}

	var mods uint
	keyToken := ""
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		switch t {
		case "ALT":
			mods |= modAlt
		case "CTRL", "CONTROL":
			mods |= modControl
		case "SHIFT":
			mods |= modShift
		case "WIN", "WINDOWS":
			mods |= modWin
		default:
			if keyToken != "" {
				return 0, 0, fmt.Errorf("multiple key tokens in combo")
			}
			keyToken = t
		}
	}

	if mods == 0 {
		return 0, 0, fmt.Errorf("missing modifier")
	}
	if keyToken == "" {
		return 0, 0, fmt.Errorf("missing key")
	}

	vk, ok := parseVirtualKey(keyToken)
	if !ok {
		return 0, 0, fmt.Errorf("unsupported key: %s", keyToken)
	}

	return mods, vk, nil
}

func parseVirtualKey(token string) (uint, bool) {
	if len(token) == 1 {
		ch := token[0]
		if (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			return uint(ch), true
		}
		if ch == '`' {
			return vkOem3, true
		}
	}

	if strings.HasPrefix(token, "F") {
		num := strings.TrimPrefix(token, "F")
		switch num {
		case "1":
			return 0x70, true
		case "2":
			return 0x71, true
		case "3":
			return 0x72, true
		case "4":
			return 0x73, true
		case "5":
			return 0x74, true
		case "6":
			return 0x75, true
		case "7":
			return 0x76, true
		case "8":
			return 0x77, true
		case "9":
			return 0x78, true
		case "10":
			return 0x79, true
		case "11":
			return 0x7A, true
		case "12":
			return 0x7B, true
		}
	}

	nameMap := map[string]uint{
		"TAB":       0x09,
		"SPACE":     0x20,
		"ESC":       0x1B,
		"ESCAPE":    0x1B,
		"LEFT":      0x25,
		"UP":        0x26,
		"RIGHT":     0x27,
		"DOWN":      0x28,
		"INSERT":    0x2D,
		"DELETE":    0x2E,
		"HOME":      0x24,
		"END":       0x23,
		"PAGEUP":    0x21,
		"PAGEDOWN":  0x22,
		"BACKSPACE": 0x08,
		"ENTER":     0x0D,
		"BACKTICK":  vkOem3,
		"GRAVE":     vkOem3,
	}
	vk, ok := nameMap[token]
	return vk, ok
}

func runMessageLoop() {
	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) == -1 {
			log.Println("GetMessageW returned error")
			return
		}
		if r == 0 {
			return
		}

		if appState != nil && appState.settingsHwnd != 0 {
			isDialog, _, _ := procIsDialogMessageW.Call(
				uintptr(appState.settingsHwnd),
				uintptr(unsafe.Pointer(&m)),
			)
			if isDialog != 0 {
				continue
			}
		}

		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func isForegroundTerminal(hwnd windows.Handle) (bool, string) {
	full := foregroundProcessPath(hwnd)
	if full == "" {
		return false, ""
	}
	lower := strings.ToLower(full)
	candidates := []string{
		"\\cmd.exe",
		"\\powershell.exe",
		"\\pwsh.exe",
		"\\windowsterminal.exe",
		"\\wt.exe",
		"\\mintty.exe",
		"\\bash.exe",
		"\\conhost.exe",
	}
	for _, s := range candidates {
		if strings.HasSuffix(lower, s) {
			return true, full
		}
	}
	return false, full
}

func isForegroundGame(gameProcesses []string) (bool, string) {
	full := foregroundProcessPath(0)
	if full == "" {
		return false, ""
	}

	lowerFull := strings.ToLower(full)
	base := strings.ToLower(filepath.Base(full))
	for _, g := range gameProcesses {
		g = strings.ToLower(strings.TrimSpace(g))
		if g == "" {
			continue
		}
		if base == g || strings.HasSuffix(lowerFull, "\\"+g) {
			return true, full
		}
	}
	return false, full
}

func foregroundProcessPath(hwnd windows.Handle) string {
	if hwnd == 0 {
		hwnd = getForegroundWindow()
	}
	if hwnd == 0 {
		return ""
	}

	var pid uint32
	procGetWindowThreadPID.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ""
	}

	h, err := windows.OpenProcess(processQueryLimited, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)

	buff := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buff))
	if err := windows.QueryFullProcessImageName(h, 0, &buff[0], &size); err != nil {
		return ""
	}

	return windows.UTF16ToString(buff[:size])
}

func focusedHandle(fg windows.Handle) windows.Handle {
	if fg == 0 {
		return 0
	}
	var pid uint32
	threadID, _, _ := procGetWindowThreadPID.Call(uintptr(fg), uintptr(unsafe.Pointer(&pid)))
	if pid == 0 || threadID == 0 {
		return 0
	}

	var gti guithreadinfo
	gti.CbSize = uint32(unsafe.Sizeof(gti))
	r, _, _ := procGetGUIThreadInfo.Call(threadID, uintptr(unsafe.Pointer(&gti)))
	if r == 0 {
		return 0
	}
	return gti.HwndFocus
}

func switchForegroundWindow() {
	fg := getForegroundWindow()
	if fg == 0 {
		return
	}

	var currentPID uint32
	procGetWindowThreadPID.Call(uintptr(fg), uintptr(unsafe.Pointer(&currentPID)))
	if currentPID == 0 {
		return
	}

	candidates := make([]windows.Handle, 0, 8)
	enumProc := windows.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		handle := windows.Handle(hwnd)
		if !isSwitchableWindow(handle) {
			return 1
		}

		var pid uint32
		procGetWindowThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid != currentPID {
			return 1
		}

		candidates = append(candidates, handle)
		return 1
	})
	procEnumWindows.Call(enumProc, 0)

	if len(candidates) < 2 {
		return
	}

	target := candidates[0]
	for index, handle := range candidates {
		if handle != fg {
			continue
		}
		target = candidates[(index+1)%len(candidates)]
		break
	}

	if target != 0 && target != fg {
		procSetForegroundWindow.Call(uintptr(target))
	}
}

func isSwitchableWindow(hwnd windows.Handle) bool {
	if hwnd == 0 {
		return false
	}
	visible, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
	if visible == 0 {
		return false
	}
	iconic, _, _ := procIsIconic.Call(uintptr(hwnd))
	if iconic != 0 {
		return false
	}
	exStyleIndex := int32(gwlExstyle)
	exStyle, _, _ := procGetWindowLongPtrW.Call(uintptr(hwnd), uintptr(exStyleIndex))
	if exStyle&wsExToolWindow != 0 {
		return false
	}
	return true
}

func sendCtrlCombo(key uint8) {
	sendSequence(
		keyEvent(vkControl, true),
		keyEvent(uint16(key), true),
		keyEvent(uint16(key), false),
		keyEvent(vkControl, false),
	)
}

func sendAltF4() {
	sendSequence(
		keyEvent(vkMenu, true),
		keyEvent(vkF4, true),
		keyEvent(vkF4, false),
		keyEvent(vkMenu, false),
	)
}

func keyEvent(vk uint16, down bool) input {
	flags := uint32(0)
	if !down {
		flags |= keyeventfKeyup
	}
	return input{
		Type: inputKeyboard,
		Ki: keybdinput{
			WVK:     vk,
			WScan:   0,
			DwFlags: flags,
		},
	}
}

func sendSequence(seq ...input) {
	if !sendInputSequence(seq, true) {
		return
	}
	time.Sleep(8 * time.Millisecond)
}

// sendSequenceImmediate sends input without sleeping. Safe for hook callback.
func sendSequenceImmediate(seq ...input) bool {
	return sendInputSequence(seq, false)
}

func sendInputSequence(seq []input, allowLog bool) bool {
	if len(seq) == 0 {
		return true
	}

	totalSent := 0
	for attempt := 0; totalSent < len(seq) && attempt < sendInputMaxAttempts; attempt++ {
		sent, _, err := procSendInput.Call(
			uintptr(len(seq)-totalSent),
			uintptr(unsafe.Pointer(&seq[totalSent])),
			unsafe.Sizeof(seq[0]),
		)
		if sent == 0 {
			if allowLog {
				log.Printf("SendInput failed: remaining=%d err=%v", len(seq)-totalSent, err)
			}
			continue
		}
		totalSent += int(sent)
	}

	if totalSent == len(seq) {
		return true
	}

	if allowLog {
		log.Printf("SendInput incomplete after retry: want=%d got=%d", len(seq), totalSent)
	}
	releasePressedKeys(seq[:totalSent], allowLog)
	return false
}

func releasePressedKeys(seq []input, allowLog bool) {
	if len(seq) == 0 {
		return
	}

	var pressed [256]int
	for _, event := range seq {
		if event.Type != inputKeyboard {
			continue
		}
		vk := event.Ki.WVK
		if vk >= 256 {
			continue
		}
		if event.Ki.DwFlags&keyeventfKeyup != 0 {
			if pressed[vk] > 0 {
				pressed[vk]--
			}
			continue
		}
		pressed[vk]++
	}

	cleanup := make([]input, 0, len(seq))
	for index := len(seq) - 1; index >= 0; index-- {
		event := seq[index]
		if event.Type != inputKeyboard {
			continue
		}
		vk := event.Ki.WVK
		if vk >= 256 || event.Ki.DwFlags&keyeventfKeyup != 0 {
			continue
		}
		if pressed[vk] == 0 {
			continue
		}
		cleanup = append(cleanup, keyEvent(vk, false))
		pressed[vk]--
	}

	if len(cleanup) == 0 {
		return
	}

	for attempt := 0; attempt < sendInputMaxAttempts; attempt++ {
		sent, _, err := procSendInput.Call(
			uintptr(len(cleanup)),
			uintptr(unsafe.Pointer(&cleanup[0])),
			unsafe.Sizeof(cleanup[0]),
		)
		if int(sent) == len(cleanup) {
			return
		}
		if allowLog {
			log.Printf("SendInput cleanup incomplete: want=%d got=%d err=%v", len(cleanup), sent, err)
		}
	}
}

func getForegroundWindow() windows.Handle {
	r, _, _ := procGetForegroundWindow.Call()
	return windows.Handle(r)
}

func postMessage(hwnd windows.Handle, message uint32, wParam, lParam uintptr) {
	procPostMessageW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
}

func destroyWindow(hwnd windows.Handle) {
	procDestroyWindow.Call(uintptr(hwnd))
}

func loadAppIconHandle() windows.Handle {
	if appIconHandle != 0 {
		return appIconHandle
	}

	path, err := ensureEmbeddedIconFile()
	if err == nil {
		pathPtr, pathErr := windows.UTF16PtrFromString(path)
		if pathErr == nil {
			handle, _, _ := procLoadImageW.Call(
				0,
				uintptr(unsafe.Pointer(pathPtr)),
				imageIcon,
				0,
				0,
				lrLoadFromFile|lrDefaultSize,
			)
			if handle != 0 {
				appIconHandle = windows.Handle(handle)
				return appIconHandle
			}
		}
	}

	handle, _, _ := procLoadIconW.Call(0, idiApplication)
	appIconHandle = windows.Handle(handle)
	return appIconHandle
}

func ensureEmbeddedIconFile() (string, error) {
	if len(embeddedIcon) == 0 {
		return "", errors.New("embedded icon is empty")
	}

	baseDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	iconDir := filepath.Join(baseDir, "AltCmd")
	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		return "", err
	}

	iconPath := filepath.Join(iconDir, "altcmd.ico")
	existing, err := os.ReadFile(iconPath)
	if err == nil && bytes.Equal(existing, embeddedIcon) {
		return iconPath, nil
	}
	if err := os.WriteFile(iconPath, embeddedIcon, 0o644); err != nil {
		return "", err
	}
	return iconPath, nil
}

func copyUTF16(dst []uint16, s string) {
	if len(dst) == 0 {
		return
	}
	u := utf16FromString(s)
	max := len(dst) - 1
	if len(u) < max {
		max = len(u)
	}
	copy(dst[:max], u[:max])
	dst[max] = 0
}

func utf16FromString(s string) []uint16 {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return []uint16{}
	}
	if len(u) == 0 {
		return []uint16{}
	}
	return u[:len(u)-1]
}
