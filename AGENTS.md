# AGENTS.md

## 项目定位

本项目是 `wx-mini-video`：Windows 优先的微信小程序媒体嗅探与下载 TUI。当前目标是通用小程序场景，启动时输入 AppID，程序通过本地 HTTPS 代理识别 PC 微信小程序加载的图片、直链视频和 m3u8，并下载到本地。

## 当前边界

- 保留：Go TUI、Echo 代理适配、系统代理设置/恢复、证书安装/导出、媒体候选识别、图片/直链视频下载、m3u8 分片预下载和 ffmpeg 合并。
- 不保留：旧视频号下载、旧启明星专用逻辑、Web 服务、Docker 发布路径、bat/ps1 启动器、内置 ffmpeg 的默认分发包。
- 分发：源码仓库不提交 `dist/` 和 zip。Windows zip 由 `build/build.bat windows` 生成后上传 GitHub Releases。

## 证据入口

- 用户文档：`README.md`
- 稳定知识：`wx-mini-video-knowledge.md`
- 当前计划：`docs/superpowers/plans/2026-07-20-repository-slimming-and-optimization.md`
- 历史文档：`docs/archive/`
- 核心入口：`main.go`
- TUI：`internal/tui/model.go`
- 代理嗅探：`internal/interceptor/miniprogram.go`
- 下载器：`internal/minidownload/downloader.go`

## 开发规则

- 修改前先检查现有代码、配置和测试；不凭记忆改行为。
- 保持项目精简，不新增未被当前验收使用的抽象或兼容层。
- 不提交 `dist/`、`downloads/`、截图、海报、日志、临时 exe。
- 每次完成修改后运行与风险相称的验证，并提交 Git。
- 不主动推送远程仓库；只有用户明确要求时才 push。

## 验证命令

```powershell
go test -count=1 ./...
go vet ./...
cmd /c build\build.bat windows
```

构建产物必须只包含：

```text
README.md
wx-mini-video.exe
wx-mini-video.yaml
```
