# ClipNote

ClipNote 是一个常驻 Windows 系统托盘的轻量剪贴板窗口工具。点击托盘图标时，它会读取当时的文本或图片，立即新建一个独立窗口，并在窗口完成首次显示后把内容异步写入本地缓存。

项目使用 Go 和 Windigo 编写，不依赖 CGo、安装器或外部运行时，程序目录可随 EXE 一起携带。

## 主要功能

- **常驻托盘**：左键点击新建并粘贴，右键点击打开分组菜单。
- **独立窗口**：每次点击托盘都会创建独立窗口和缓存记录；同一内容点击 N 次就得到 N 个窗口。
- **文本编辑**：自动换行、逻辑行号、字体和字号调整、1.0× / 1.25× / 1.5× / 2.0× 行距。
- **图片查看**：图片按比例缩放且只读，保存时输出 PNG。
- **内容标题**：窗口和任务栏显示剪贴板内容预览；图片显示“（图片）宽 × 高”。
- **快速保存**：标题栏保存按钮或 `Ctrl+S` 保存当前内容；首次保存使用 Windows 原生“另存为”对话框。
- **编辑选项**：可实时预览字体、字号和行距，设置行号、关闭提醒和随 Windows 启动。
- **异步缓存**：窗口先显示，缓存写入在后台执行，不阻塞首次显示或后续编辑。
- **单实例运行**：重复启动不会创建第二个托盘实例，而是通知已运行实例打开当前剪贴板。
- **便携数据**：配置、缓存和日志默认放在 `clipnote.exe` 同目录。

## 快速开始

### 使用发布版

