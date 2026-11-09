package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestConfigDefaultsAutoStartOnAndConsoleOff(t *testing.T) {
	var cfg config
	if err := json.Unmarshal([]byte(`{"auto_start":false}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.ShowConsole {
		t.Fatal("show_console should default to false when omitted")
	}
	if cfg.AutoStart {
		t.Fatal("an explicit auto_start=false should be respected")
	}
}

func TestConfigAutoStartDefaultsToTrue(t *testing.T) {
	var cfg config
	if err := json.Unmarshal([]byte(`{}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.AutoStart {
		t.Fatal("auto_start should default to true when omitted")
	}
}

func TestConfigShowConsoleCanBeEnabled(t *testing.T) {
	var cfg config
	if err := json.Unmarshal([]byte(`{"show_console":true}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.ShowConsole {
		t.Fatal("show_console should respect an explicit true value")
	}
}

func TestApplicationVersionMatchesVersionFile(t *testing.T) {
	raw, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := applicationVersion, strings.TrimSpace(string(raw)); got != want {
		t.Fatalf("application version = %q, want VERSION value %q", got, want)
	}
}

func TestSaveConfigPersistsHotkeyEnabledState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config{
		Hotkeys: []hotkeyConfig{
			{Combo: "Alt+C", Action: hotkeyActionCopy, Enabled: true},
			{Combo: "Alt+V", Action: hotkeyActionPaste, Enabled: false},
		},
	}
	if err := saveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved config
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Hotkeys) != 2 || !saved.Hotkeys[0].Enabled || saved.Hotkeys[1].Enabled {
		t.Fatalf("saved hotkey enabled states = %#v, want [true false]", saved.Hotkeys)
	}
}

func TestDefaultConfigEnablesAutoStartAndHidesConsole(t *testing.T) {
	cfg := defaultConfig()
	if !cfg.AutoStart {
		t.Fatal("new installations should enable auto-start")
	}
	if cfg.ShowConsole {
		t.Fatal("new installations should hide the trace console")
	}
}

func TestDefaultConfigIncludesFindHotkey(t *testing.T) {
	cfg := defaultConfig()
	found := false
	for _, item := range cfg.Hotkeys {
		if item.Combo == "Alt+F" && item.Action == hotkeyActionFind && item.Enabled {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("default config should enable Alt+F to find")
	}

	a := &app{cfg: cfg}
	a.registerHotkeys()
	id := a.altComboTable['F']
	if id == 0 || a.registeredByID[id].ActionName != hotkeyActionFind {
		t.Fatalf("Alt+F registration = %#v, want find action", a.registeredByID[id])
	}
}

func TestDefaultConfigIncludesRefreshHotkey(t *testing.T) {
	cfg := defaultConfig()
	found := false
	for _, item := range cfg.Hotkeys {
		if item.Combo == "Alt+R" && item.Action == hotkeyActionRefresh && item.Enabled {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("default config should enable Alt+R to refresh")
	}

	a := &app{cfg: cfg}
	a.registerHotkeys()
	id := a.altComboTable['R']
	if id == 0 || a.registeredByID[id].ActionName != hotkeyActionRefresh {
		t.Fatalf("Alt+R registration = %#v, want refresh action", a.registeredByID[id])
	}
}

func TestMigrateConfigAddsFindHotkeyOnce(t *testing.T) {
	cfg := config{
		Hotkeys: []hotkeyConfig{
			{Combo: "Alt+C", Action: hotkeyActionCopy, Enabled: true},
		},
	}

	if !migrateConfig(&cfg) {
		t.Fatal("legacy config should be migrated")
	}
	if cfg.ConfigVersion != currentConfigVersion {
		t.Fatalf("config version = %d, want %d", cfg.ConfigVersion, currentConfigVersion)
	}
	if len(cfg.Hotkeys) != 3 ||
		cfg.Hotkeys[1] != (hotkeyConfig{Combo: "Alt+F", Action: hotkeyActionFind, Enabled: true}) ||
		cfg.Hotkeys[2] != (hotkeyConfig{Combo: "Alt+R", Action: hotkeyActionRefresh, Enabled: true}) {
		t.Fatalf("migrated hotkeys = %#v, want existing hotkey and enabled Alt+F/Alt+R", cfg.Hotkeys)
	}
	if migrateConfig(&cfg) {
		t.Fatal("an already migrated config should not be migrated again")
	}
}

func TestMigrateConfigVersionOneAddsRefreshHotkey(t *testing.T) {
	cfg := config{
		ConfigVersion: 1,
		Hotkeys: []hotkeyConfig{
			{Combo: "Alt+F", Action: hotkeyActionFind, Enabled: true},
		},
	}

	if !migrateConfig(&cfg) {
		t.Fatal("version-one config should be migrated")
	}
	if cfg.ConfigVersion != currentConfigVersion || len(cfg.Hotkeys) != 2 ||
		cfg.Hotkeys[1] != (hotkeyConfig{Combo: "Alt+R", Action: hotkeyActionRefresh, Enabled: true}) {
		t.Fatalf("migrated config = %#v, want enabled Alt+R refresh shortcut", cfg)
	}
}

func TestMigrateConfigPreservesExistingAltFHotkey(t *testing.T) {
	existing := hotkeyConfig{Combo: "alt+f", Action: hotkeyActionCopy, Enabled: false}
	cfg := config{Hotkeys: []hotkeyConfig{existing}}

	if !migrateConfig(&cfg) {
		t.Fatal("legacy config version should be updated")
	}
	if len(cfg.Hotkeys) != 2 || cfg.Hotkeys[0] != existing ||
		cfg.Hotkeys[1] != (hotkeyConfig{Combo: "Alt+R", Action: hotkeyActionRefresh, Enabled: true}) {
		t.Fatalf("migration changed existing Alt+F mapping: %#v", cfg.Hotkeys)
	}
}

func TestMigrateConfigPreservesExistingAltRHotkey(t *testing.T) {
	existing := hotkeyConfig{Combo: "alt+r", Action: hotkeyActionCopy, Enabled: false}
	cfg := config{ConfigVersion: 1, Hotkeys: []hotkeyConfig{existing}}

	if !migrateConfig(&cfg) {
		t.Fatal("version-one config should be migrated")
	}
	if len(cfg.Hotkeys) != 1 || cfg.Hotkeys[0] != existing {
		t.Fatalf("migration changed existing Alt+R mapping: %#v", cfg.Hotkeys)
	}
}

func TestInstalledExecutablePath(t *testing.T) {
	t.Setenv("LOCALAPPDATA", `C:\Users\test\AppData\Local`)
	got, err := installedExecutablePath()
	if err != nil {
		t.Fatal(err)
	}
	want := `C:\Users\test\AppData\Local\Programs\AltCmd\AltCmd.exe`
	if got != want {
		t.Fatalf("installedExecutablePath() = %q, want %q", got, want)
	}
}

func TestCopyExecutableReplacesTarget(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.exe")
	target := filepath.Join(dir, "install", "AltCmd.exe")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("new executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old executable"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := copyExecutable(source, target); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("new executable")) {
		t.Fatalf("installed executable = %q, want %q", got, "new executable")
	}
}

func TestRegisterHotkeysOnlyRegistersEnabledEntries(t *testing.T) {
	a := &app{
		cfg: config{
			Hotkeys: []hotkeyConfig{
				{Combo: "Alt+C", Action: hotkeyActionCopy, Enabled: false},
				{Combo: "Alt+V", Action: hotkeyActionPaste, Enabled: true},
			},
		},
	}

	a.registerHotkeys()
	if len(a.registeredByID) != 1 || a.altComboTable['C'] != 0 || a.altComboTable['V'] == 0 {
		t.Fatalf("registered hotkeys = %#v, Alt+C=%d, Alt+V=%d; want only Alt+V", a.registeredByID, a.altComboTable['C'], a.altComboTable['V'])
	}

	a.cfg.Hotkeys[0].Enabled = true
	a.cfg.Hotkeys[1].Enabled = false
	a.registerHotkeys()
	if len(a.registeredByID) != 1 || a.altComboTable['C'] == 0 || a.altComboTable['V'] != 0 {
		t.Fatalf("after toggling, registered hotkeys = %#v, Alt+C=%d, Alt+V=%d; want only Alt+C", a.registeredByID, a.altComboTable['C'], a.altComboTable['V'])
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

func TestStandaloneAltPressReplaysAltTapOnKeyUp(t *testing.T) {
	var sent []input
	a := &app{
		altDownPending: true,
		altDownVK:      vkLMenu,
		sendImmediate: func(seq ...input) bool {
			sent = append(sent, seq...)
			return true
		},
	}

	if !a.handleHookKeyUp(vkLMenu) {
		t.Fatal("standalone Alt key-up should be swallowed after replay")
	}
	if len(sent) != 2 {
		t.Fatalf("sent %d input events, want Alt key-down and key-up", len(sent))
	}
	if sent[0].Ki.WVK != uint16(vkLMenu) || sent[0].Ki.DwFlags != 0 {
		t.Fatalf("first input = %#v, want left Alt key-down", sent[0])
	}
	if sent[1].Ki.WVK != uint16(vkLMenu) || sent[1].Ki.DwFlags != keyeventfKeyup {
		t.Fatalf("second input = %#v, want left Alt key-up", sent[1])
	}
	if a.altDownPending || a.altDownVK != 0 {
		t.Fatal("Alt state should be cleared after key-up")
	}
}

func TestAltKeyUpDoesNotLeakMappedKeyRelease(t *testing.T) {
	a := &app{
		altDownPending:  true,
		pendingByKey:    [256]int{'Z': 9},
		altComboHandled: true,
		sendImmediate: func(...input) bool {
			t.Fatal("a handled Alt combo must not replay Alt")
			return false
		},
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
