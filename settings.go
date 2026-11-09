package main

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wsChild        = 0x40000000
	wsVisible      = 0x10000000
	wsTabStop      = 0x00010000
	wsBorder       = 0x00800000
	wsVScroll      = 0x00200000
	wsHScroll      = 0x00100000
	wsCaption      = 0x00C00000
	wsSysMenu      = 0x00080000
	wsMinimizeBox  = 0x00020000
	wsOverlapped   = wsCaption | wsSysMenu | wsMinimizeBox
	wsExClientEdge = 0x00000200

	esAutoHScroll = 0x00000080
	lbsNotify     = 0x00000001
	lbsHasStrings = 0x00000040
	cbsDropDown   = 0x00000002
	cbsHasStrings = 0x00000200
	bsAutoCheck   = 0x00000003

	lbAddString    = 0x0180
	lbResetContent = 0x0184
	lbSetCurSel    = 0x0186
	lbGetCurSel    = 0x0188
	cbAddString    = 0x0143
	cbResetContent = 0x014B
	cbSetCurSel    = 0x014E
	cbGetCurSel    = 0x0147
	bmGetCheck     = 0x00F0
	bmSetCheck     = 0x00F1
	bstChecked     = 1
	bnClicked      = 0
	lbnSelChange   = 1
	swShow         = 5
	mbOK           = 0x00000000
	mbIconError    = 0x00000010
	mbIconWarning  = 0x00000030

	settingsListID   = 2001
	settingsComboID  = 2002
	settingsActionID = 2003
	settingsEnableID = 2004
	settingsAddID    = 2005
	settingsUpdateID = 2006
	settingsRemoveID = 2007
	settingsProbeID  = 2008
	settingsSaveID   = 2009
	settingsCancelID = 2010

	modNoRepeat = 0x4000

	globalHotkeyProbeIDBase = 0x7000
	settingsAddGameID       = 2011

	ofnFileMustExist = 0x00001000
	ofnPathMustExist = 0x00000800
	ofnNoChangeDir   = 0x00000008
	ofnExplorer      = 0x00080000
)

var (
	settingsUser32           = windows.NewLazySystemDLL("user32.dll")
	settingsComdlg32         = windows.NewLazySystemDLL("comdlg32.dll")
	procRegisterHotKey       = settingsUser32.NewProc("RegisterHotKey")
	procUnregisterHotKey     = settingsUser32.NewProc("UnregisterHotKey")
	procSetWindowTextW       = settingsUser32.NewProc("SetWindowTextW")
	procGetWindowTextW       = settingsUser32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW = settingsUser32.NewProc("GetWindowTextLengthW")
	procSendMessageW         = settingsUser32.NewProc("SendMessageW")
	procShowWindow           = settingsUser32.NewProc("ShowWindow")
	procMessageBoxW          = settingsUser32.NewProc("MessageBoxW")
	procIsDialogMessageW     = settingsUser32.NewProc("IsDialogMessageW")
	procGetAncestor          = settingsUser32.NewProc("GetAncestor")
	procGetOpenFileNameW     = settingsComdlg32.NewProc("GetOpenFileNameW")
	procCommDlgExtendedError = settingsComdlg32.NewProc("CommDlgExtendedError")
	settingsWndProcCallback  = windows.NewCallback(settingsWindowProc)

	settingsClassRegistered bool
	settingsActions         = []struct {
		value string
		label string
	}{
		{hotkeyActionCopy, "复制"},
		{hotkeyActionPaste, "粘贴"},
		{hotkeyActionCut, "剪切"},
		{hotkeyActionUndo, "撤销"},
		{hotkeyActionSave, "保存"},
		{hotkeyActionSelectAll, "全选"},
		{hotkeyActionNewTab, "新建标签页"},
		{hotkeyActionCloseTab, "关闭标签页"},
		{hotkeyActionCloseWindow, "关闭窗口"},
		{hotkeyActionSwitchWindow, "切换应用窗口"},
	}
)

