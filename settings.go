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

	lbAddString       = 0x0180
	lbResetContent    = 0x0184
	lbSetCurSel       = 0x0186
	lbGetCurSel       = 0x0188
	cbAddString       = 0x0143
	cbResetContent    = 0x014B
	cbSetCurSel       = 0x014E
	cbGetCurSel       = 0x0147
	bmGetCheck        = 0x00F0
	bmSetCheck        = 0x00F1
	bstChecked        = 1
	bnClicked         = 0
	lbnSelChange      = 1
	swShow            = 5
	mbOK              = 0x00000000
	mbIconError       = 0x00000010
	mbIconInformation = 0x00000040
	wmCtlColorStatic  = 0x0138
	wmTimer           = 0x0113
	ssNotify          = 0x0100
	colorWindow       = 5
	bkModeTransparent = 1
	gmemMoveable      = 0x0002
	cfUnicodeText     = 13

	settingsListID     = 2001
	settingsComboID    = 2002
	settingsActionID   = 2003
	settingsEnableID   = 2004
	settingsUpdateID   = 2006
	settingsRemoveID   = 2007
	settingsProbeID    = 2008
	settingsSaveID     = 2009
	settingsCancelID   = 2010
	settingsGameListID = 2013
	aboutJoinGroupID   = 3001
	aboutGitHubID      = 3002
	aboutCloseID       = 3003
	aboutStatusID      = 3004
	aboutStatusTimerID = 3005
	aboutStatusTimeout = 2500
	aboutWindowWidth   = 440
	aboutWindowHeight  = 260

	modNoRepeat = 0x4000

	globalHotkeyProbeIDBase = 0x7000
	settingsAddGameID       = 2011
	settingsRemoveGameID    = 2012

	ofnFileMustExist = 0x00001000
	ofnPathMustExist = 0x00000800
	ofnNoChangeDir   = 0x00000008
	ofnExplorer      = 0x00080000
)

