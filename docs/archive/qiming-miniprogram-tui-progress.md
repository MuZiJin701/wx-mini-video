# 启明星小程序视频下载 TUI 实施进度

## 当前状态

核心功能已收敛为 Windows 优先的启明星小程序视频 TUI 下载器。最新验证：`go test ./...`、`go vet ./...`、`go build -o qiming-video.exe .` 全部通过。

本轮重新梳理后，启动时会检测 ffmpeg；Windows 下未检测到时会后台自动下载便携版 `ffmpeg.exe` 到下载目录。代理仍优先启动，下载器新增进度事件，TUI 会显示 ffmpeg 下载进度、直链视频进度条和 `m3u8` 缓存/合并阶段。

## 已完成

### 核心功能

- [x] `go run .` 直接进入 TUI
- [x] 固定目标：启明星软件（AppID `wx9ed9ca51b87086d9`）
- [x] 本地 HTTPS 代理启停 + 系统代理自动设置/恢复
- [x] Bubble Tea TUI：候选列表、日志面板、快捷键
- [x] 资源嗅探：Content-Type + URL 扩展名识别 mp4/webm/mov/m3u8
- [x] JSON 响应体递归提取媒体 URL
- [x] 候选资源去重存储（SHA1 ID）
- [x] 直链视频 HTTP 下载（复用嗅探请求头）
- [x] m3u8 调用本机 ffmpeg 合并为 .mp4
- [x] 启动时检测 ffmpeg；Windows 下缺失时自动下载便携版 ffmpeg
- [x] Windows 证书管理
- [x] Echo 代理（纯 Go）
- [x] 配置系统（viper）：下载目录、ffmpeg 路径、代理端口等
- [x] 默认下载目录改为当前工作目录下的 `downloads/`

### m3u8 下载方案

- [x] 嗅探到 m3u8 时立即缓存 playlist 内容到本地文件
- [x] 本地缓存的 m3u8 中 TS 分段相对路径 → 绝对 URL 重写
- [x] 下载时优先使用本地缓存文件（避免 auth token 过期 404）
- [x] 回退方案：下载时通过 HTTP 同步抓取 m3u8

### 代码级精简

- [x] 删除 `pkg/argv/`（旧 CLI 参数解析）
- [x] 删除 `pkg/cache/`（未使用的分片缓存）
- [x] 删除 `pkg/util/`（fs.go、time.go、util.go 均未被引用）
- [x] 删除无用方法 `CacheM3U8`

### 构建配置更新

- [x] `build/build.bat`：输出 Windows 便携目录和 zip
- [x] 删除 `build/release.sh`（旧的 NAS server 部署脚本，不适用于 TUI 工具）
- [x] `internal/interceptor/proxy/echo.go` 自进程名列表添加 `qiming-video`

### TUI 优化

- [x] 候选列表按终端高度自适应，显示 `↑ N more...` / `↓ N more...`
- [x] 所有渲染行截断到 `m.width`，防止换行溢出
- [x] ffmpeg 错误输出过滤掉版本 banner，只保留最后 6 行关键错误
- [x] 日志行截断到终端宽度
- [x] 下载进度条：直链视频显示字节百分比，`m3u8` 显示缓存/ffmpeg 合并阶段
- [x] TUI 顶部显示 ffmpeg 检查、下载、解压、可用/失败状态
- [x] 用户在 ffmpeg 自动下载完成前点击 `m3u8` 下载时，会等待同一个 ffmpeg 准备任务完成后继续下载
- [x] JSON 响应里提取出的 `m3u8` 不再进入候选列表，避免选择到没有本地缓存且 token 易过期的 playlist URL
- [x] 候选列表增加人类可读标题列：mp4 文件名、m3u8 回放日期/时间、stream 标识
- [x] ffmpeg 已可用时不再启动后台检测任务，减少启动期无用工作
- [x] 代理启动日志显示耗时，用于定位慢启动是否来自证书/系统代理设置
- [x] 日志完整写入下载目录 `qiming-video.log`，TUI 支持 PageUp/PageDown/Home/End 翻阅
- [x] 候选列表显示捕获时间、缓存状态、来源，并默认跟随最新候选
- [x] 界面提示先按 `c` 清空后播放目标视频，降低多个 `m3u8` 难以判断的问题