type openFileName struct {
	StructSize       uint32
	Owner            windows.Handle
	Instance         windows.Handle
	Filter           *uint16
	CustomFilter     *uint16
	MaxCustFilter    uint32
	FilterIndex      uint32
	File             *uint16
	MaxFile          uint32
	FileTitle        *uint16
	MaxFileTitle     uint32
	InitialDir       *uint16
	Title            *uint16
	Flags            uint32
	FileOffset       uint16
	FileExtension    uint16
	DefaultExtension *uint16
	CustData         uintptr
	Hook             uintptr
	TemplateName     *uint16
	Reserved         uintptr
	Reserved2        uint32
	FlagsEx          uint32
}

func (a *app) showSettingsWindow() error {
	if a.settingsHwnd != 0 {
		procShowWindow.Call(uintptr(a.settingsHwnd), swShow)
		procSetForegroundWindow.Call(uintptr(a.settingsHwnd))
		return nil
	}
	if err := registerSettingsWindowClass(); err != nil {
		return err
	}

	a.cancelPendingAltCombo()
	a.settingsDraft = append([]hotkeyConfig(nil), a.cfg.Hotkeys...)
	a.settingsSelected = 0
	a.settingsConflicts = nil

	className, err := windows.UTF16PtrFromString("AltCmdSettingsWindow")
	if err != nil {
		return err
	}
	title, err := windows.UTF16PtrFromString("AltCmd 快捷键设置")
	if err != nil {
		return err
	}
	hInstance, _, _ := procGetModuleHandleW.Call(0)
	hwnd, _, createErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsOverlapped|wsVisible,
		100, 100, 700, 600,
		0, 0, hInstance, 0,
	)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW(settings) failed: %w", createErr)
	}
	a.settingsHwnd = windows.Handle(hwnd)
	if err := a.createSettingsControls(); err != nil {
		procDestroyWindow.Call(hwnd)
		return err
	}
	a.refreshSettingsList()
	if len(a.settingsDraft) != 0 {
		a.loadSettingsSelection(0)
	}
	procSetForegroundWindow.Call(hwnd)
	return nil
}

func registerSettingsWindowClass() error {
	if settingsClassRegistered {
		return nil
	}
	className, err := windows.UTF16PtrFromString("AltCmdSettingsWindow")
	if err != nil {
		return err
	}
	hInstance, _, _ := procGetModuleHandleW.Call(0)
	wc := wndclassex{
		CbSize:        uint32(unsafe.Sizeof(wndclassex{})),
		LpfnWndProc:   settingsWndProcCallback,
		HInstance:     windows.Handle(hInstance),
		HIcon:         loadAppIconHandle(),
		HIconSm:       loadAppIconHandle(),
		HbrBackground: windows.Handle(6),
		LpszClassName: className,
	}
	r, _, registerErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if r == 0 {
		const errClassAlreadyExists syscall.Errno = 1410
		if !errors.Is(registerErr, errClassAlreadyExists) {
			return fmt.Errorf("RegisterClassExW(settings) failed: %w", registerErr)
		}
	}
	settingsClassRegistered = true
	return nil
}