var (
	settingsUser32           = windows.NewLazySystemDLL("user32.dll")
	settingsComdlg32         = windows.NewLazySystemDLL("comdlg32.dll")
	settingsGdi32            = windows.NewLazySystemDLL("gdi32.dll")
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
	procGetDlgCtrlID         = settingsUser32.NewProc("GetDlgCtrlID")
	procGetSysColorBrush     = settingsUser32.NewProc("GetSysColorBrush")
	procGetSystemMetrics     = settingsUser32.NewProc("GetSystemMetrics")
	procSetTimer             = settingsUser32.NewProc("SetTimer")
	procKillTimer            = settingsUser32.NewProc("KillTimer")
	procOpenClipboard        = settingsUser32.NewProc("OpenClipboard")
	procEmptyClipboard       = settingsUser32.NewProc("EmptyClipboard")
	procSetClipboardData     = settingsUser32.NewProc("SetClipboardData")
	procCloseClipboard       = settingsUser32.NewProc("CloseClipboard")
	procSetBkMode            = settingsGdi32.NewProc("SetBkMode")
	procSetTextColor         = settingsGdi32.NewProc("SetTextColor")
	procGlobalAlloc          = kernel32.NewProc("GlobalAlloc")
	procGlobalLock           = kernel32.NewProc("GlobalLock")
	procGlobalUnlock         = kernel32.NewProc("GlobalUnlock")
	procGlobalFree           = kernel32.NewProc("GlobalFree")
	procGetOpenFileNameW     = settingsComdlg32.NewProc("GetOpenFileNameW")
	procCommDlgExtendedError = settingsComdlg32.NewProc("CommDlgExtendedError")
	settingsWndProcCallback  = windows.NewCallback(settingsWindowProc)

	settingsClassRegistered bool
	aboutClassRegistered    bool
	aboutWindowBrush        windows.Handle
	aboutWndProcCallback    = windows.NewCallback(aboutWindowProc)
	settingsActions         = []struct {
		value string
		label string
	}{
		{hotkeyActionCopy, "复制"},
		{hotkeyActionFind, "查找"},
		{hotkeyActionRefresh, "刷新"},
		{hotkeyActionPaste, "粘贴"},
		{hotkeyActionCut, "剪切"},
		{hotkeyActionUndo, "撤销"},
		{hotkeyActionSave, "保存"},
		{hotkeyActionSelectAll, "全选"},
		{hotkeyActionNew, "新建"},
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
	if _, err = createSettingsControl("BUTTON", "更新选中项", wsChild|wsVisible|wsTabStop, 325, 260, 115, 32, settingsUpdateID, 0); err != nil {
		return err
	}
	if _, err = createSettingsControl("BUTTON", "删除选中项", wsChild|wsVisible|wsTabStop, 325, 305, 125, 32, settingsRemoveID, 0); err != nil {
		return err
	}
	if _, err = createSettingsControl("BUTTON", "探测冲突", wsChild|wsVisible|wsTabStop, 465, 305, 120, 32, settingsProbeID, 0); err != nil {
		return err
	}
	if _, err = createSettingsControl("STATIC", "更新项目后点“探测冲突”；保存后立即生效。", wsChild|wsVisible, 325, 350, 330, 38, 0, 0); err != nil {
		return err
	}
	a.settingsGameStatus, err = createSettingsControl("STATIC", "", wsChild|wsVisible, 325, 390, 330, 22, 0, 0)
	if err != nil {
		return err
	}
	a.settingsGameList, err = createSettingsControl("LISTBOX", "", wsChild|wsVisible|wsBorder|wsVScroll|wsHScroll|wsTabStop|lbsHasStrings, 325, 415, 330, 65, settingsGameListID, 0)
	if err != nil {
		return err
	}
	if _, err = createSettingsControl("BUTTON", "添加游戏程序...", wsChild|wsVisible|wsTabStop, 325, 485, 145, 30, settingsAddGameID, 0); err != nil {
		return err
	}
	if _, err = createSettingsControl("BUTTON", "删除选中游戏", wsChild|wsVisible|wsTabStop, 480, 485, 145, 30, settingsRemoveGameID, 0); err != nil {
		return err
	}
	if _, err = createSettingsControl("BUTTON", "保存并应用", wsChild|wsVisible|wsTabStop, 435, 520, 115, 34, settingsSaveID, 0); err != nil {
		return err
	}
	if _, err = createSettingsControl("BUTTON", "取消", wsChild|wsVisible|wsTabStop, 565, 520, 90, 34, settingsCancelID, 0); err != nil {
		return err
	}
	a.updateGameStatus()
	a.refreshGameProcessList()
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
			case settingsUpdateID:
				appState.updateSettingsEntry()
			case settingsRemoveID:
				appState.removeSettingsEntry()
			case settingsProbeID:
				appState.probeSettingsConflicts()
			case settingsAddGameID:
				appState.addGameExecutable()
			case settingsRemoveGameID:
				appState.removeSelectedGameProcess()
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
			appState.settingsGameList = 0
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
		conflict, err := probeWindowsRegisteredHotkey(a.hwnd, globalHotkeyProbeIDBase+index, mods, key)
		if err != nil {
			log.Printf("probe global hotkey %s failed: %v", a.settingsDraft[index].Combo, err)
		}
		return conflict, err
	})
	a.settingsConflicts = conflicts
	a.refreshSettingsList()
	if len(messages) == 0 {
		showSettingsMessage(a.settingsHwnd, "未发现可探测的 Windows 全局热键冲突。\n此探测无法检查应用内部快捷键或其他低层键盘钩子。", false)
		return
	}
	showSettingsMessage(
		a.settingsHwnd,
		"探测结果：\n"+strings.Join(messages, "\n")+"\n\n“无法探测”不代表配置错误或存在冲突。Windows 不提供全局热键占用进程的查询接口，因此无法显示具体占用软件。",
		false,
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
	cfg.GameProcesses = append([]string(nil), a.cfg.GameProcesses...)
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
	a.refreshGameProcessList()
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

func (a *app) removeSelectedGameProcess() {
	selected, _, _ := procSendMessageW.Call(uintptr(a.settingsGameList), lbGetCurSel, 0, 0)
	index := int(int32(selected))
	cfg := a.cfg
	cfg.GameProcesses = append([]string(nil), a.cfg.GameProcesses...)
	processName, removed := removeGameProcess(&cfg, index)
	if !removed {
		showSettingsMessage(a.settingsHwnd, "请先选择要删除的游戏程序。", true)
		return
	}
	if err := saveConfig(a.configPath, cfg); err != nil {
		showSettingsMessage(a.settingsHwnd, "保存游戏名单失败：\n"+err.Error(), true)
		return
	}

	a.cfg = cfg
	a.updateGameStatus()
	a.refreshGameProcessList()
	showSettingsMessage(a.settingsHwnd, processName+" 已从游戏名单中删除。", false)
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

func removeGameProcess(cfg *config, index int) (string, bool) {
	if index < 0 || index >= len(cfg.GameProcesses) {
		return "", false
	}
	processName := cfg.GameProcesses[index]
	cfg.GameProcesses = append(cfg.GameProcesses[:index], cfg.GameProcesses[index+1:]...)
	return processName, true
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

func (a *app) refreshGameProcessList() {
	if a.settingsGameList == 0 {
		return
	}
	procSendMessageW.Call(uintptr(a.settingsGameList), lbResetContent, 0, 0)
	for _, processName := range a.cfg.GameProcesses {
		addSettingsString(a.settingsGameList, lbAddString, processName)
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
			results[i] = "无法探测"
			messages = append(messages, fmt.Sprintf("%s：暂时无法确认是否冲突", item.Combo))
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
		if isHotkeyAlreadyRegistered(callErr) {
			return true, nil
		}
		return false, fmt.Errorf("RegisterHotKey failed: %w", callErr)
	}

	result, _, unregisterErr := procUnregisterHotKey.Call(uintptr(hwnd), uintptr(id))
	if result == 0 {
		return false, fmt.Errorf("UnregisterHotKey failed: %w", unregisterErr)
	}
	return false, nil
}

func isHotkeyAlreadyRegistered(err error) bool {
	return errors.Is(err, windows.ERROR_HOTKEY_ALREADY_REGISTERED)
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
	icon := uintptr(mbIconInformation)
	if isError {
		icon = mbIconError
	}
	procMessageBoxW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), mbOK|icon)
}

func showAboutDialog(hwnd windows.Handle) error {
	if appState == nil {
		return errors.New("application is not initialized")
	}
	if appState.aboutHwnd != 0 {
		procShowWindow.Call(uintptr(appState.aboutHwnd), swShow)
		procSetForegroundWindow.Call(uintptr(appState.aboutHwnd))
		return nil
	}
	if err := registerAboutWindowClass(); err != nil {
		return err
	}

	className, err := windows.UTF16PtrFromString("AltCmdAboutWindow")
	if err != nil {
		return fmt.Errorf("format about window class failed: %w", err)
	}
	title, err := windows.UTF16PtrFromString("关于 AltCmd")
	if err != nil {
		return fmt.Errorf("format about window title failed: %w", err)
	}
	hInstance, _, _ := procGetModuleHandleW.Call(0)
	screenWidth, _, _ := procGetSystemMetrics.Call(0)
	screenHeight, _, _ := procGetSystemMetrics.Call(1)
	windowX := (int(screenWidth) - aboutWindowWidth) / 2
	windowY := (int(screenHeight) - aboutWindowHeight) / 2
	window, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsCaption|wsSysMenu|wsVisible,
		uintptr(windowX), uintptr(windowY), aboutWindowWidth, aboutWindowHeight,
		uintptr(hwnd), 0, hInstance, 0,
	)
	if window == 0 {
		return fmt.Errorf("CreateWindowExW(about) failed: %w", callErr)
	}
	appState.aboutHwnd = windows.Handle(window)
	if err := createAboutControls(appState.aboutHwnd); err != nil {
		procDestroyWindow.Call(window)
		return err
	}
	procSetForegroundWindow.Call(window)
	return nil
}