1. 从 [GitHub Releases](https://github.com/nzai/MyNote/releases/latest) 下载 `clipnote-windows-amd64.exe`。
2. 将 EXE 放到一个可写目录，例如 `D:\Tools\ClipNote\clipnote.exe`。
3. 双击运行，或在命令提示符中执行：

```bat
clipnote.exe
```

启动后不会自动弹出一个空白主窗口；请查看 Windows 右下角通知区域，点击 ClipNote 托盘图标即可新建剪贴板窗口。

### 从源码构建

要求 Windows 10/11 x64、Go 1.26 或更高版本。项目不使用 CGo，因此不需要安装 GCC。

在命令提示符中执行：

```bat
scripts\build.cmd
```

默认生成 `bin\clipnote.exe`，使用 Windows GUI 子系统，启动时不会附带控制台窗口。

生成带控制台的调试版本：

```bat
scripts\build.cmd -ConsoleBuild
```

指定输出路径或版本号：

```bat
scripts\build.cmd -Output bin\clipnote-test.exe
scripts\build.cmd -Version v1.0.0
```

`-DebugBuild` 和 `-Debug` 是 `-ConsoleBuild` 的兼容别名。构建脚本会检查生成的 PE 子系统，GUI 和 console 类型不符合预期时会直接失败。

## 日常使用

### 托盘菜单

- **左键单击**：新建并粘贴当前剪贴板。
- **右键单击**：打开菜单。
- **新建并粘贴**：读取当前剪贴板并创建独立窗口。
- **清空剪贴板**：清空 Windows 系统剪贴板，不关闭已经打开的 ClipNote 窗口。
- **打开缓存文件夹**：在资源管理器中打开便携目录下的 `cache`。
- **清空缓存**：确认后删除 ClipNote 的临时缓存，不影响手动保存到其他位置的文件。
- **退出**：退出整个程序；如果仍有未保存内容，会按当前设置询问是否保存。

关闭所有剪贴板窗口不会退出程序。ClipNote 会继续驻留在托盘，只有选择“退出”才会结束。

### 剪贴板捕获

- 文本以 Unicode 文本读取。
- 图片支持 Windows DIBV5 和 DIB 格式。
- 剪贴板为空、只有不支持的格式，或内容无法读取时，会创建一个空白文本窗口，方便直接开始输入。
- 点击托盘时只读取一次剪贴板，不会持续监听后续变化。

### 窗口和保存

窗口标题栏从左到右依次显示应用图标、内容预览、保存按钮、选项按钮；关闭按钮固定在右上角。

- 标题包含 `*` 表示文本有未保存修改。
- 单击保存按钮或按 `Ctrl+S`：保存当前内容。首次保存弹出 Windows 原生“另存为”对话框，之后覆盖当前文件。
- `Shift+单击保存按钮` 或 `Ctrl+Shift+S`：执行“另存为”。
- `Ctrl+W` 或 `Alt+F4`：关闭当前窗口。
- 文本以 UTF-8 无 BOM、CRLF 换行保存。
- 图片保存为 PNG。

### 编辑选项

单击标题栏中的选项按钮打开“编辑选项”模式窗口。设置修改会立即预览到背后的文本窗口：

- **字体**：从系统已安装字体中选择。
- **字号**：通过滑块在 8–48 pt 之间调整。
- **行间距**：可拖动选择 1.0×、1.25×、1.5× 或 2.0×。
- **显示行号**：控制文本编辑区左侧行号。
- **关闭窗口时不提醒保存**：勾选后，关闭有未保存修改的窗口不再弹出确认，未保存内容会直接丢失。
- **随 Windows 启动**：将当前 EXE 路径写入当前用户的自动启动项，不需要管理员权限。

点击选项窗口中的“保存”才会写入设置并关闭；点击“取消”或按 `Esc` 会恢复打开选项前的预览状态。选项窗口中的“保存”保存的是设置，不是当前剪贴板内容。

自动启动项写入：

```text
HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run
```

值名为 `ClipNote`。如果移动了 EXE，需要在新位置重新勾选并保存一次，自动启动路径才会更新。

### 快捷键

| 快捷键 | 功能 |
| --- | --- |
| `Ctrl+A` | 全选当前文本框内容 |
| `Ctrl+S` | 保存当前内容 |
| `Ctrl+Shift+S` | 另存为 |
| `Ctrl+W` | 关闭当前窗口 |
| `Alt+F4` | 关闭当前窗口 |

## 缓存和本地数据

ClipNote 使用便携模式，所有运行数据都在 `clipnote.exe` 所在目录：

```text
clipnote.exe
config.json
cache\
logs\
```

- `config.json`：界面和限制配置，首次保存设置或手动创建后出现。
- `cache\`：每次点击托盘产生的临时内容记录。
- `logs\`：JSON Lines 格式的运行日志。

默认缓存策略：

- 保留最近 7 天。
- 总量不超过 500 MiB。
- 先清理过期记录，仍超限时从最旧记录开始清理。
- 缓存写入发生在窗口完成首次显示之后。

将整个程序目录复制到另一个位置即可连同设置和缓存迁移。程序目录必须可写；如果不可写，ClipNote 会报告错误并拒绝启动，不会回退到 `%LOCALAPPDATA%`。

缓存和日志没有加密，可能包含剪贴板正文或图片。处理敏感内容后，可从托盘菜单使用“清空剪贴板”和“清空缓存”。

## 配置

配置文件不存在时使用默认值。示例：

```json
{
  "cache": {
    "max_age_days": 7,
    "max_bytes": 524288000
  },
  "capture": {
    "max_text_bytes": 33554432,
    "max_image_bytes": 134217728
  },
  "ui": {
    "progress_delay_ms": 3000,
    "editor": {
      "font_family": "Microsoft YaHei UI",
      "font_point_size": 11,
      "line_spacing": 125,
      "show_line_numbers": true,
      "suppress_close_prompt": false
    }
  },
  "log": {
    "level": "info",
    "debug_scopes": []
  }
}
```

`line_spacing` 支持 `100`、`125`、`150`、`200`。直接编辑配置文件后需要重启 ClipNote；通过选项窗口保存则会立即应用。

## 命令行和诊断

查看版本、实际数据路径和当前限制：

```bat
clipnote.exe --check
```

按范围打开 debug 日志：

```bat
clipnote.exe --debug=cache,ui,clipboard
```

日志默认为 `info` 级别，按范围输出，不记录剪贴板正文或图片二进制内容。缓存或保存操作超过 `ui.progress_delay_ms` 后，窗口状态栏才显示进度和取消入口，默认阈值为 3000 毫秒。

## GitHub Release

`.github/workflows/release.yml` 在发布 GitHub Release 时使用 Windows runner 执行：

1. `go test ./...`
2. `go vet ./...`
3. 构建便携版 GUI EXE，并注入 Release tag 作为程序版本
4. 生成 SHA-256 校验文件
5. 上传以下 assets：

```text
clipnote-windows-amd64.exe
SHA256SUMS.txt
```

GitHub 自动生成的源码 ZIP 和 TAR 会继续保留。Release 事件只会使用仓库默认分支上的工作流文件，因此发布前需要先把该工作流合入默认分支。

## 范围与限制

- 只捕获点击托盘时那一刻的剪贴板，不提供剪贴板历史监听。
- 不支持富文本、HTML、文件拖放列表或图片标注。
- 不提供安装器、云同步或自动更新。
- 默认文本上限为 32 MiB，图片上限为 128 MiB。
- 文本窗口面向常见剪贴板内容，不是大文件编辑器，不承诺打开或编辑 2 GiB 级文本文件。

## 开发与验证

在仓库根目录的命令提示符中运行：

```bat
set "GOCACHE=%CD%\.work\gocache"
go test ./... -count=1 -timeout=180s
go vet ./...
scripts\build.cmd
```

Win32 窗口测试可通过以下环境变量启用：

```bat
set "CLIPNOTE_GUI_TEST=1"
go test ./internal/ui -count=1
```

完整的人工验收步骤见 [docs/TESTING.md](docs/TESTING.md)。

## 许可证

[MIT](LICENSE)