func (a *app) createSettingsControls() error {
	var err error
	a.settingsList, err = createSettingsControl("LISTBOX", "", wsChild|wsVisible|wsBorder|wsVScroll|wsHScroll|wsTabStop|lbsNotify|lbsHasStrings, 20, 20, 285, 365, settingsListID, 0)
	if err != nil {
		return err
	}
	if _, err = createSettingsControl("STATIC", "快捷键组合（例如 Alt+C）", wsChild|wsVisible, 325, 25, 300, 22, 0, 0); err != nil {
		return err
	}
	a.settingsCombo, err = createSettingsControl("EDIT", "", wsChild|wsVisible|wsBorder|wsTabStop|esAutoHScroll, 325, 50, 330, 28, settingsComboID, wsExClientEdge)
	if err != nil {
		return err
	}
	if _, err = createSettingsControl("STATIC", "执行动作", wsChild|wsVisible, 325, 95, 300, 22, 0, 0); err != nil {
		return err
	}
	a.settingsAction, err = createSettingsControl("COMBOBOX", "", wsChild|wsVisible|wsTabStop|cbsDropDown|cbsHasStrings, 325, 120, 330, 220, settingsActionID, wsExClientEdge)
	if err != nil {
		return err
	}
	for _, action := range settingsActions {
		addSettingsString(a.settingsAction, cbAddString, action.label)
	}
	procSendMessageW.Call(uintptr(a.settingsAction), cbSetCurSel, 0, 0)

	a.settingsEnabled, err = createSettingsControl("BUTTON", "启用此快捷键", wsChild|wsVisible|wsTabStop|bsAutoCheck, 325, 165, 200, 28, settingsEnableID, 0)
	if err != nil {
		return err
	}
	procSendMessageW.Call(uintptr(a.settingsEnabled), bmSetCheck, bstChecked, 0)

	if _, err = createSettingsControl("STATIC", "当前仅支持 Alt+按键；冲突探测不能识别占用软件名称。", wsChild|wsVisible, 325, 210, 330, 38, 0, 0); err != nil {
		return err
	}
	if _, err = createSettingsControl("BUTTON", "新增", wsChild|wsVisible|wsTabStop, 325, 260, 95, 32, settingsAddID, 0); err != nil {
		return err
	}
	if _, err = createSettingsControl("BUTTON", "更新选中项", wsChild|wsVisible|wsTabStop, 430, 260, 115, 32, settingsUpdateID, 0); err != nil {
		return err
	}
	if _, err = createSettingsControl("BUTTON", "删除选中项", wsChild|wsVisible|wsTabStop, 325, 305, 125, 32, settingsRemoveID, 0); err != nil {
		return err
	}
	if _, err = createSettingsControl("BUTTON", "探测冲突", wsChild|wsVisible|wsTabStop, 465, 305, 120, 32, settingsProbeID, 0); err != nil {
		return err
	}
	if _, err = createSettingsControl("STATIC", "添加或更新项目后点“探测冲突”；保存后立即生效。", wsChild|wsVisible, 325, 365, 330, 40, 0, 0); err != nil {
		return err
	}
	a.settingsGameStatus, err = createSettingsControl("STATIC", "", wsChild|wsVisible, 325, 410, 330, 36, 0, 0)
	if err != nil {
		return err
	}
	if _, err = createSettingsControl("BUTTON", "添加游戏程序...", wsChild|wsVisible|wsTabStop, 325, 450, 160, 32, settingsAddGameID, 0); err != nil {
		return err
	}
	if _, err = createSettingsControl("BUTTON", "保存并应用", wsChild|wsVisible|wsTabStop, 435, 510, 115, 34, settingsSaveID, 0); err != nil {
		return err
	}
	if _, err = createSettingsControl("BUTTON", "取消", wsChild|wsVisible|wsTabStop, 565, 510, 90, 34, settingsCancelID, 0); err != nil {
		return err
	}
	a.updateGameStatus()
	return nil
}

func createSettingsControl(className, text string, style uint32, x, y, width, height, id int, exStyle uint32) (windows.Handle, error) {
	classPtr, err := windows.UTF16PtrFromString(className)
	if err != nil {
		return 0, err
	}
	textPtr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return 0, err
	}
	hInstance, _, _ := procGetModuleHandleW.Call(0)
	parent := uintptr(0)
	if appState != nil {
		parent = uintptr(appState.settingsHwnd)
	}
	hwnd, _, createErr := procCreateWindowExW.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(classPtr)),
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(style),
		uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		parent, uintptr(id), hInstance, 0,
	)
	if hwnd == 0 {
		return 0, fmt.Errorf("create %s control failed: %w", className, createErr)
	}
	return windows.Handle(hwnd), nil
}

func settingsWindowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	if appState == nil {
		return 0
	}
	switch message {
	case wmCommand:
		controlID := int(uint16(wParam & 0xFFFF))
		notification := uint16((wParam >> 16) & 0xFFFF)
		if controlID == settingsListID && notification == lbnSelChange {
			selected, _, _ := procSendMessageW.Call(uintptr(appState.settingsList), lbGetCurSel, 0, 0)
			index := int(int32(selected))
			appState.loadSettingsSelection(index)
			return 0
		}
		if notification == bnClicked {
			switch controlID {
			case settingsAddID:
				appState.addSettingsEntry()
			case settingsUpdateID:
				appState.updateSettingsEntry()
			case settingsRemoveID:
				appState.removeSettingsEntry()
			case settingsProbeID:
				appState.probeSettingsConflicts()
			case settingsAddGameID:
				appState.addGameExecutable()
			case settingsSaveID:
				appState.saveSettings()
			case settingsCancelID:
				procDestroyWindow.Call(hwnd)
			}
		}
		return 0
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		if appState.settingsHwnd == windows.Handle(hwnd) {
			appState.settingsHwnd = 0
			appState.settingsList = 0
			appState.settingsCombo = 0
			appState.settingsAction = 0
			appState.settingsEnabled = 0
			appState.settingsDraft = nil
			appState.settingsConflicts = nil
			appState.settingsSelected = -1
			appState.settingsGameStatus = 0
		}
		return 0
	default:
		result, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
		return result
	}
}

func (a *app) refreshSettingsList() {
	procSendMessageW.Call(uintptr(a.settingsList), lbResetContent, 0, 0)
	for i, item := range a.settingsDraft {
		status := "启用"
		if !item.Enabled {
			status = "禁用"
		}
		action := settingsActionLabel(item.Action)
		conflict := ""
		if result, ok := a.settingsConflicts[i]; ok {
			conflict = "    [" + result + "]"
		}
		addSettingsString(a.settingsList, lbAddString, fmt.Sprintf("%s    %s    [%s]%s", item.Combo, action, status, conflict))
	}
	if a.settingsSelected >= 0 && a.settingsSelected < len(a.settingsDraft) {
		procSendMessageW.Call(uintptr(a.settingsList), lbSetCurSel, uintptr(a.settingsSelected), 0)
	}
}

func (a *app) loadSettingsSelection(index int) {
	a.settingsSelected = index
	if index < 0 || index >= len(a.settingsDraft) {
		setWindowText(a.settingsCombo, "")
		procSendMessageW.Call(uintptr(a.settingsAction), cbSetCurSel, 0, 0)
		procSendMessageW.Call(uintptr(a.settingsEnabled), bmSetCheck, bstChecked, 0)
		return
	}
	item := a.settingsDraft[index]
	if err := setWindowText(a.settingsCombo, item.Combo); err != nil {
		setWindowText(a.settingsCombo, "")
		showSettingsMessage(a.settingsHwnd, "该快捷键包含无法显示的字符，请重新输入并更新。", true)
	}
	actionIndex := -1
	for i, action := range settingsActions {
		if action.value == item.Action {
			actionIndex = i
			break
		}
	}
	procSendMessageW.Call(uintptr(a.settingsAction), cbSetCurSel, uintptr(actionIndex), 0)
	check := uintptr(0)
	if item.Enabled {
		check = bstChecked
	}
	procSendMessageW.Call(uintptr(a.settingsEnabled), bmSetCheck, check, 0)
}

func (a *app) readSettingsEntry() (hotkeyConfig, error) {
	combo := strings.TrimSpace(getWindowText(a.settingsCombo))
	mods, _, err := parseCombo(combo)
	if err != nil {
		return hotkeyConfig{}, fmt.Errorf("快捷键格式无效：%w", err)
	}
	if mods != modAlt {
		return hotkeyConfig{}, errors.New("当前仅支持 Alt+按键组合，例如 Alt+C")
	}
	selected, _, _ := procSendMessageW.Call(uintptr(a.settingsAction), cbGetCurSel, 0, 0)
	actionIndex := int(int32(selected))
	if actionIndex < 0 || actionIndex >= len(settingsActions) {
		return hotkeyConfig{}, errors.New("请选择一个执行动作")
	}
	enabled, _, _ := procSendMessageW.Call(uintptr(a.settingsEnabled), bmGetCheck, 0, 0)
	return hotkeyConfig{
		Combo:   combo,
		Action:  settingsActions[actionIndex].value,
		Enabled: enabled == bstChecked,
	}, nil
}