### 配置

- [x] 默认下载目录从 `%UserDownloads%\qiming-video` 改为 `{当前工作目录}/downloads/`
- [x] 用户可在 `qiming-video.yaml` 中通过 `download.dir` 覆盖

### 单元测试

- [x] 资源分类：mp4、m3u8、webm、mov、.ts、无关 URL
- [x] JSON 嵌套字段中提取媒体 URL
- [x] 请求头清洗：过滤 Host、Content-Length、Accept-Encoding 等
- [x] 文件名生成：候选 ID、m3u8 → .mp4 强制转换
- [x] ffmpeg 命令构建：headers 拼接、路径处理
- [x] m3u8 相对路径 → 绝对 URL 重写
- [x] m3u8 已绝对 URL 保持不变
- [x] 完整 m3u8 缓存流程（HTTP server → URL 重写 → 本地文件）
- [x] 缺失 ffmpeg 时报错
- [x] `Store.OnCandidateAdded` 锁外执行，避免回调中读取候选列表时死锁
- [x] 重复候选会合并 `CachedPath`、响应头、Content-Type、Content-Length
- [x] 下载前从 Store 重新读取最新候选，避免 TUI 持有旧的无缓存 `m3u8`
- [x] 直链下载进度上报
- [x] `Runtime.EnsureFFmpeg` 串行化，避免后台检测和用户下载同时触发重复 ffmpeg 下载
- [x] `m3u8` 缓存失败时不再退回远程 URL 交给 ffmpeg，而是给出清空候选并重新播放的明确提示
- [x] 验证流程调整：开发迭代先跑 `go test ./...` 和 `go vet ./...`；用户确认后再构建 exe
- [x] 清洗重放请求中的 `If-*`、`Range` 等头，避免 m3u8 重新抓取返回 `304 Not Modified`
- [x] m3u8 重写支持普通分片行和 `URI="..."` 标签属性，并在需要时继承 playlist query
- [x] ffmpeg 调用增加 reconnect 参数，降低临时网络波动导致的分片失败

## 未完成

- [ ] 集成测试（需 PC 微信 + 启明星小程序环境）

## 不再适用

- [x] `docs/` VitePress 文档站点已删除到只剩专用 Markdown 文档，不再维护旧站点。

## 问题记录与解决方案

### 问题 1：`.ts` 分段出现在候选列表

**症状**：候选列表中出现大量 `.ts` transport stream 分段，每个 5MB+，无独立下载意义。

**原因**：segment（kind="segment"）未被过滤，与普通视频候选同等处理。

**修复**：`internal/interceptor/miniprogram.go:73` — `addMiniProgramCandidate` 中增加判断 `candidate.Kind == "segment"` 时直接返回。

---

### 问题 2：TUI 界面只显示底部，上部不可见

**症状**：启动后 TUI 只显示日志面板底部，候选列表和标题都看不到。

**原因**：`View()` 一次性渲染所有内容，不根据 `m.height` 截断。候选列表全量输出导致总行数远超终端高度，终端自动滚动到底部。

**修复**：重写 `View()` 布局计算：
- 固定头部 4 行（标题 + 信息 + 状态 + 快捷键）
- 日志面板固定最多 9 行
- 剩余行数分配给候选列表（窗口化显示，中心对齐选中项）
- 所有行截断到 `m.width` 防止换行

---

### 问题 3：ffmpeg 未检测到时仍尝试下载 m3u8

**症状**：启动时提示"未找到 ffmpeg"，但用户下载 m3u8 时抛出原始 exec 错误。

**原因**：`downloadM3U8` 未在调用 ffmpeg 前检查 `exec.LookPath`。

**修复**：
1. `internal/minidownload/downloader.go:144` — `downloadM3U8` 开头检查 `exec.LookPath(d.FFmpegPath)`
2. TUI 启动不再阻塞等待 ffmpeg；只有下载 `m3u8` 时才要求本机 ffmpeg 可用。

---

