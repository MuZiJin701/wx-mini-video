# 仓库瘦身与后续优化实施计划

> **给 agent 工作者：** 实施本计划时，按任务逐项推进并更新复选框状态；每个代码修改任务完成后运行对应测试并提交 Git。

**目标：** 让 `wx-mini-video` 保持小而可靠，面向需要嗅探并下载 PC 微信小程序媒体资源的 Windows 用户。

**架构：** 保持当前单二进制 Go TUI 架构。新增功能前，先压缩仓库噪声，清理旧分发产物、旧产品专用命名和过时文档入口。

**技术栈：** Go、Bubble Tea/Lipgloss、Echo 代理适配、Windows 系统代理/证书 API、ffmpeg。

## 全局约束

- 不恢复旧视频号、Web 服务、Docker、bat/ps1 启动器或启明星专用流程。
- 不提交 `dist/`、`downloads/`、截图、海报、日志、生成的 exe 或 ffmpeg 二进制。
- Windows zip 只通过 GitHub Releases 发布，不进入 Git 历史。
- 代码修改后运行 `go test -count=1 ./...` 和 `go vet ./...`。
- 面向用户、维护者和 agent 的项目文档默认使用中文。

---

### 任务 1：完成仓库瘦身

**文件：**
- 修改：`.gitignore`
- 删除：`.dockerignore`
- 仅本地保留：`dist/`、`downloads/`、`poster/`、截图

**产出：**
- Git 只跟踪源码、测试、构建脚本和长期有效文档。

- [x] 删除 `.dockerignore`，因为 Docker 不再是支持的构建或分发路径。
- [x] 忽略截图和海报输出，避免宣传图、本地 QA 截图进入提交。
- [x] 保持 `dist/` 被忽略，zip 分发包通过 Releases 发布。
- [x] 验证 `pkg/system/fs.go`、`pkg/system/util.go`、`pkg/platform/*` 的引用；保留仍被调用的 `system.Open`，删除未使用的辅助函数和包。

### 任务 2：规范文档结构

**文件：**
- 新增：`AGENTS.md`
- 新增：`wx-mini-video-knowledge.md`
- 新增：`docs/superpowers/plans/2026-07-20-repository-slimming-and-optimization.md`
- 移动：旧启明星文档到 `docs/archive/`
- 修改：`README.md`

**产出：**
- 为用户、维护者和后续 agent 提供稳定文档入口。

- [x] 让 `docs/` 根目录只包含 `archive/` 和 `superpowers/`。
- [x] 将历史启明星专用文档放入 `docs/archive/`。
- [x] 在 `AGENTS.md` 记录当前项目边界。
- [x] README 保持面向用户和发布下载。
- [x] 在 `wx-mini-video-knowledge.md` 保存可复用的架构、配置、分发和排障知识。
- [x] 在 `AGENTS.md` 中明确项目文档默认使用中文。

### 任务 3：下一阶段优化

**文件：**
- 修改：`internal/tui/model.go`
- 修改：`internal/miniprogram/candidate.go`
- 修改：`internal/minidownload/downloader.go`
- 测试：对应 `*_test.go`

**产出：**
- 更容易判断资源来源，并让下载流程更可恢复。

- [x] 增加选中资源详情视图，显示完整 URL、来源 URL、请求头摘要和本地缓存路径；主列表继续保持简洁。
- [x] 从媒体 URL 附近的 JSON 字段中提取标题，写入 `Candidate` 的展示字段，帮助用户判断哪个资源是目标。
- [x] 直链下载改为 `.part` 临时文件，下载完成后原子重命名，支持失败后复用已有部分文件。
- [x] 增加 `downloads/history.jsonl` 下载历史，记录完成路径、来源域名、大小和时间。
- [x] 增加构建校验脚本，检查 zip 内容必须精确等于 `README.md`、`wx-mini-video.exe`、`wx-mini-video.yaml`。

### 任务 4：发布流程

**文件：**
- 修改：`README.md`
- 可选新增：`docs/superpowers/plans/YYYY-MM-DD-release-checklist.md`

**产出：**
- 可重复执行的 Windows 发布流程。

- [x] 运行 `go test -count=1 ./...`。
- [x] 运行 `go vet ./...`。
- [x] 运行 `cmd /c build\build.bat windows`。
- [x] 使用 `tar -tf dist\wx-mini-video-windows-amd64.zip` 检查 zip 内容。
- [ ] 将 zip 上传到 GitHub Releases，并附简短变更说明。
