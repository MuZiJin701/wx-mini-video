# AGENTS.md

## 项目

`wx-mini-video` 是 Windows 优先、同时提供 macOS 未签名测试版的 Go TUI：通过本地 HTTPS 代理识别 PC 微信目标小程序加载的图片、直链视频、直链音频、m3u8 和 HTTP-FLV 直播流，并下载到本地。

保持单二进制 TUI、Echo 代理适配、Windows 系统代理与证书、macOS 系统代理与钥匙串、目标 AppID 严格归属、手动输入与小程序发现、直链与 HTTP-FLV 直播录制、m3u8 的 ffmpeg 合并。Windows 主程序以当前用户运行并优先使用已安装的 Terminal，仅证书安装/旧证书清理请求 UAC。Windows 与 macOS 各只分发一个完整包，随包附带 ffmpeg（Windows `ffmpeg\ffmpeg.exe`，macOS `.app` 外 `ffmpeg/ffmpeg-amd64|arm64`），缺失或损坏时自动下载受管 ffmpeg 兜底。

## 先读

- **术语：** `GLOSSARY.md`。
- **实现、配置、构建、排障与验收：** `wx-mini-video-knowledge.md`。
- **用户流程：** `README.md`；macOS 见 `docs/macos-usage.md`。**版本历史：** `CHANGELOG.md`。
- **ffmpeg 许可与源码义务：** `THIRD-PARTY-NOTICES.md`、`build/ffmpeg/README.md`、[ADR 0007](docs/adr/0007-bundled-ffmpeg.md)、[ADR 0010](docs/adr/0010-source-built-ffmpeg.md)。
- **关键决策：** [ADR 0008](docs/adr/0008-target-attribution.md) 归属语义、[ADR 0009](docs/adr/0009-user-scoped-windows-runtime.md) 与 [ADR 0004](docs/adr/0004-windows-terminal-startup.md) Windows 启动/提权、[ADR 0011](docs/adr/0011-single-main-surface-ui.md) UI、[ADR 0012](docs/adr/0012-per-appid-download-layout.md) 下载目录、[ADR 0013](docs/adr/0013-live-stream-recording.md) 直播录制；历史计划见 `docs/archive/`。
- **代码入口：** `main.go`/`internal/app/` 启动与运行时；`internal/tui/model.go` TUI；`internal/interceptor/miniprogram.go` 嗅探；`internal/minidownload/` 下载；`third_party/echo/` 代理库 patch。

## Agent skills

- Issue tracker：GitHub Issues `MuZiJin701/wx-mini-video`，用 `gh`；见 `docs/agents/issue-tracker.md`。
- Triage labels：`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`，加 `bug`/`enhancement`；见 `docs/agents/triage-labels.md`。
- Domain docs：单上下文，`GLOSSARY.md` + `docs/adr/`；见 `docs/agents/domain.md`。

## 变更

- 先检查相关实现、调用方、测试和已有未提交修改；保留无关用户修改。
- 用现有模式完成最小正确改动；在共享路径修复根因。
- 用户可见行为、配置、构建或发布变化时同步更新对应文档；维护决策写入 `docs/adr/`。
- 项目文档使用中文；代码标识、命令和专有名词保留原文。
- `dist/`、`downloads/`、截图、海报、日志、临时 exe、ffmpeg 二进制与许可文本缓存只留本地；`build/licenses/` 下的 GPL 全文与根目录 `THIRD-PARTY-NOTICES.md` 必须提交。
- 不放宽目标候选的严格匹配，归属与标记语义以 [ADR 0008](docs/adr/0008-target-attribution.md) 为准。

## 验证与提交

- Go 修改后运行 `go test -count=1 ./...`、`go vet ./...`；修改 `third_party/echo/` 后加跑 `go test -count=1 github.com/ltaoo/echo/...`。
- Windows 分发改动后运行 `cmd /c build\build.bat windows`；包内只可有 `README.md`、`wx-mini-video.exe`、`wx-mini-video.yaml`、`ffmpeg\ffmpeg.exe`、`ffmpeg\LICENSE.txt`、`THIRD-PARTY-NOTICES.md`。
- macOS 分发改动后运行 `go run ./build/macos` 和 `python3 build/verify-macos.py`。
- 修改 `build/ffmpeg/` 后运行 `python -B -m unittest discover -s build/ffmpeg -p "test_*.py"`；三架构构建按 `build/ffmpeg/README.md`。
- 文档改动至少运行 `git diff --check`。
- 每次完成修改后提交 Git；除非用户明确要求，不推送远程仓库。