### 问题 4：启动期自动下载 ffmpeg 不应阻塞代理

**症状**：启动后先检查或下载 ffmpeg，网络异常时代理迟迟不启动，用户无法开始嗅探视频。

**原因**：环境准备和代理启动耦合过紧。下载器真正需要 ffmpeg 的场景只有 `m3u8` 合并，启动时处理它会影响所有用户。

**修复**：TUI 启动时同时启动代理和后台 `EnsureFFmpeg`。Windows 下缺失 ffmpeg 时自动下载便携版到下载目录，下载完成后更新下载器的 `FFmpegPath`；下载进度显示在 TUI 顶部。

---

### 问题 5：m3u8 下载失败 — auth token 过期 404

**症状**：嗅探到 m3u8 候选后，点击下载时 ffmpeg 报 `HTTP error 404 Not Found`。

**原因**：m3u8 URL 中的 `auth_key` 参数为一次性使用或极短有效期。从嗅探到用户点击下载期间（几秒到几十秒），token 已失效。

**尝试方案**：
1. **异步 HTTP 回调**（`OnCandidateAdded` goroutine → `CacheM3U8`）：token 已过期，404
2. **同步 HTTP 回调**（插件内 `fetchM3U8HTTP`）：token 已被微信播放器请求消费，404
3. **直接读代理响应体**（`GetResponseBody()`）：echo 库在 `OnResponse` 回调前已消费/转发响应体

**最终方案**：

1. **响应体读取**：Echo 适配层的 `GetResponseBody` 在读取底层响应流后会重新塞回 body，避免缓存 m3u8 时把微信播放器的响应读空
2. **URL 重写**：缓存时将 m3u8 内 TS 分段相对路径（`../../../streams/xxx.ts`）解析为绝对 URL（`https://venus-live.qmxdata.com/streams/xxx.ts`）
3. **去重更新**：`Store.Add` 对重复条目检查 `CachedPath`，防止"JSON 提取（无缓存）先到 → m3u8 响应（有缓存）后到被跳过"

**涉及文件**：
- `internal/interceptor/miniprogram.go`：`captureM3U8` → `readProxyBody` + `fetchM3U8HTTP` 双通道
- `internal/minidownload/downloader.go`：`cacheM3U8FromURL` + `rewriteM3U8URLs`
- `internal/miniprogram/candidate.go`：`Store.Add` 重复条目 CachedPath 更新

---

### 问题 6：ffmpeg 错误信息过长导致 TUI 溢出

**症状**：ffmpeg 输出完整 version banner（15+ 行），在日志面板中造成大量换行。

**修复**：
1. `downloadM3U8` 中 `extractFFmpegError` 过滤版本信息，只保留最后 6 行有效错误
2. 错误信息标明来源：`缓存的本地文件` / `下载到本地文件` / `远程URL(本地缓存失败: ...)`

---

### 问题 7：候选资源回调在 Store 写锁内执行

**症状**：如果 `OnCandidateAdded` 回调中调用 `store.List`、`store.Get` 或其他读操作，会等待写锁释放；但回调本身又在写锁内执行，形成死锁。

**修复**：

- `internal/miniprogram/candidate.go`：`Store.Add` 在锁内只完成新增/更新，然后释放锁，再执行 `OnCandidateAdded`。
- 新增测试 `TestStoreOnCandidateAddedRunsAfterUnlock`，先验证会卡住，再改为通过。

---

### 问题 8：Windows 分发时可能无法自动安装证书

**症状**：首次运行需要安装本地代理根证书，普通用户权限、UAC 或企业策略可能阻止写入系统根证书。

**修复**：

- 程序启动时把根证书导出到下载目录 `QimingRoot.cer`。
- 自动安装失败时，错误信息提示管理员权限运行或手动安装导出的证书文件。
- 代理运行时可通过 `http://127.0.0.1:2023/cert` 下载证书。

---

### 问题 9：打开小程序时一次性出现大量 m3u8，无法判断哪个是目标视频

**症状**：小程序初始化或列表页可能预加载多个 `m3u8`，候选列表中难以判断具体视频。

**处理**：