func registerAboutWindowClass() error {
	if aboutClassRegistered {
		return nil
	}
	className, err := windows.UTF16PtrFromString("AltCmdAboutWindow")
	if err != nil {
		return fmt.Errorf("format about window class failed: %w", err)
	}
	hInstance, _, _ := procGetModuleHandleW.Call(0)
	brush, _, _ := procGetSysColorBrush.Call(colorWindow)
	if brush == 0 {
		return errors.New("GetSysColorBrush(COLOR_WINDOW) failed")
	}
	aboutWindowBrush = windows.Handle(brush)
	wc := wndclassex{
		CbSize:        uint32(unsafe.Sizeof(wndclassex{})),
		LpfnWndProc:   aboutWndProcCallback,
		HInstance:     windows.Handle(hInstance),
		HIcon:         loadAppIconHandle(),
		HIconSm:       loadAppIconHandle(),
		HbrBackground: aboutWindowBrush,
		LpszClassName: className,
	}
	result, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if result == 0 {
		const errClassAlreadyExists syscall.Errno = 1410
		if !errors.Is(callErr, errClassAlreadyExists) {
			return fmt.Errorf("RegisterClassExW(about) failed: %w", callErr)
		}
	}
	aboutClassRegistered = true
	return nil
}

func createAboutControls(parent windows.Handle) error {
	if _, err := createAboutControl(parent, "STATIC", fmt.Sprintf("AltCmd %s", applicationVersion), wsChild|wsVisible, 24, 24, 380, 30, 0); err != nil {
		return err
	}
	if _, err := createAboutControl(parent, "STATIC", "Windows 常驻托盘快捷键工具", wsChild|wsVisible, 24, 62, 380, 24, 0); err != nil {
		return err
	}
	if _, err := createAboutControl(parent, "STATIC", "QQ群：1102396065（点击复制）", wsChild|wsVisible|wsTabStop|ssNotify, 24, 110, 340, 26, aboutJoinGroupID); err != nil {
		return err
	}
	if _, err := createAboutControl(parent, "STATIC", "github.com/lylyong/AltCmd", wsChild|wsVisible|wsTabStop|ssNotify, 24, 150, 340, 26, aboutGitHubID); err != nil {
		return err
	}
	status, err := createAboutControl(parent, "STATIC", "", wsChild|wsVisible, 24, 190, 280, 30, aboutStatusID)
	if err != nil {
		return err
	}
	if appState != nil {
		appState.aboutStatus = status
	}
	if _, err := createAboutControl(parent, "BUTTON", "关闭", wsChild|wsVisible|wsTabStop, 330, 190, 80, 30, aboutCloseID); err != nil {
		return err
	}
	return nil
}