func (a *app) addSettingsEntry() {
	item, err := a.readSettingsEntry()
	if err != nil {
		showSettingsMessage(a.settingsHwnd, err.Error(), true)
		return
	}
	a.settingsDraft = append(a.settingsDraft, item)
	a.settingsConflicts = nil
	a.settingsSelected = len(a.settingsDraft) - 1
	a.refreshSettingsList()
	a.loadSettingsSelection(a.settingsSelected)
}

func (a *app) updateSettingsEntry() {
	if a.settingsSelected < 0 || a.settingsSelected >= len(a.settingsDraft) {
		showSettingsMessage(a.settingsHwnd, "请先选择要更新的快捷键。", true)
		return
	}
	item, err := a.readSettingsEntry()
	if err != nil {
		showSettingsMessage(a.settingsHwnd, err.Error(), true)
		return
	}
	a.settingsDraft[a.settingsSelected] = item
	a.settingsConflicts = nil
	a.refreshSettingsList()
	a.loadSettingsSelection(a.settingsSelected)
}

func (a *app) removeSettingsEntry() {
	if a.settingsSelected < 0 || a.settingsSelected >= len(a.settingsDraft) {
		showSettingsMessage(a.settingsHwnd, "请先选择要删除的快捷键。", true)
		return
	}
	a.settingsDraft = append(a.settingsDraft[:a.settingsSelected], a.settingsDraft[a.settingsSelected+1:]...)
	a.settingsConflicts = nil
	if a.settingsSelected >= len(a.settingsDraft) {
		a.settingsSelected = len(a.settingsDraft) - 1
	}
	a.refreshSettingsList()
	a.loadSettingsSelection(a.settingsSelected)
}

func (a *app) probeSettingsConflicts() {
	conflicts, messages := inspectHotkeyConflicts(a.settingsDraft, func(index int, mods, key uint) (bool, error) {
		return probeWindowsRegisteredHotkey(a.hwnd, globalHotkeyProbeIDBase+index, mods, key)
	})
	a.settingsConflicts = conflicts
	a.refreshSettingsList()
	if len(messages) == 0 {
		showSettingsMessage(a.settingsHwnd, "未发现可探测的 Windows 全局热键冲突。\n此探测无法检查应用内部快捷键或其他低层键盘钩子。", false)
		return
	}
	showSettingsMessage(
		a.settingsHwnd,
		"探测结果：\n"+strings.Join(messages, "\n")+"\n\nWindows 不提供全局热键占用进程的查询接口，因此无法准确显示是哪个软件冲突。",
		true,
	)
}

func (a *app) addGameExecutable() {
	path, selected, err := selectGameExecutable(a.settingsHwnd)
	if err != nil {
		showSettingsMessage(a.settingsHwnd, "选择游戏程序失败：\n"+err.Error(), true)
		return
	}
	if !selected {
		return
	}

	cfg := a.cfg
	added, processName := addGameProcess(&cfg, path)
	if !added && cfg.GameModeEnabled {
		showSettingsMessage(a.settingsHwnd, processName+" 已在游戏名单中。", false)
		return
	}
	cfg.GameModeEnabled = true
	if err := saveConfig(a.configPath, cfg); err != nil {
		showSettingsMessage(a.settingsHwnd, "保存游戏名单失败：\n"+err.Error(), true)
		return
	}

	a.cfg = cfg
	a.updateGameStatus()
	if added {
		showSettingsMessage(
			a.settingsHwnd,
			fmt.Sprintf("已添加 %s，并自动开启游戏模式。\n该程序成为前台窗口时，Alt 快捷键映射会暂停。", processName),
			false,
		)
	} else {
		showSettingsMessage(a.settingsHwnd, "游戏模式已开启。", false)
	}
}

