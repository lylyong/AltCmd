# AltCmd

[简体中文](README.md) | [English](README.en.md)

AltCmd is a Windows system-tray utility that maps familiar Alt-based shortcuts to common development actions.

Current version: **1.0.2**

## Features

- **System tray:** Right-click the tray icon to configure shortcuts, toggle game mode and startup behavior, show or hide the trace console, reload configuration, view About information, or exit.
- **Shortcut settings UI:** View, add, edit, enable or disable, and remove shortcuts. Changes take effect when saved. The UI uses built-in Windows controls and has no additional GUI dependencies. Supported on Windows 10 and 11.
- **Game mode:** When enabled, shortcut actions are suspended while a process on the configured game list is in the foreground. Add a game's `.exe` from the shortcut settings window to add its process name and automatically enable game mode.
- **JSON configuration:** Customize shortcut combinations and actions in a JSON configuration file.
- **Run at sign-in:** Configure startup behavior in the settings file or toggle it from the tray menu.

## Default shortcuts

- `Alt+C` -> Copy: tries `WM_COPY` first, then falls back to `Ctrl+C`
- `Alt+V` -> Paste: tries `WM_PASTE` first, then falls back to `Ctrl+V`
- `Alt+X` -> `Ctrl+X`
- `Alt+Z` -> `Ctrl+Z`
- `Alt+S` -> `Ctrl+S`
- `Alt+A` -> `Ctrl+A`
- `Alt+T` -> `Ctrl+T`
- `Alt+W` -> `Ctrl+W`
- `Alt+Q` -> `Alt+F4`
- `Alt+Backtick` -> Cycle through other windows belonging to the foreground application

## Run from source

```powershell
go mod tidy
go run .
```

AltCmd runs in the system tray; the console is used for logs. The tray menu's **Show trace console** option displays or hides the log window and is disabled by default. This setting is saved in the user configuration. If AltCmd was launched from an existing terminal, hiding the trace console only detaches AltCmd from that terminal; it does not hide or close the terminal window.

The trace console runs in a separate process. Closing its window does not exit AltCmd. You can also close or reopen it from the tray menu. Logs are written to `AltCmd.log` in the user configuration directory.

## Build

```powershell
go build -ldflags "-H=windowsgui" -o AltCmd-1.0.2.exe .
```

The Windows GUI subsystem prevents a console window from appearing at startup. The trace console is opened only when explicitly enabled from the tray menu. Run `.\AltCmd-1.0.2.exe --version` to display the version. Windows file properties also show `1.0.2.0`.

## Install and run at sign-in

Run `AltCmd-1.0.2.exe` directly to deploy without Go or the deployment script. On first launch, AltCmd creates the user configuration, installs itself to `%LOCALAPPDATA%\Programs\AltCmd\AltCmd.exe`, enables run-at-sign-in for the current user, and starts the installed application. Auto-start is enabled by default and can be disabled from the tray menu. The trace console is hidden by default.

Alternatively, run this script from the project directory in Windows PowerShell:

```powershell
.\deploy.ps1
```

The script checks the Windows and Go versions, builds AltCmd, installs it to `%LOCALAPPDATA%\Programs\AltCmd`, configures it to run when the current user signs in, and launches it.

- **Build requirements:** Go 1.23 or later must be installed on the build machine. The deployment script checks the Go version. Running `go build` directly also requires the Go toolchain.
- **Runtime requirements:** The generated `AltCmd.exe` is self-contained; the target computer does not need Go installed.
- **Windows support:** Go 1.23 officially supports Windows 10 / Windows Server 2016 and later. Accordingly, the deployment script only allows installation on these versions or later. Older Windows versions are outside the supported range of this toolchain.

Not requiring Go at runtime does not mean AltCmd runs on every older Windows version. Runtime Windows support is still limited by the Go toolchain used to build it.

To uninstall AltCmd and remove its startup entry while keeping user configuration:

```powershell
.\deploy.ps1 -Uninstall
```

AltCmd is an interactive desktop application that needs access to the current user's desktop and displays a tray icon. It therefore uses the current user's `HKCU\...\Run` registry entry to start at sign-in; the same method works on Windows 10 and 11. Windows services run in a non-interactive session and are not suitable for tray applications or a replacement for starting after user sign-in.

## Configuration

AltCmd uses `os.UserConfigDir()`. On Windows, the configuration file is typically located at:

`C:\Users\<username>\AppData\Roaming\AltCmd\config.json`

The file is created automatically on first launch. Example:

```json
{
	"game_mode_enabled": false,
	"auto_start": true,
	"show_console": false,
	"game_processes": [
		"valorant.exe",
		"cs2.exe"
	],
	"hotkeys": [
		{ "combo": "Alt+C", "action": "copy", "enabled": true },
		{ "combo": "Alt+V", "action": "paste", "enabled": true },
		{ "combo": "Alt+X", "action": "cut", "enabled": true },
		{ "combo": "Alt+Z", "action": "undo", "enabled": true },
		{ "combo": "Alt+S", "action": "save", "enabled": true },
		{ "combo": "Alt+A", "action": "select_all", "enabled": true },
		{ "combo": "Alt+T", "action": "new_tab", "enabled": true },
		{ "combo": "Alt+W", "action": "close_tab", "enabled": true },
		{ "combo": "Alt+Q", "action": "close_window", "enabled": true },
		{ "combo": "Alt+BACKTICK", "action": "switch_window", "enabled": true }
	]
}
```

Supported `action` values:

- `copy`
- `paste`
- `cut`
- `undo`
- `save`
- `select_all`
- `new_tab`
- `close_tab`
- `close_window`
- `switch_window`

Examples of `combo` values:

- `Alt+C`
- `Alt+F4`
- `Alt+BACKTICK`

The shortcut settings window currently accepts only `Alt+key` combinations. Select **Probe conflicts** to check for conflicts with Windows registered global hotkeys and mark results in the list. Windows does not provide an API to identify which process registered a hotkey. The probe also cannot detect shortcuts handled internally by applications or conflicts with other low-level keyboard hooks. An unavailable probe result does not necessarily mean that a shortcut is invalid or in conflict.

After editing the JSON file manually, select **Reload configuration** from the tray menu to apply the changes. In the settings window, select **Save and apply** to save and apply changes immediately.

Select **Add game executable...** in the shortcut settings window and choose a game's `.exe` to add its process name to the game list and enable game mode. When that game is in the foreground, Alt shortcut mappings are suspended. The list is saved in the user configuration.

`switch_window` cycles through top-level windows owned by the foreground application. It is similar to switching between windows within an application on macOS, but it is not a system-wide replacement for `Alt+Tab`.

## Startup behavior

- When `auto_start=true`, AltCmd creates this registry value:
  `HKCU\Software\Microsoft\Windows\CurrentVersion\Run\AltCmd`
- When `auto_start=false`, AltCmd removes that value.

## Notes

- Some applications do not support `WM_COPY` or `WM_PASTE`; AltCmd falls back to keyboard input.
- In terminal windows, the `Ctrl+C` copy fallback may trigger an interrupt.
- Sending keystrokes to an elevated window may require running AltCmd as administrator.

## License

This project is licensed under the [MIT License](LICENSE).