- TUI 候选行增加捕获时间、缓存状态、来源字段。
- 新候选出现时默认跟随最新条目。
- 界面提示工作流：先按 `c` 清空候选，再播放目标视频。
- 这是当前低风险方案；后续若能从业务 JSON 中提取标题/课程 ID，可再把候选按标题分组。

---

### 问题 10：ffmpeg 已就绪但 m3u8 仍下载失败，报远程 URL 404

**症状**：日志显示 `ffmpeg 就绪`，但下载 `m3u8` 时失败：

```text
ffmpeg 合并失败(远程URL(本地缓存失败: HTTP 404 Not Found))
```

**原因**：用户选中的候选可能来自 JSON 响应中的 m3u8 URL，而不是代理现场捕获到的 playlist 响应。此类 URL 的 `auth_key` 很快过期，下载时再请求会 404。

**修复**：

- JSON 中提取出的 `m3u8` 不再加入候选列表。
- TUI 中没有 `cache` 标记的 `m3u8` 不允许直接下载，并提示重新播放目标视频。
- 下载器不再把缓存失败的远程 `m3u8` 交给 ffmpeg，避免输出误导性的 ffmpeg 404 错误。

---

### 问题 11：缓存 m3u8 合并失败，报 Failed to open segment

**症状**：已出现 `cache` 标记，下载时仍失败：

```text
Failed to open segment 262 of playlist 0
Error when loading first segment 'https://venus-video.qmxdata.com/records/streams/5313_v/...ts'
Error opening input file ...m3u8.
```

**原因**：缓存到本地的 m3u8 内部包含远程 `https://...ts` 分片。ffmpeg 读取本地 m3u8 时默认协议白名单只有 `file,crypto,data`，会拒绝打开其中的 `https` 分片。继续加 `-headers` 或 `-reconnect` 也会失败，因为这些是 HTTP 输入选项，会被绑定到本地 file 输入上。

**修复**：

- ffmpeg 参数增加 `-protocol_whitelist file,http,https,tcp,tls,crypto,data`。
- 当输入是本地缓存 m3u8 时，不再传 `-headers`、`-reconnect`、`-reconnect_streamed`、`-reconnect_delay_max`、`-http_persistent`。
- ffmpeg 失败时把完整 stderr 写到同名 `*.ffmpeg.log`，TUI 日志只展示摘要和完整日志路径，便于复制排查。

---

### 问题 12：长 m3u8 下载时间长且缺少真实进度

**症状**：长回放 m3u8 可能包含数百个 ts 分片。旧实现由 ffmpeg 顺序远程拉取分片并合并，只能显示滚动式进度条，用户难以判断是卡住还是视频太长。

**修复**：

- 下载器会先并发下载 m3u8 内的远程 ts、`#EXT-X-KEY`、`#EXT-X-MAP` 资源到 `downloads/.segments/<candidateID>/`。
- 默认分片并发数为 `6`，失败重试 `3` 次，成功合并后默认删除分片缓存。
- 生成本地 m3u8 后再调用 ffmpeg 合并，避免 ffmpeg 逐片远程等待。
- ffmpeg 合并阶段实时解析 stderr 中的 `time=...`，TUI 显示时长百分比和已处理时长。
- TUI 分片阶段显示 `分片下载中 x%  当前/总数  已下载大小`。
- 启动时自动下载 ffmpeg 显示字节百分比进度条，解压阶段显示固定状态。

**新增配置**：

```yaml
download:
  segmentConcurrency: 6
  segmentRetries: 3
  keepSegments: false
```

## 验证记录

### 自动化测试

```powershell
go test ./...
```
全部通过：
- `wx_channel/internal/minidownload`
- `wx_channel/internal/miniprogram`
- `wx_channel/pkg/certificate` — 2 个测试

### 编译

```powershell
go build -o qiming-video.exe .
```
通过，输出约 16MB。

### go vet

```powershell
go vet ./...
```
无警告。

### 最新验证时间

由 Codex 在当前工作区重新执行：

```powershell
go test ./...
go vet ./...
go build -o qiming-video.exe .
```

三项均通过；验证后已删除临时构建产物 `qiming-video.exe`。