func selectGameExecutable(owner windows.Handle) (string, bool, error) {
	filter := append(utf16.Encode([]rune("Applications (*.exe)\x00*.exe\x00All files (*.*)\x00*.*\x00")), 0)
	title, err := windows.UTF16PtrFromString("选择游戏可执行文件")
	if err != nil {
		return "", false, err
	}
	fileBuffer := make([]uint16, windows.MAX_PATH)
	ofn := openFileName{
		StructSize: uint32(unsafe.Sizeof(openFileName{})),
		Owner:      owner,
		Filter:     &filter[0],
		File:       &fileBuffer[0],
		MaxFile:    uint32(len(fileBuffer)),
		Title:      title,
		Flags:      ofnFileMustExist | ofnPathMustExist | ofnNoChangeDir | ofnExplorer,
	}
	result, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if result == 0 {
		extendedError, _, _ := procCommDlgExtendedError.Call()
		if extendedError == 0 {
			return "", false, nil
		}
		return "", false, fmt.Errorf("common file dialog error 0x%X", extendedError)
	}
	path := windows.UTF16ToString(fileBuffer)
	if !strings.EqualFold(filepath.Ext(path), ".exe") {
		return "", false, errors.New("所选文件不是 .exe 可执行文件")
	}
	return path, true, nil
}

func addGameProcess(cfg *config, executablePath string) (bool, string) {
	executablePath = strings.TrimSpace(executablePath)
	if !strings.EqualFold(filepath.Ext(executablePath), ".exe") {
		return false, ""
	}
	processName := strings.ToLower(filepath.Base(executablePath))
	if processName == "" || processName == "." || processName == string(filepath.Separator) {
		return false, processName
	}
	for _, existing := range cfg.GameProcesses {
		if strings.EqualFold(strings.TrimSpace(existing), processName) {
			return false, processName
		}
	}
	cfg.GameProcesses = append(cfg.GameProcesses, processName)
	cfg.GameModeEnabled = true
	return true, processName
}

func (a *app) updateGameStatus() {
	if a.settingsGameStatus == 0 {
		return
	}
	if err := setWindowText(
		a.settingsGameStatus,
		fmt.Sprintf("游戏名单：%d 个；命中前台进程时自动暂停映射。", len(a.cfg.GameProcesses)),
	); err != nil {
		log.Printf("update game process status failed: %v", err)
	}
}

func inspectHotkeyConflicts(
	hotkeys []hotkeyConfig,
	probe func(index int, mods, key uint) (bool, error),
) (map[int]string, []string) {
	results := make(map[int]string)
	messages := make([]string, 0)
	seen := make(map[string]int)

	for i, item := range hotkeys {
		if !item.Enabled {
			results[i] = "未启用"
			continue
		}
		mods, key, err := parseCombo(item.Combo)
		if err != nil || mods != modAlt {
			results[i] = "组合无效"
			messages = append(messages, fmt.Sprintf("%s：组合无效或不支持", item.Combo))
			continue
		}
		signature := fmt.Sprintf("%d:%d", mods, key)
		if previous, ok := seen[signature]; ok {
			results[previous] = "配置重复"
			results[i] = "配置重复"
			messages = append(messages, fmt.Sprintf("%s：与配置第 %d 项重复", item.Combo, previous+1))
			continue
		}
		seen[signature] = i

		conflict, err := probe(i, mods, key)
		if err != nil {
			results[i] = "探测失败"
			messages = append(messages, fmt.Sprintf("%s：探测失败（%v）", item.Combo, err))
			continue
		}
		if conflict {
			results[i] = "系统热键冲突"
			messages = append(messages, fmt.Sprintf("%s：Windows 全局热键已占用（占用者未知）", item.Combo))
			continue
		}
		results[i] = "未发现系统冲突"
	}
	return results, messages
}

