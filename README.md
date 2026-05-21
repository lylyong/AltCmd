# AltCmd

AltCmd 是一个 Windows 常驻托盘工具，用于把常用快捷键习惯从 Alt 组合映射到常见开发动作。

## 已实现能力

- 托盘常驻：右键托盘图标可开关游戏模式、开机自启动、重载配置、退出
- 游戏模式：开启后，当前台进程匹配游戏进程名单时，自动阻断热键动作
- 配置模式：通过 JSON 配置文件自定义热键组合与动作
- 开机自启动：配置项控制，也可通过托盘菜单即时切换

## 默认动作

- `Alt+C` -> 复制：先尝试 `WM_COPY`，失败回退 `Ctrl+C`
- `Alt+V` -> 粘贴：先尝试 `WM_PASTE`，失败回退 `Ctrl+V`
- `Alt+X` -> `Ctrl+X`
- `Alt+Z` -> `Ctrl+Z`
- `Alt+S` -> `Ctrl+S`
- `Alt+A` -> `Ctrl+A`
- `Alt+T` -> `Ctrl+T`
- `Alt+W` -> `Ctrl+W`
- `Alt+Q` -> `Alt+F4`
- `Alt+反引号` -> 切换当前应用的其他窗口

## 运行

```powershell
go mod tidy
go run .
```

启动后会进入托盘常驻，控制台仅用于日志输出。

## 构建

```powershell
go build -o AltCmd.exe .
```

## 配置文件

程序使用 `os.UserConfigDir()`，在 Windows 上通常是：

`C:\Users\<用户名>\AppData\Roaming\AltCmd\config.json`

程序首次启动会自动生成该文件。

示例：

```json
{
	"game_mode_enabled": false,
	"auto_start": false,
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

支持的 `action`：

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

`combo` 语法示例：

- `Alt+C`
- `Ctrl+Alt+S`
- `Alt+F4`
- `Alt+Shift+W`
- `Alt+BACKTICK`

改完配置后可用托盘菜单 `重载配置` 立即生效。

`switch_window` 会在当前前台应用的同进程顶级窗口之间循环切换，效果接近 macOS 的应用内窗口切换；它不是系统级的 `Alt+Tab` 替代。

## 自启动说明

- `auto_start=true` 时，写入注册表：
	`HKCU\Software\Microsoft\Windows\CurrentVersion\Run\AltCmd`
- `auto_start=false` 时，删除该项

## 注意事项

- 在某些软件中，`WM_COPY/WM_PASTE` 不被支持，会回退到键盘注入。
- 复制回退到 `Ctrl+C` 时，在终端窗口可能触发中断。
- 向高权限窗口发送按键时，可能需要以管理员权限运行本程序。
