# AGENTS.md

## 项目

`wx-mini-video` 是 Windows 优先、同时提供 macOS 未签名测试版的 Go TUI：通过本地 HTTPS 代理识别 PC 微信目标小程序加载的图片、直链视频、直链音频和 m3u8，并下载到本地。

当前范围是通用小程序媒体嗅探与下载。保持单二进制 TUI、Echo 代理适配、Windows 系统代理与证书、macOS 系统代理与钥匙串、目标 AppID 严格归属、手动输入与小程序发现、图片/直链视频/直链音频下载及 m3u8 的 ffmpeg 合并。Windows 主程序以当前用户运行并优先使用已安装的 Terminal；只有安装根证书或清理旧共享证书时请求 UAC。Windows 与 macOS 均只分发一个完整包，随包附带 ffmpeg（Windows `ffmpeg\ffmpeg.exe`，macOS `.app` 外 `ffmpeg/ffmpeg-amd64|arm64`）；随包缺失或损坏时自动下载受管 ffmpeg 兜底。

## 先读

- **领域术语：** `GLOSSARY.md`。
- **架构、配置、构建与排障：** `wx-mini-video-knowledge.md`。
- **用户流程或发布说明：** `README.md`；macOS 使用见 `docs/macos-usage.md`。
- **版本变更与发布合并：** `CHANGELOG.md`；发布操作见 `wx-mini-video-knowledge.md` 的“分发”。
- **ffmpeg 随包、许可与源码义务：** `THIRD-PARTY-NOTICES.md`、`docs/adr/0007-bundled-ffmpeg.md`；需求背景见 `docs/bundled-ffmpeg-requirements.md`。
- **历史计划与背景：** `docs/archive/`。
- **代码入口：** 启动看 `main.go` 和 `internal/app/`；TUI 看 `internal/tui/model.go`；嗅探看 `internal/interceptor/miniprogram.go`；下载看 `internal/minidownload/`；代理库 patch 看 `third_party/echo/`。

## 变更

- 先检查相关实现、调用方、测试和已有未提交修改；保留无关用户修改。
- 用现有模式完成最小正确改动；在共享路径修复根因。
- 用户可见行为、配置、构建或发布变化时同步更新对应文档；维护决策写入 `docs/adr/`。
- 面向用户、维护者和 agent 的项目文档使用中文；代码标识、命令和专有名词保留原文。
- `dist/`、`downloads/`、截图、海报、日志、临时 exe、ffmpeg 二进制与许可文本缓存只留本地，不加入提交；`build/licenses/` 下的 GPL 全文与根目录 `THIRD-PARTY-NOTICES.md` 例外，必须提交。
- 目标归属、来源未确认候选的显示方式与来源标记的语义见 `docs/adr/0008-target-attribution.md`；不要为了“多抓”而放宽目标候选的严格匹配。

## 验证与提交

- Go 代码修改后运行 `go test -count=1 ./...`、`go vet ./...`；修改 `third_party/echo/` 后额外运行 `go test -count=1 github.com/ltaoo/echo/...`。
- Windows 分发改动后运行 `cmd /c build\build.bat windows`（唯一分发包，含随包 ffmpeg；需联网下载并校验 ffmpeg，缓存于 `dist/ffmpeg-cache/`，由 `build/fetch-ffmpeg.ps1` 下载）；包内只可包含 `README.md`、`wx-mini-video.exe`、`wx-mini-video.yaml`、`ffmpeg\ffmpeg.exe`、`ffmpeg\LICENSE.txt`、`THIRD-PARTY-NOTICES.md`。
- macOS 分发改动后运行 `go run ./build/macos` 和 `python3 build/verify-macos.py`；通用包包含应用、配置、说明、`ffmpeg/ffmpeg-amd64`、`ffmpeg/ffmpeg-arm64`、`ffmpeg/LICENSE.txt` 与 `THIRD-PARTY-NOTICES.md`。
- 文档改动至少运行 `git diff --check`。
- 每次完成修改后提交 Git；除非用户明确要求，不推送远程仓库。