func probeWindowsRegisteredHotkey(hwnd windows.Handle, id int, mods, key uint) (bool, error) {
	result, _, callErr := procRegisterHotKey.Call(
		uintptr(hwnd),
		uintptr(id),
		uintptr(mods|modNoRepeat),
		uintptr(key),
	)
	if result == 0 {
		lastErr := windows.GetLastError()
		if errors.Is(lastErr, windows.ERROR_HOTKEY_ALREADY_REGISTERED) {
			return true, nil
		}
		return false, fmt.Errorf("RegisterHotKey failed: %w (call error: %v)", lastErr, callErr)
	}

	result, _, unregisterErr := procUnregisterHotKey.Call(uintptr(hwnd), uintptr(id))
	if result == 0 {
		return false, fmt.Errorf("UnregisterHotKey failed: %w", unregisterErr)
	}
	return false, nil
}

func (a *app) saveSettings() {
	if a.settingsSelected >= 0 && a.settingsSelected < len(a.settingsDraft) {
		item, err := a.readSettingsEntry()
		if err != nil {
			showSettingsMessage(a.settingsHwnd, err.Error(), true)
			return
		}
		a.settingsDraft[a.settingsSelected] = item
	}

	seen := make(map[string]struct{}, len(a.settingsDraft))
	for i, item := range a.settingsDraft {
		if !isSupportedAction(item.Action) {
			showSettingsMessage(a.settingsHwnd, fmt.Sprintf("第 %d 项动作不受支持。", i+1), true)
			return
		}
		mods, key, err := parseCombo(item.Combo)
		if err != nil || mods != modAlt {
			showSettingsMessage(a.settingsHwnd, fmt.Sprintf("第 %d 项快捷键无效；目前只支持 Alt+按键组合。", i+1), true)
			return
		}
		signature := fmt.Sprintf("%d:%d", mods, key)
		if _, exists := seen[signature]; exists {
			showSettingsMessage(a.settingsHwnd, fmt.Sprintf("第 %d 项快捷键与其他项目重复。", i+1), true)
			return
		}
		seen[signature] = struct{}{}
	}

	cfg := a.cfg
	cfg.Hotkeys = append([]hotkeyConfig(nil), a.settingsDraft...)
	if err := saveConfig(a.configPath, cfg); err != nil {
		showSettingsMessage(a.settingsHwnd, "保存配置失败：\n"+err.Error(), true)
		return
	}
	a.cfg = cfg
	a.registerHotkeys()
	if err := applyAutoStart(a.cfg.AutoStart); err != nil {
		showSettingsMessage(a.settingsHwnd, "快捷键配置已保存并生效，但更新开机自启动失败：\n"+err.Error(), false)
	}
	procDestroyWindow.Call(uintptr(a.settingsHwnd))
}

func settingsActionLabel(value string) string {
	for _, action := range settingsActions {
		if action.value == value {
			return action.label
		}
	}
	return value
}

func addSettingsString(hwnd windows.Handle, message uintptr, text string) {
	textPtr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	procSendMessageW.Call(uintptr(hwnd), message, 0, uintptr(unsafe.Pointer(textPtr)))
}

func setWindowText(hwnd windows.Handle, text string) error {
	textPtr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return err
	}
	procSetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(textPtr)))
	return nil
}

func getWindowText(hwnd windows.Handle) string {
	length, _, _ := procGetWindowTextLengthW.Call(uintptr(hwnd))
	buffer := make([]uint16, int(length)+1)
	if len(buffer) == 1 {
		return ""
	}
	procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	return windows.UTF16ToString(buffer)
}

func showSettingsMessage(hwnd windows.Handle, text string, isError bool) {
	message, messageErr := windows.UTF16PtrFromString(text)
	title, titleErr := windows.UTF16PtrFromString("AltCmd 设置")
	if messageErr != nil || titleErr != nil {
		return
	}
	icon := uintptr(mbIconWarning)
	if isError {
		icon = mbIconError
	}
	procMessageBoxW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), mbOK|icon)
}
