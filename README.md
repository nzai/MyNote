# ClipNote

ClipNote 是一个常驻 Windows 系统托盘的轻量剪贴板窗口工具。每次点击托盘图标时，它捕获当时的文本或图片，立即新建一个独立窗口，并在窗口完成首次显示后把原始快照异步写入临时缓存。

## 功能

- 常驻托盘。左键点击打开当前剪贴板，右键点击显示菜单。
- 同一份剪贴板内容可以重复打开；打开 N 次就产生 N 个窗口和 N 条缓存记录。
- 文本窗口可编辑，图片窗口按比例缩放显示且只读。
- 窗口没有菜单栏：标题栏从左到右依次是应用图标、标题、紧贴标题的保存图标；关闭图标固定在右上角。
- 第一次保存弹出 Windows 原生“另存为”对话框，之后直接覆盖当前文件。
- 后台缓存不参与窗口创建、首次绘制或后续编辑。
- 单实例运行。再次启动程序时，会把“打开当前剪贴板”请求转发给已运行的实例。

## 构建

要求 Windows 10/11 x64 和 Go 1.26 或更高版本。项目不使用 CGo，不需要安装 GCC。

```powershell
.\scripts\build.ps1
```

默认生成 `bin\clipnote.exe`，使用 Windows GUI 子系统，不会额外打开控制台窗口。

如果本机 PowerShell 执行策略禁止运行脚本，可对本次命令临时放开，不修改系统策略：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\build.ps1
```

指定输出路径：

```powershell
.\scripts\build.ps1 -Output .\bin\clipnote-test.exe
```

生成带控制台的调试版本：

```powershell
.\scripts\build.ps1 -DebugBuild
```

也可以直接构建：

```powershell
go build -buildvcs=false -trimpath -ldflags "-s -w -H=windowsgui" -o .\bin\clipnote.exe .\cmd\clipnote
```

运行：

```powershell
.\bin\clipnote.exe
```

## 使用

### 托盘

- 左键单击：捕获当前剪贴板的文本或图片并新建窗口。
- 右键单击：打开“打开当前剪贴板”“打开缓存文件夹”“清空临时缓存”“退出”菜单。
- 关闭所有剪贴板窗口不会退出程序；需要从托盘菜单选择“退出”。

### 保存

- 单击标题右侧的保存图标或按 `Ctrl+S`：保存。首次保存打开原生“另存为”对话框，之后覆盖当前文件。
- `Ctrl+Shift+S` 或 `Shift+单击保存图标`：另存为。
- `Ctrl+W` 或 `Alt+F4`：关闭当前窗口。
- 文本有未保存修改时，关闭窗口会询问“保存 / 不保存 / 取消”。
- 文本按 UTF-8 无 BOM 保存，换行使用 CRLF；图片保存为 PNG。

## 本地数据

默认路径：

```text
%LOCALAPPDATA%\ClipNote\cache\
%LOCALAPPDATA%\ClipNote\config.json
```

日志默认写入程序同目录的 `logs\`。如果该目录不可写，会回退到：

```text
%LOCALAPPDATA%\ClipNote\logs\
```

实际使用的路径可通过以下命令查看：

```powershell
.\bin\clipnote.exe --check
```

临时缓存保留 7 天，总量不超过 500 MiB；先删除过期记录，仍超限时从最旧记录开始清理。缓存和应用日志都不做加密，缓存中是剪贴板原文或图片，请注意隐私。

## 配置与诊断

配置文件不存在时使用默认值。配置示例：

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
    "progress_delay_ms": 3000
  },
  "log": {
    "level": "info",
    "debug_scopes": []
  }
}
```

按范围打开 debug 日志：

```powershell
.\bin\clipnote.exe --debug=cache,ui,clipboard
```

日志为 JSON Lines，默认 `info`。日志不会写入剪贴板正文或图片内容。缓存和保存操作超过 3 秒后，状态栏才显示进度与取消入口。

## 范围与限制

- 只捕获点击托盘时那一刻的内容，不监听全局剪贴板变化。
- 不支持富文本、HTML、文件拖放列表或图片标注。
- 不提供安装器、开机自启动、云同步或自动更新。
- 文本窗口面向常见剪贴板内容，默认上限为 32 MiB；它不是大文件编辑器，不承诺打开或编辑 2 GiB 级别的文本文件。

## 验证

自动化验证：

```powershell
go test ./...
go vet ./...
.\scripts\build.ps1
```

完整的人工验收步骤见 [docs/TESTING.md](docs/TESTING.md)。
