# 微信小程序视频下载器

这是一个通用 TUI 工具，用于嗅探并下载 PC 微信小程序加载的图片和视频。

## 功能

- 启动本地 HTTPS 代理并嗅探 PC 微信小程序请求。
- 启动后输入小程序 AppID，名称可选；AppID 作为本次会话标签。
- 自动识别 `jpg`、`png`、`webp`、`gif`、`bmp`、`avif` 图片资源，以及 `mp4`、`webm`、`mov`、`m3u8` 视频资源。
- 在终端 TUI 中实时显示候选资源。
- 显示下载进度：图片和直链视频显示字节进度，`m3u8` 显示分片下载、复用、失败数和 ffmpeg 合并时长进度。
- `m3u8` 支持 `auto`、`prefetch`、`remote-ffmpeg` 三种模式；默认长视频并发预下载分片，再调用 `ffmpeg` 合并输出为 `.mp4`。
- 启动时检测 ffmpeg；Windows 下未检测到时会自动下载便携版 `ffmpeg.exe` 到下载目录。
- 退出时恢复启动前的 Windows 系统代理设置。

## 使用

开发运行：

```powershell
go run .
```

分发包运行：

1. 解压 `wx-mini-video-windows-amd64.zip`。
2. 右键 `wx-mini-video.exe`，选择“以管理员身份运行”。
3. 在 TUI 首屏输入小程序 AppID，名称可选，按 `Enter` 启动代理。
4. 在 PC 微信中打开对应小程序，打开图片或播放视频。

TUI 快捷键：

- `↑/↓` 或 `k/j`：选择资源
- `d` 或 `Enter`：下载选中资源
- `r`：刷新候选资源
- `c`：清空候选资源
- `o`：打开下载目录
- `l`：显示/隐藏日志
- `PgUp/PgDn`：翻阅日志
- `q`：退出并恢复系统代理

如果打开小程序时一次性加载出很多资源，建议先按 `c` 清空列表，再打开目标图片或播放目标视频。第一版不会自动识别每条请求真实来自哪个小程序，输入的 AppID 用作本次会话标签；请确认后只操作目标小程序。

如果程序异常退出导致网络异常，可重新以管理员身份运行本工具，然后按 `q` 正常退出以恢复本工具残留的系统代理。

## Windows 证书说明

首次运行需要安装本地代理根证书，Windows 可能弹出 UAC，企业策略也可能阻止写入系统根证书。

如果自动安装失败：

- 优先用管理员权限重新运行。
- 程序会把证书导出到下载目录：`QimingRoot.cer`，可手动安装到“受信任的根证书颁发机构”。
- 如果代理已经启动，也可以打开 `http://127.0.0.1:2023/cert` 下载证书。

## 配置

默认配置文件名为 `wx-mini-video.yaml`，也可以通过环境变量 `WX_MINI_VIDEO_CONFIG` 指定配置文件路径。

常用配置：

```yaml
download:
  dir: ""
  ffmpeg: "ffmpeg"
  mode: "auto"
  segmentConcurrency: 6
  segmentRetries: 3
  keepSegments: false

proxy:
  system: true
  hostname: "127.0.0.1"
  port: 2023
  skipInstallRootCert: false

target:
  appID: ""
  name: ""
```

`target.appID` 和 `target.name` 会预填到启动输入页；AppID 必填，名称可选。

`download.mode` 可选：

- `auto`：默认。短 m3u8 直接交给 ffmpeg，长 m3u8 先并发下载分片。
- `prefetch`：始终先并发下载分片，适合长视频或网络不稳定时重试。
- `remote-ffmpeg`：始终让 ffmpeg 远程读取 m3u8，适合很短的视频。

如果分片失败数较多，建议把 `segmentConcurrency` 从 `6` 降到 `4`；网络很好时可以尝试 `8` 或 `12`。

## 构建与发布

```powershell
build\build.bat windows
```

产物位于：

- `dist\wx-mini-video-windows-amd64\`
- `dist\wx-mini-video-windows-amd64.zip`

基础包不内置 ffmpeg，首次运行会自动下载。

`dist\` 是本机构建产物目录，不提交到 Git。对外分发时，把 `dist\wx-mini-video-windows-amd64.zip` 上传到 GitHub Releases 附件，例如：

- Release 标题：`wx-mini-video v0.1.0`
- Release 附件：`wx-mini-video-windows-amd64.zip`

旧 full 包已归档到 `dist\archive\full\`，仅用于本地保留历史版本，不作为主版本发布。

## 免责声明

本项目仅用于技术交流学习和研究。请遵守法律法规，请勿用于任何非法用途。