func createAboutControl(parent windows.Handle, className, text string, style uint32, x, y, width, height, id int) (windows.Handle, error) {
	classPtr, err := windows.UTF16PtrFromString(className)
	if err != nil {
		return 0, fmt.Errorf("format about control class failed: %w", err)
	}
	textPtr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return 0, fmt.Errorf("format about control text failed: %w", err)
	}
	hInstance, _, _ := procGetModuleHandleW.Call(0)
	control, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(classPtr)),
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(style),
		uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		uintptr(parent), uintptr(id), hInstance, 0,
	)
	if control == 0 {
		return 0, fmt.Errorf("create about %s control failed: %w", className, callErr)
	}
	return windows.Handle(control), nil
}

func aboutWindowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	if appState == nil {
		return 0
	}
	switch message {
	case wmCtlColorStatic:
		controlID, _, _ := procGetDlgCtrlID.Call(lParam)
		textColor := uintptr(0x0028201A)
		if controlID == aboutJoinGroupID || controlID == aboutGitHubID {
			textColor = 0x00D76B1F
		}
		procSetTextColor.Call(wParam, textColor)
		procSetBkMode.Call(wParam, bkModeTransparent)
		return uintptr(aboutWindowBrush)
	case wmTimer:
		if wParam == aboutStatusTimerID {
			procKillTimer.Call(hwnd, aboutStatusTimerID)
			setWindowText(windows.Handle(appState.aboutStatus), "")
		}
		return 0
	case wmCommand:
		controlID := int(uint16(wParam & 0xFFFF))
		notification := uint16((wParam >> 16) & 0xFFFF)
		if notification == bnClicked {
			switch controlID {
			case aboutJoinGroupID:
				if err := copyTextToClipboard(hwnd, "1102396065"); err != nil {
					setAboutStatus("复制失败：" + err.Error())
				} else {
					setAboutStatus("群号已复制到剪贴板")
				}
			case aboutGitHubID:
				if err := openURL(hwnd, "https://github.com/lylyong/AltCmd"); err != nil {
					showSettingsMessage(windows.Handle(hwnd), "打开 GitHub 页面失败：\n"+err.Error(), true)
				}
			case aboutCloseID:
				procDestroyWindow.Call(hwnd)
			}
		}
		return 0
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		if appState.aboutHwnd == windows.Handle(hwnd) {
			appState.aboutHwnd = 0
			appState.aboutStatus = 0
		}
		return 0
	default:
		result, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
		return result
	}
}

