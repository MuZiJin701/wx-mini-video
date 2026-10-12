# AGENTS.md

## 项目

`wx-mini-video` 是 Windows 优先的 Go TUI：通过本地 HTTPS 代理识别 PC 微信目标小程序加载的图片、直链视频、直链音频和 m3u8，并下载到本地。

当前范围是通用小程序媒体嗅探与下载。保持单二进制 TUI、Echo 代理适配、Windows 系统代理与证书、目标 AppID 严格过滤、手动输入与小程序发现、图片/直链视频/直链音频下载及 m3u8 的 ffmpeg 合并。Windows 启动请求 UAC 并优先使用已安装的 Terminal；发布包不内置 ffmpeg。

## 先读

- **领域术语：** `GLOSSARY.md`。
- **架构、配置、构建与排障：** `wx-mini-video-knowledge.md`。
- **用户流程或发布说明：** `README.md`。
- **版本变更与发布合并：** `CHANGELOG.md`；发布操作见 `wx-mini-video-knowledge.md` 的“分发”。
- **历史计划与背景：** `docs/archive/`。
- **代码入口：** 启动看 `main.go` 和 `internal/app/`；TUI 看 `internal/tui/model.go`；嗅探看 `internal/interceptor/miniprogram.go`；下载看 `internal/minidownload/`。

## 变更

- 先检查相关实现、调用方、测试和已有未提交修改；保留无关用户修改。
- 用现有模式完成最小正确改动；在共享路径修复根因。
- 用户可见行为、配置、构建或发布变化时同步更新对应文档；维护决策写入 `docs/adr/`。
- 面向用户、维护者和 agent 的项目文档使用中文；代码标识、命令和专有名词保留原文。
- `dist/`、`downloads/`、截图、海报、日志、临时 exe 与 ffmpeg 二进制只留本地，不加入提交。

## 验证与提交

- Go 代码修改后运行 `go test -count=1 ./...` 和 `go vet ./...`。
- Windows 分发改动后运行 `cmd /c build\build.bat windows`；产物仅可包含 `README.md`、`wx-mini-video.exe`、`wx-mini-video.yaml`。
- 文档改动至少运行 `git diff --check`。
- 每次完成修改后提交 Git；除非用户明确要求，不推送远程仓库。

## Agent skills

### Issue tracker

创建、读取或更新需求与缺陷时使用 GitHub Issues；操作约定见 `docs/agents/issue-tracker.md`。

### Triage labels

执行 triage 时使用默认五类状态标签及分类标签；映射见 `docs/agents/triage-labels.md`。

### Domain docs

探索代码或维护领域文档时采用单上下文布局：根目录 `GLOSSARY.md` 与 `docs/adr/`；读取和维护规则见 `docs/agents/domain.md`。
