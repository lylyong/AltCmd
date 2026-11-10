# AltCmd

[简体中文](README.md) | [English](README.en.md)

AltCmd 是一个 Windows 常驻托盘工具，用于把常用快捷键习惯从 Alt 组合映射到常见开发动作。

当前版本：**1.2.3**

## 已实现能力

- 托盘常驻：右键托盘图标可打开快捷键设置、开关游戏模式、开机自启动和追踪终端显示、重载配置、查看关于信息或退出
- 图形化快捷键设置：查看、编辑、启用/禁用和删除已有快捷键，保存后立即生效；使用 Windows 内置控件，无额外 GUI 依赖，适用于 Windows 10/11
- 游戏模式：开启后，当前台进程匹配游戏进程名单时，自动阻断热键动作；可在快捷键设置窗口添加游戏 `.exe`，添加时会自动开启游戏模式
- 配置模式：通过 JSON 配置文件自定义热键组合与动作
- 开机自启动：配置项控制，也可通过托盘菜单即时切换

## 默认动作

- `Alt+C` -> 复制：先尝试 `WM_COPY`，失败回退 `Ctrl+C`
- `Alt+F` -> `Ctrl+F`（查找）
- `Alt+R` -> `Ctrl+R`（刷新/重新加载）
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
托盘菜单中的“显示追踪终端”可显示或隐藏日志控制台，默认关闭；此设置会保存到用户配置中。隐藏从已有终端启动的程序时，只会让 AltCmd 脱离该终端，不会隐藏或关闭用户正在使用的终端窗口。
追踪终端运行在独立进程中，关闭该窗口不会退出 AltCmd；也可使用托盘菜单中的“显示追踪终端”关闭或重新打开日志窗口。日志保存在用户配置目录下的 `AltCmd.log`。

## 构建

```powershell
.\build.ps1
```

构建使用 Windows GUI 子系统，启动时不会弹出控制台窗口；追踪终端仅在托盘菜单中手动开启时显示。脚本读取 `VERSION`，自动生成程序和 Windows 文件属性所需的版本信息，并输出 `AltCmd-<VERSION>.exe`。首次构建会下载固定版本的 Windows 资源生成工具。也可通过 `.\build.ps1 -OutputPath "$env:TEMP\AltCmd.exe"` 指定输出位置。

## 部署与开机启动

直接运行 `AltCmd-<VERSION>.exe` 即可完成部署，无需 Go 或部署脚本：程序会在首次启动时生成用户配置，将自身安装到固定路径 `%LOCALAPPDATA%\Programs\AltCmd\AltCmd.exe`，开启当前用户登录时自启动并启动已安装的程序。默认配置启用自启动；可通过托盘菜单关闭。追踪终端默认隐藏。

也可以在 Windows PowerShell 中从项目目录运行部署脚本：

```powershell
.\deploy.ps1
```

脚本会检测 Windows 版本和 Go 版本，在项目目录生成带版本号的 `AltCmd-<VERSION>.exe`，然后以固定名称 `AltCmd.exe` 安装到当前用户的 `%LOCALAPPDATA%\Programs\AltCmd`，设置当前用户登录时启动，并立即运行程序。
重新部署时，脚本会先关闭从该安装路径运行的 AltCmd，再替换并启动新版本；从其他路径运行的实例不会被关闭。卸载时也会先关闭安装目录中的实例。

- **构建时**：运行构建或部署脚本的电脑需要安装 Go 1.23 或更高版本；脚本会检查 Go 版本。首次构建需要网络连接以下载固定版本的 Windows 资源生成工具。

- **运行时**：生成的 `AltCmd-<VERSION>.exe` 是独立可执行文件，目标电脑不需要安装 Go；安装后的程序名固定为 `AltCmd.exe`。
- **Windows 版本**：使用 Go 1.23 构建的程序，Go 官方支持的最低 Windows 版本是 Windows 10 / Windows Server 2016。脚本也因此只允许在这些版本及更新版本上部署。更早的 Windows 版本不在此工具链的支持范围内。

因此，“运行程序不需要 Go”不等于“可以在任意旧版 Windows 上运行”：运行时 Windows 支持范围仍受构建所用 Go 工具链限制。

卸载程序并移除自启动项（保留用户配置）：

```powershell
.\deploy.ps1 -Uninstall
```

AltCmd 是需要访问当前用户桌面并显示托盘图标的交互式程序，因此使用当前用户的 `HKCU\...\Run` 注册表项在登录时启动；Windows 10 和 11 使用相同方式。Windows 服务运行在非交互会话中，不适合托盘程序，也不能替代用户登录后的启动。

## 配置文件

程序使用 `os.UserConfigDir()`，在 Windows 上通常是：

`C:\Users\<用户名>\AppData\Roaming\AltCmd\config.json`

程序首次启动会自动生成该文件。

示例：

```json
{
	"config_version": 1,
	"game_mode_enabled": false,
	"auto_start": true,
	"show_console": false,
	"game_processes": [
		"valorant.exe",
		"cs2.exe"
	],
	"hotkeys": [
		{ "combo": "Alt+C", "action": "copy", "enabled": true },
		{ "combo": "Alt+F", "action": "find", "enabled": true },
		{ "combo": "Alt+R", "action": "refresh", "enabled": true },
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
- `find`
- `refresh`
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
- `Alt+F4`
- `Alt+BACKTICK`

快捷键设置窗口目前只接受 `Alt+按键` 组合，只能编辑或删除已有快捷键，不能新增快捷键。`enabled` 是互斥的布尔状态：`true` 表示启用，`false` 表示禁用；禁用的快捷键不会注册，也不会执行。点击“探测冲突”可检查 Windows 已注册的全局热键，并在列表中标出冲突项；Windows 不提供查询占用软件名称的接口，探测也无法发现应用内部快捷键或其他低层键盘钩子冲突。手动修改 JSON 后可用托盘菜单 `重载配置` 立即生效；在图形化设置窗口点击“保存并应用”则会保存并立即生效。

旧版配置会在首次启动时自动升级：若尚未配置 `Alt+F` 或 `Alt+R`，程序会分别添加查找或刷新快捷键；已有的对应快捷键配置会保留。

快捷键设置窗口会显示当前游戏进程名单，可添加或删除项目；添加游戏时会自动开启游戏模式。名单中的游戏成为前台窗口时，Alt 快捷键映射会暂停；名单保存在用户配置中。

`switch_window` 会在当前前台应用的同进程顶级窗口之间循环切换，效果接近 macOS 的应用内窗口切换；它不是系统级的 `Alt+Tab` 替代。

## 自启动说明

- `auto_start=true` 时，写入注册表：
	`HKCU\Software\Microsoft\Windows\CurrentVersion\Run\AltCmd`
- `auto_start=false` 时，删除该项

## 注意事项

- 在某些软件中，`WM_COPY/WM_PASTE` 不被支持，会回退到键盘注入。
- 复制回退到 `Ctrl+C` 时，在终端窗口可能触发中断。
- 向高权限窗口发送按键时，可能需要以管理员权限运行本程序。

## 许可证

本项目使用 [MIT 许可证](LICENSE)。
