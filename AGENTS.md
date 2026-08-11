# AGENTS.md

## 项目

`wx-mini-video` 是 Windows 优先的 Go TUI：通过本地 HTTPS 代理识别 PC 微信小程序加载的图片、直链视频和 m3u8，并下载到本地。

当前范围是通用小程序媒体嗅探与下载。保持单二进制 TUI、Echo 代理适配、Windows 系统代理与证书、媒体候选识别、图片/直链视频下载及 m3u8 的 ffmpeg 合并。发布包不内置 ffmpeg。

## 先读

- **术语、边界或不变量：** `CONTEXT.md`。
- **架构、配置、构建与排障：** `wx-mini-video-knowledge.md`。
- **用户流程或发布说明：** `README.md`。
- **当前计划：** `docs/superpowers/plans/`；`docs/archive/` 只提供历史背景。
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

需求与缺陷记录在 GitHub Issues。参见 `docs/agents/issue-tracker.md`。

### Triage labels

使用默认五类标签。参见 `docs/agents/triage-labels.md`。

### Domain docs

单上下文布局：根目录 `CONTEXT.md` 与 `docs/adr/`。参见 `docs/agents/domain.md`。