func setAboutStatus(text string) {
	if appState == nil || appState.aboutStatus == 0 {
		return
	}
	if err := setWindowText(appState.aboutStatus, text); err != nil {
		log.Printf("update about status failed: %v", err)
		return
	}
	procKillTimer.Call(uintptr(appState.aboutHwnd), aboutStatusTimerID)
	timer, _, timerErr := procSetTimer.Call(uintptr(appState.aboutHwnd), aboutStatusTimerID, aboutStatusTimeout, 0)
	if timer == 0 {
		log.Printf("start about status timer failed: %v", timerErr)
	}
}

func copyTextToClipboard(hwnd uintptr, text string) (resultErr error) {
	opened, _, openErr := procOpenClipboard.Call(hwnd)
	if opened == 0 {
		return fmt.Errorf("OpenClipboard failed: %w", openErr)
	}
	defer func() {
		closed, _, closeErr := procCloseClipboard.Call()
		if closed == 0 && resultErr == nil {
			resultErr = fmt.Errorf("CloseClipboard failed: %w", closeErr)
		}
	}()

	empty, _, emptyErr := procEmptyClipboard.Call()
	if empty == 0 {
		return fmt.Errorf("EmptyClipboard failed: %w", emptyErr)
	}
	units := append(utf16.Encode([]rune(text)), 0)
	size := uintptr(len(units)) * unsafe.Sizeof(uint16(0))
	memory, _, allocErr := procGlobalAlloc.Call(gmemMoveable, size)
	if memory == 0 {
		return fmt.Errorf("GlobalAlloc failed: %w", allocErr)
	}
	clipboardMemory := windows.Handle(memory)
	locked, _, lockErr := procGlobalLock.Call(memory)
	if locked == 0 {
		procGlobalFree.Call(memory)
		return fmt.Errorf("GlobalLock failed: %w", lockErr)
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(locked)), len(units)), units)
	procGlobalUnlock.Call(memory)

	set, _, setErr := procSetClipboardData.Call(cfUnicodeText, memory)
	if set == 0 {
		procGlobalFree.Call(uintptr(clipboardMemory))
		return fmt.Errorf("SetClipboardData failed: %w", setErr)
	}
	return nil
}

func openURL(hwnd uintptr, url string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return fmt.Errorf("format shell operation failed: %w", err)
	}
	link, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return fmt.Errorf("format URL failed: %w", err)
	}
	result, _, _ := procShellExecuteW.Call(
		hwnd,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(link)),
		0, 0, swShow,
	)
	if result <= 32 {
		return fmt.Errorf("Windows could not open URL (ShellExecuteW result %d)", result)
	}
	return nil
}
