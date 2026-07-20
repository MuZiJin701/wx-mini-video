# wx-mini-video 基础知识

## 架构

`wx-mini-video` 是单进程 Go TUI 工具。入口 `main.go` 加载配置后进入 Bubble Tea TUI；用户输入小程序 AppID 后，`internal/app` 启动代理、维护候选资源和下载器。

核心数据流：

1. PC 微信请求经过本地代理 `127.0.0.1:2023`。
2. `internal/interceptor/miniprogram.go` 按响应 URL、Content-Type 和 JSON 响应体提取媒体候选。
3. `internal/miniprogram.Store` 按 AppID 和 URL 去重，保留缓存路径、请求头、大小和来源。
4. TUI 默认显示“视频”分类，可切换全部、图片、视频、m3u8。
5. 下载器保存图片/直链视频；m3u8 先并发缓存分片，再调用 ffmpeg 合并为 mp4。

## 配置

默认配置模板在 `internal/config/config.template.yaml`。常用项：

- `download.dir`: 下载目录，空值表示程序旁 `downloads/`。
- `download.ffmpeg`: ffmpeg 命令或路径，默认 `ffmpeg`。
- `download.mode`: `auto`、`prefetch`、`remote-ffmpeg`。
- `download.segmentConcurrency`: m3u8 分片并发数，默认 `6`。
- `proxy.system`: 是否设置 Windows 系统代理，默认 `true`。
- `target.appID`: 会话目标 AppID，启动 TUI 可输入或确认。

## 分发

基础包不内置 ffmpeg。首次下载 m3u8 时，如果本机未找到 ffmpeg，程序会自动下载便携版到下载目录。

构建命令：

```powershell
cmd /c build\build.bat windows
```

生成：

```text
dist\wx-mini-video-windows-amd64\
dist\wx-mini-video-windows-amd64.zip
```

zip 用于 GitHub Releases，不提交到源码仓库。

## 排障

- 小程序网络错误：通常是系统代理残留。重新以管理员身份运行 exe，进入后按 `q` 正常退出。
- 证书失败：以管理员身份运行，或手动安装下载目录导出的 `WxMiniVideoRoot.cer` 到“受信任的根证书颁发机构”。
- 资源太多：先按 `c` 清空，再打开目标图片或播放目标视频。
- m3u8 慢：长视频分片多，优先看 TUI 的分片数、失败数和 ffmpeg 合并进度。失败数高时把 `segmentConcurrency` 调低到 `4`。

## 当前限制

- AppID 是会话标签，不承诺自动证明每个请求真实属于该小程序。
- 只识别常见图片、直链视频和普通 VOD/回放 m3u8。
- Windows 是主要分发目标；其他系统未作为验收环境。
