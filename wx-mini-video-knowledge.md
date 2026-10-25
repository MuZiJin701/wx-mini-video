# 维护指南

用户操作见 [README.md](README.md)，术语见 [GLOSSARY.md](GLOSSARY.md)，发布历史见 [CHANGELOG.md](CHANGELOG.md)，macOS 使用见 [docs/macos-usage.md](docs/macos-usage.md)。本文件记录实现、配置、构建、排障与验收；决策背景见 `docs/adr/`，历史计划见 `docs/archive/`。

## 架构

Windows 优先的单二进制 Go TUI，Bubble Tea + Echo 代理适配。

| 路径 | 职责 |
| --- | --- |
| `main.go`、`pkg/system/console_windows.go` | 启动、Terminal 选择、提权子进程入口 |
| `pkg/certificate/` | 根证书生成/复用、信任检查、安装与旧证书清理 |
| `internal/app/` | 代理、目标、候选与下载器的运行时编排；下载阶段直播状态回写 |
| `internal/tui/` | 单主界面、统一目标选择区、媒体视图、外观下拉、主题与鼠标命中 |
| `internal/tui/ui/` | 外观模式与 `ui.json`、系统明暗、主题色板、布局度量与显示宽度 |
| `internal/tui/logo/` | 终端图片能力探测、Sixel/kitty 编码与每帧绘制（失败静默降级）；`assets/logo.png` 为复用参考项目的 512×512 品牌图 |
| `internal/interceptor/miniprogram.go` | 来源过滤、媒体与 HTTP-FLV 直播识别、播放列表缓存 |
| `internal/miniprogram/` | 目标、发现列表、候选存储、媒体分类与资源类型目录（图片/视频/音频/直播/分段媒体/其他） |
| `internal/minidownload/` | 直链下载（含直播录制）、m3u8 与 ffmpeg、按最终类型归档与晚修正 |
| `build/genicon/` | 把 `build/icons/logo.ico` 转换为 Windows COFF 资源（`.syso`） |
| `build/icons/` | 复用参考项目的 ICO/ICNS 品牌图标（仓库资产） |
| `build/uipreview/`、`build/ansi_to_png.py` | 文档界面预览与截图生成（`preview` 构建标签，不进入正式二进制） |
| `third_party/echo/` | 本地 patch 的代理库（MITM listener 回收） |

### 启动与代理

Windows 的 `PrepareConsole` 在初始化前只选终端：优先已安装的 `wt.exe`，已在 Windows Terminal 中则沿用当前窗口，否则保留原控制台；主程序始终以当前用户运行，系统代理写入当前用户 `HKCU`。只有安装根证书或清理旧共享根证书时，才通过 `ShellExecuteExW` 的 `runas` 启动提权子进程，读取退出码后核对信任库，失败及时报错。取舍见 [ADR 0009](docs/adr/0009-user-scoped-windows-runtime.md)，修订 [ADR 0004](docs/adr/0004-windows-terminal-startup.md)。

macOS 通过系统授权管理活动网络服务的 HTTP/HTTPS 代理，捕获原地址后设置、退出恢复；PAC、SOCKS、认证代理或活动 VPN 默认路由冲突时拒绝覆盖，每 2 秒检查默认路由接口变化。私钥存用户 `Library/Application Support` 受限目录，信任安装到当前用户默认钥匙串；签名、公证与真实 Mac 微信验收尚未完成，见 [ADR 0006](docs/adr/0006-macos-portable-distribution.md)。

TUI 启动即自动启动代理并持续发现，界面显示启动状态；首次证书检查、UAC 与系统授权沿用现有流程，失败时提供可点击的“重试启动 · r”与页脚退出按钮。`target.appID` 只预填输入框：`prepareDiscoveryTarget` 在启动时清空运行时目标，发现阶段只登记小程序与来源未确认媒体，不产生目标候选；用户确认目标后才 `SetTarget` 并开始收集目标候选。正常退出恢复原代理；恢复或关闭失败时 TUI 保留错误界面，按 `r` 重试、`f` 强制退出，不静默丢弃。

### 目标与媒体

1. 默认代理 `127.0.0.1:2023`。只有 `https://servicewechat.com/<AppID>/<版本>/page-frame.html` 来源与目标 AppID 完整匹配的媒体才计入**目标候选**并缓存播放列表；来源缺失、格式不符或其他 AppID 的媒体类响应作为**来源未确认**候选（`?`）按类型并入分类，可查看和下载，不写播放列表缓存。取舍见 [ADR 0008](docs/adr/0008-target-attribution.md)。
2. **启动即发现、统一目标选择区**：打开程序后自动启动代理并持续发现；发现阶段只登记 AppID 与首次/最近出现时间（仅本次运行），按 AppID 去重，不产生目标候选，但照常收集来源未确认媒体，确认目标后显示。`target.appID` 只预填输入框，运行时目标在用户确认前为空。目标选择区同时呈现 AppID 输入（光标编辑）、确认按钮、启动/发现状态与持续更新的已发现列表，`Tab`/`Shift+Tab` 顺序为外观 → 输入 → 发现列表（有结果时）→ 确认 → 取消（仅更换目标时）；`↑/↓` 导航列表，`Enter` 执行当前焦点对应的确认动作，鼠标点击发现项直接确认。发现结果不自动选定目标、不覆盖输入、不抢焦点，选中身份按 AppID 而非行号保持。确认后收起完整输入与列表，保留目标条与“更换目标 · s”；`s` 展开同一区域，`Esc`/取消保留当前目标、候选与已输入内容；下载/录制中禁止更换目标。v0.2.0 删除小程序名称与 `Ctrl+D` 模式切换。
3. **六个媒体分类**：页签固定 `1 全部 / 2 图片 / 3 视频 / 4 音频 / 5 直播 / 6 分段媒体`，进入媒体视图默认“全部”。直播与普通视频互斥：HTTP-FLV 直播候选只进“全部”和“直播”；疑似直播仍标“直播流”，实际响应确认是有限 FLV 后归回“视频”。m3u8 分类显示名为“分段媒体”，详情保留 m3u8/HLS 格式信息与“由播放列表描述、合并为 mp4”，HLS 视频、音频、播放列表都留在该分组。未知文件只在“全部”。直播状态被下载响应修正而移出当前分类时，选中会清空并提示重新选择，避免误操作另一条资源。
4. 直接响应按 Content-Type 或 URL 后缀识别媒体。FLV（`video/x-flv`、`video/flv`、`application/x-flv` 或 `.flv` 地址）归为视频并保存 `.flv`；无 `Content-Length` 的 FLV 响应或 `.flv` 地址标记为疑似 **HTTP-FLV 直播流**（`Candidate.Live`），真实响应会修正 JSON/H5 的推测。JSON 候选可带标题；跨域链接仅保留 User-Agent、Referer、Accept、Accept-Language，不转发接口 Cookie/Authorization。未知类型且 ≥256 KB 的响应可作「未知文件」兜底。对小程序 H5 页面/脚本（≤4 MB，含 `miniProgram`、`jweixin`、`ckplayer` 或 `<video` 标记）扫描内联媒体地址，作为来源未确认候选（标记「H5 页面/脚本扫描（未播放验证）」）。
5. 直链先写带资源标识的 `.part` 与校验元数据，仅在有强 ETag 时续传；HTTP 分段响应从 Content-Range 取得总长度并校验。默认命名在重试时会按候选标识反查遗留 `.part`，避免时间戳变化导致重新下载全部内容；复用前按 `FLV` 魔数跳过未保存的直播录制，并为新录制生成独立文件名——选名在同一循环同时避开成品与同候选 `.part`（含 `_2` 变体），续传直接使用遗留基名，避免二次改名绕过保护；指定文件名与疑似直播（含 `.flv` 地址）也不复用旧文件。下载时才确认为直播时，产物后缀会修正为 `.flv`，并由 `finalizeOutputPath` 移入 `直播/`。**HTTP-FLV 直播**在响应头与 FLV 魔数确认后持续录制：不发 Range、不校验总长度、状态为 `recording`；服务端结束则正常提交；用户按 `x` 或确认退出时取消上下文，把已录内容提交为 `.flv`（`ErrRecordingStopped`，仍写 `history.jsonl`）。异常断流（`ctx` 未取消时的 `unexpected EOF`、断网等）同样提交已录内容并写历史，返回 `ErrRecordingInterrupted` 并在状态行报告原因；提交失败则保留 `.part` 并原样报错。只有真正的上下文取消才折算为「已停止」，停止录制时的保存失败不会被 `ErrDownloadStopped` 掩盖（普通下载停止保留 `.part`）。成功记录追加到下载根目录 `history.jsonl`，含路径、目标/来源 AppID、类型、归属、来源域名、大小与 UTC 时间；路径为最终真实路径。决策见 [ADR 0013](docs/adr/0013-live-stream-recording.md)。本轮仍不支持 HLS（m3u8）持续直播录制。
6. m3u8 使用已捕获播放列表，缺失时允许远程回补，由 ffmpeg 合并为 mp4。捕获缓存保留相对引用，下载阶段统一解析；预取按候选与播放列表内容隔离，按来源重放鉴权头。ffmpeg 经仅在下载期间运行的随机端口入口读取缓存，入口只接受已登记路径；跨源重定向不携带凭据。合并产物为 `.mp4`，播放列表缓存与 `.segments` 分片同放 `分段媒体/`。
7. 下载目录为 `downloads/<目标 AppID>/{图片,视频,音频,直播,分段媒体,其他}`；“全部”是汇总视图，无对应目录。HTTP-FLV 直播录制进入 `直播/`；普通有限视频（含确认是有限文件的 FLV）进入 `视频/`；m3u8 合并产物、播放列表缓存与 `.segments` 分片进入 `分段媒体/`；未知文件进入 `其他/`。下载时才发现类型不一致的候选会被移到正确目录，跨目录调整不覆盖已有成品或未保存录制；若创建最终目录或移动成品失败，成品保留在原目录并返回明确的归档失败错误（不按成功上报、不写 `history.jsonl`，进度行为错误并给出保留路径）。文件名 `<类型前缀>_<YYYYMMDD-HHMMSS>_<候选ID前8位>.<扩展名>`（直播前缀“直播_”，分段媒体前缀“分段媒体_”，重名追加 `_2`、`_3`），时间取本地时区下载开始时间。来源未确认候选按下载时目标 AppID 归档；AppID 目录名清洗非法字符、拒绝 `.`/`..` 与路径穿越、处理 Windows 保留名。日志、debug 日志、`history.jsonl`、`ui.json`、根证书、受管 ffmpeg 与 `.ffmpeg/` 留在下载根目录。`o`/`Ctrl+O` 在目标已选且目录存在时打开 `<目标目录>`，否则打开下载根。旧版平铺文件与旧 `m3u8/` 目录不迁移、不自动删除、不复用；程序正常启动不清空 `downloads/`，旧数据的一次性本地清理是本次人工授权操作，不是程序行为。取舍见 [ADR 0012](docs/adr/0012-per-appid-download-layout.md)。

支持格式：图片 `jpg/png/webp/gif/bmp/avif`；直链视频 `mp4/webm/mov/flv`；直链音频 `mp3/m4a/aac/wav/ogg/flac/opus/amr`。不提供转码、音轨提取或播放。

### 界面、外观与下载目录

TUI 为单主界面：品牌区（真图 logo 或块字符降级、右侧“外观 <当前模式> ▾”下拉）→ 渐变分隔线 → 主区（统一目标选择区，或目标条 + 分类页签 + 操作按钮栏 + 候选表格 + 状态/进度行）→ 页脚（GitHub 链接与“退出 · q”）。目标选择区中状态行下方、发现列表上方是确认/取消/重试启动按钮；媒体视图在页签与表格之间有下载、停止、详情、日志、刷新、打开目录、清空、更换目标八个按钮，宽窗口为 3 行带边框按钮，小窗口按宽度换行或收缩为单行 chip，禁用状态与快捷键一致。详情抽屉、日志浮层、外观下拉、退出确认、旧证书确认与代理恢复失败界面都是覆盖层，带可点击的关闭/取消/重试/退出按钮；覆盖层用横向合成保留边缘内容。页签、行、按钮、外观控件支持鼠标，模态覆盖层（退出确认、旧证书确认）的按钮、悬停与取消也响应鼠标，其余区域在模态期间忽略。

外观模式（跟随系统/浅色/深色）在目标选择区可 `Tab` 聚焦后用 `←/→` 直接切换，`Enter`/点击展开下拉，`↑/↓` + `Enter` 或点击选择，`Esc` 关闭；媒体视图点击控件展开。输入框聚焦时 `←/→` 只编辑文本。偏好原子写入 `downloads/ui.json`；Windows 读 `HKCU\...\Themes\Personalize\AppsUseLightTheme`，macOS 读 `defaults read -g AppleInterfaceStyle`，失败回退深色；跟随系统在界面刷新定时器中持续重新读取，无需重启。真图 logo 在 Bubble Tea 接管 stdin 前探测（kitty 查询 + CSI 16t + DA1），仅正面回复才绘制内嵌 512×512 PNG，覆盖层/缩放/退出时关闭；否则降级块字符，极窄窗口隐藏外观控件与品牌图。

布局断点为 76 列、52 列、32 行；110×40、80×24、60×20 均可用：小窗口缩减次要信息、按钮换行或收缩为单行 chip，分类页签自动从“带计数完整标签”降级到“无计数标签”再到 `1全部`…`6分段媒体` 紧凑标签，保证 60 列仍能看到全部分类。悬停样式不得改变行列尺寸；自动化回归遍历目标选择（启动中/已发现/切换目标）、媒体列表（普通/下载中）、详情抽屉、外观下拉、日志浮层、退出确认、旧证书确认、启动失败与代理恢复失败等界面状态，在 110×40、80×24、60×20 三种尺寸下对每个命中区域断言物理行数、每行显示宽度与命中坐标稳定。列表「已下载」标记仅在当前进程有效，详情显示本地路径。直播候选显示「直播流」，录制中行尾显示「录制中」，状态行显示已录大小与 `x` 提示，中断时显示「直播中断并保存」与原因。下载/录制中确认退出会先停止并保存现场；若收尾失败（如保存已录内容失败）会取消自动退出并保留错误界面与 `.part`，处理后可再次退出。取舍见 [ADR 0011](docs/adr/0011-single-main-surface-ui.md)。

## 配置

模板见 [internal/config/config.template.yaml](internal/config/config.template.yaml)。发布包默认读 exe 旁 `wx-mini-video.yaml`，`WX_MINI_VIDEO_CONFIG` 可指定其他文件；修改后重启。

| 配置 | 默认值 | 含义 |
| --- | --- | --- |
| `download.dir` | 空 | 配置目录下 `downloads/`；普通发布包即 exe 旁，相对路径按当前工作目录解析 |
| `download.ffmpeg` | 空 | 显式 ffmpeg 路径，优先级最高；旧值 `ffmpeg` 视为空，不查 PATH |
| `download.mode` | `auto` | 分片 ≥20 时并发预取，否则 ffmpeg 经本机入口获取；`prefetch`/`remote-ffmpeg` 强制对应方式 |
| `download.segmentConcurrency` / `segmentRetries` | `6` / `3` | 预取并发数与重试次数 |
| `download.keepSegments` | `false` | 合并成功后保留分片缓存 |
| `capture.showUnattributed` | `true` | 是否收集并显示来源未确认媒体 |
| `proxy.system` | `true` | 设置并恢复 Windows/macOS 系统代理 |
| `proxy.hostname` / `proxy.port` | `127.0.0.1` / `2023` | 本地监听地址 |
| `proxy.skipInstallRootCert` | `false` | 跳过根证书检查与自动安装 |
| `proxy.upstreamProxy` | 空 | 上游代理 |
| `target.appID` | 空 | 目标选择区输入框预填值；启动不会自动确认，空值不表示接收全部媒体 |
| `cert.file` / `cert.key` | 空、空 | 必须成对配置的 PEM 证书与私钥；优先于自动生成证书 |
| `cert.name` | `Echo` | 仅兼容保留，信任检查使用指纹 |

ffmpeg 优先级：`download.ffmpeg` > 随包（Windows `ffmpeg\ffmpeg.exe`，macOS `.app` 外 `ffmpeg/ffmpeg-amd64|arm64`）> 受管下载。随包版本为固定 FFmpeg 9.0.2 + OpenSSL 3.5.9 官方源码自建（LGPLv3，静态链接），启动时做存在/可执行/`-version` 探测，缺失或损坏回退受管下载；受管源与 SHA-256 在 `internal/minidownload/ffmpeg.go`，更新时同步修改。见 [ADR 0007](docs/adr/0007-bundled-ffmpeg.md)、[ADR 0010](docs/adr/0010-source-built-ffmpeg.md)、`build/ffmpeg/README.md`。

## 证书风险与清理

首次启动代理前为当前用户生成独立 RSA 根证书与私钥（5 年），保存到 `%LOCALAPPDATA%\wx-mini-video\certificates\root-ca.pem` 并复用；目录使用受限 ACL。证书损坏、过期或不匹配时停止启动，不覆盖原文件。升级/移动程序不改变证书；换电脑或 Windows 用户需重新生成并信任。

显式 `cert.file`/`cert.key` 优先，必须成对且可用于 Echo。安装通过提权子进程执行 `certutil -addstore Root` 写入本地计算机根信任库，只影响信任范围，退出不卸载；信任检查比对 SHA-1 指纹而非名称。

旧版默认 SunnyNet 根证书使用公开共享私钥，仅替换 exe 不能撤销信任。启动前检查固定旧指纹 `D70CD039051F77C30673B8209FC15EFA650ED52C`，按 `y` 精确删除或按 `n` 保留并继续；手动清理时在 `certlm.msc`/`certmgr.msc` 核对指纹后只删该项，不按名称批量删除。见 [ADR 0005](docs/adr/0005-local-ca-lifecycle.md)。

## 排障

| 现象 | 处理 |
| --- | --- |
| 异常退出后网络不通 | 重开 exe → 等界面显示“代理运行中”→ 按 `q`（输入框聚焦时用 `Ctrl+C` 或页脚“退出 · q”）；仍异常时在系统代理设置移除本工具残留地址 |
| 启动即发现但没有目标 | 正常：发现不等于选定；`target.appID` 只预填输入框，确认前运行时目标为空、不产生目标候选。选择已发现小程序或输入 AppID 后按 `Enter`/点击确认。 |
| 证书失败 | 查看错误弹窗并确认 UAC；仍失败可将 `downloads/WxMiniVideoRoot.cer` 安装到根信任库，或启动代理后访问 `http://127.0.0.1:2023/cert` |
| 没有候选或音频 | 核对目标 AppID，按 `1` 看全部、`4` 看音频，重新加载媒体；`?` 媒体不会成为目标候选 |
| 来源未确认媒体 | H5/web-view、原生播放器与 CDN 常无 page-frame Referer；列表上限 500 条，按 `c` 清空，来源不可靠 |
| 分类里找不到直播或 m3u8 | 直播只在“全部”和“直播”，m3u8 在“分段媒体”；疑似直播确认为有限文件后会回到“视频”，选中被清空时按提示重新选择 |
| 想录制 HLS/m3u8 直播 | 本轮不支持持续录制；无 `#EXT-X-ENDLIST` 的播放列表可能只保存当前快照并合并为 mp4，不能当完整录制 |
| 直播下载不结束 | 正常：HTTP-FLV 无结束点；按 `x` 停止并保存 `.flv`，需要 mp4 时用 ffmpeg 转封装 |
| 直播断流后内容 | 已录内容会自动保存为 `.flv` 并写历史，状态行/日志给出原因；保存失败时错误与 `.part` 均保留，可重试或手动恢复 |
| 确认退出时保存失败 | 自动取消退出并保留错误界面与 `.part`，`l` 查看日志，处理后可再次按 `q` 退出 |
| 旧下载文件与旧 `m3u8/` 目录 | 不迁移、不自动删除、不复用；程序启动不清空 `downloads/`，需要时人工整理 |
| 诊断嗅探 | `--debug` 启动（参数随 Terminal 重启转发）或 `WX_MINI_VIDEO_DEBUG=1`，日志写入 `wx-mini-video-debug.log`，query 与 Cookie 不落盘 |
| m3u8 预取失败多 | 降低 `download.segmentConcurrency`（如 4）后重新播放重试 |
| m3u8 合并失败 | 确认 `ffmpeg/` 可用或自动下载兜底；查看错误给出的 `.ffmpeg/<候选ID>.log`，日志已脱敏但仍需检查 URL |
| 中文缺失 | 使用 Windows Terminal 或选择支持中文的字体；程序不改默认终端 |

## 开发验证

```powershell
go test -count=1 ./...
go vet ./...
git diff --check
```

测试覆盖来源过滤与副作用、启动即发现与预填目标不自动确认、统一目标区的焦点顺序/输入编辑/列表身份保持/确认收起与取消、六个分类的顺序与默认页签、直播互斥与疑似直播标记、直播状态下载回写与候选移出当前分类时清空选中、下载目录与命名（图片/视频/音频/直播/分段媒体/其他、晚修正跨目录移动、跨目录不覆盖成品与未保存录制、history 最终路径）、Windows 启动编排、TUI 布局/鼠标/外观下拉与持久化、logo 探测与编码，以及 HTTP-FLV 分类、chunked 录制到用户停止、异常断流保存、停止时保存失败不被掩盖、真实时钟下的重试续传、未保存录制不被重试覆盖、成品与 `.part` 双重命名冲突、晚确认直播的后缀修正与 `.flv` 命名；悬停回归覆盖目标选择、媒体列表、下载中、详情、外观下拉、日志、退出确认、旧证书确认、启动失败与代理恢复失败等 12 种界面状态 × 110×40、80×24、60×20 三种尺寸，对每个命中区域断言物理行数、每行显示宽度与命中坐标稳定；图标回归覆盖 `build/genicon` 从 7 帧 ICO 生成 COFF 资源与 macOS `Info.plist`/`logo.icns` 打包检查；退出回归覆盖保存失败时取消自动退出。修改 `third_party/echo/` 后运行 `go test -count=1 github.com/ltaoo/echo/...`；代理恢复回归使用替身，不修改真实代理或证书。自动化不能替代真实微信、Windows Terminal 交互和 Mac 真机验收。

文档界面预览由真实 `View` 渲染，不手工伪造：`go run -tags preview ./build/uipreview` 生成 `build/preview/{home,discovered,appearance,media,recording,downloaded,logs}.ansi`，再用 `build/ansi_to_png.py` 转为 PNG（依赖 Cascadia Mono、微软雅黑与 Segoe UI Symbol 字体）。`build/preview/` 与 `dist/` 不入库。

macOS CI 在 `macos-15-intel` 与 `macos-15` 上运行全量测试、`go vet`、通用包构建（含双架构 ffmpeg）与 `python3 build/verify-macos.py`，设置 `WX_MINI_VIDEO_MACOS_INTEGRATION=1` 时只读调用真实网络服务与钥匙串并验证鉴权 m3u8 合并。打包检查在中文/空格路径解压并启动 TUI：TUI 验收使用临时目录中的隔离配置与临时 `HOME`（`proxy.system=false`、`proxy.skipInstallRootCert=true`、独立下载目录与随机端口），只启动本地代理，不修改用户系统代理、不安装证书；不覆盖 Finder 双击、真实系统授权、真实微信或 macOS 13 兼容性。

## 分发

Windows 与 macOS 各只发布一个完整包，不提供精简包；两平台附件在同一 Release 并列提供并标明状态。

```powershell
cmd /c build\build.bat windows          # dist\wx-mini-video-windows-amd64\ 与同名 zip
go run ./build/macos                    # dist\wx-mini-video-macos-universal-unsigned.zip
```

Windows 包内精确包含 `README.md`、`wx-mini-video.exe`、`wx-mini-video.yaml`、`ffmpeg\ffmpeg.exe`、`ffmpeg\LICENSE.txt`、`THIRD-PARTY-NOTICES.md`；macOS 通用包包含 `.app`、配置、说明、`.app` 外双架构 `ffmpeg/` 与许可声明。构建脚本联网下载并校验 ffmpeg，缓存于 `dist/ffmpeg-cache/`；`WXMV_FFMPEG_DIR` 可改用自建目录。签名/公证只覆盖 `.app`，Mac 正式发布仍需 Developer ID、公证和双架构真机验收，见 `build/sign-macos.sh` 与 [ADR 0006](docs/adr/0006-macos-portable-distribution.md)。

图标资源复用本地参考项目 `wechat-channels-downloader` 的 logo，不重新绘制：`internal/tui/logo/assets/logo.png`（512×512）用于 TUI 品牌区；`build/icons/logo.ico`（7 帧）由 `go run ./build/genicon` 转换为仓库根 `wx-mini-video_windows_amd64.syso`（单个 `.rsrc` 段，含 `RT_GROUP_ICON`/`RT_ICON`），`build.bat windows` 在 `go build` 前自动执行该生成步骤，Go 链接器把资源写入 exe；`*.syso` 被 `.gitignore` 忽略，是本地构建产物、不入库。macOS 构建 `go run ./build/macos` 把 `build/icons/logo.icns` 复制到 `.app/Contents/Resources/logo.icns`，`Info.plist` 声明 `CFBundleIconFile`/`CFBundleIconName=logo`，`build/verify-macos.py` 校验 ZIP 内图标字节与 plist 字段。旧的 `build/genlogo`（生成品牌图）已删除，品牌图只来自参考项目资产。

发布前完成检查并提交，确认 exe 的 `vcs.revision` 与提交一致、`vcs.modified=false`；上传前运行 `python3 build/verify-release-sources.py`，核对 zip SHA-256、最新版本标记和既有附件未变。随包 ffmpeg 为 LGPLv3 独立程序，网络分发须在同一 Release 提供对应源码归档与许可全文，见 `THIRD-PARTY-NOTICES.md`。推送、创建 Release、上传附件须有明确授权。

## 真实 PC 微信验收

当前只确认普通用户 Windows Terminal 中显示正常；证书提权、标准用户 + 独立管理员、真实微信音频、来源覆盖、真实终端缩放与 Mac 真机仍待验收。

1. 双击启动，确认 Terminal 选择、中文/空格路径、首次证书 UAC 与取消提示；程序打开即自动启动代理并显示“正在启动代理，自动发现小程序…”，随后显示“代理运行中”和“已发现 N 个小程序”。在 PC 微信打开两个小程序，确认发现列表按 AppID 去重并持续更新；未确认目标前列表无目标候选，来源未确认媒体照常收集。
2. 目标选择区：分别验证手动输入（`←/→`、`Home/End`、`Backspace/Delete`、`Ctrl+U` 编辑；`Ctrl+D` 不应有任何模式切换效果）和从发现列表选择（`↑/↓` 导航、`Enter` 确认、鼠标点击发现项直接确认）。确认前 `target.appID` 只预填输入框，不自动确认；确认后完整输入框与发现列表收起，保留目标条与“更换目标 · s”。`s` 重新展开同一区域，`Esc`/取消保留当前目标、候选与已输入内容；下载/录制中尝试更换应被拒绝。多个发现结果持续更新时不覆盖输入、不抢焦点，选中身份按 AppID 保持。
3. 重新加载图片、直链视频、直链音频、m3u8 和 HTTP-FLV 直播；核对六个页签顺序 `1 全部 / 2 图片 / 3 视频 / 4 音频 / 5 直播 / 6 分段媒体`、默认“全部”、各分类计数、标题、后缀、原文件保存、`✓`/`?` 与严格过滤。直播只出现在“全部/直播”，m3u8 显示为“分段媒体”，未知文件只在“全部”。
4. 其他小程序媒体不进入目标候选；来源未确认媒体允许出现但标记 `?`；JSON 图片/视频/音频可出现，JSON m3u8 继续跳过。打开 H5 详情页可产生「H5 页面/脚本扫描（未播放验证）」候选。
5. 按钮与快捷键：媒体视图上方八个按钮（下载 · d、停止 · x、详情 · i、日志 · l、刷新 · r、打开目录 · o、清空 · c、更换目标 · s）与页脚 GitHub 链接、“退出 · q”按钮可点击；逐个验证点击与快捷键动作一致；空列表时下载/详情禁用；下载/录制中禁用下载与更换目标、停止可用。详情抽屉、日志浮层、目标选择区、退出确认、旧证书确认、代理恢复失败界面都能用可点击按钮关闭/取消/重试/退出。
6. 外观：品牌区右侧“外观 <当前模式> ▾”；目标区 `Tab` 聚焦后 `←/→` 直接切换，`Enter`/点击展开跟随系统/浅色/深色，`↑/↓` + `Enter` 或点击选择，`Esc` 关闭；媒体视图点击可展开；输入框聚焦时 `←/→` 只移动光标。切换后重启确认 `downloads/ui.json` 保留；切换系统明暗确认跟随模式自动更新；不支持真图的终端确认降级块字符，极窄窗口确认外观控件隐藏。
7. 布局与悬停：在 110×40、80×24、60×20 三种窗口尺寸下检查品牌区、分类页签（60 列仍能看到 `1全部`…`6分段媒体`）、按钮换行与单行 chip、目标选择区按钮位置（状态行下、列表上方）；把鼠标移入/移出每个页签、候选行、按钮、发现项、外观下拉、覆盖层按钮，确认行列数与命中坐标不跳变，覆盖层不抹掉边缘内容；验证滚动与切换分类。
8. 退出并核对代理恢复；异常退出按排障恢复；证书不随退出卸载；验收记录不含 Cookie、鉴权头或带签名 URL。
9. 下载目录：核对 `downloads/<AppID>/{图片,视频,音频,直播,分段媒体,其他}`、文件名 `<类型前缀>_<YYYYMMDD-HHMMSS>_<候选ID前8位>.<扩展名>`、`history.jsonl` 的 `app_id`/`source_app_id`/`kind`/`attribution` 与最终路径、`o` 打开的目录；确认旧平铺文件与旧 `m3u8/` 目录不迁移、不自动删除，程序启动不清空 `downloads/`。
10. HTTP-FLV：候选显示「直播流」与识别依据；录制中显示已录大小、行尾「录制中」；`x` 停止后保存 `.flv` 到 `直播/` 且写入历史；录制中退出先停止保存再恢复代理；不再出现 `.flv.mp4`；制造断流（关网络或断开响应）后确认已录内容自动保存、状态行提示原因、`history.jsonl` 有记录；人为让最终文件保存失败时应看到真实保存错误且 `.part` 保留，确认退出时应取消退出并保留错误界面；重试不得覆盖上一次未保存的直播录制。
11. 晚修正：只有下载时才发现是直播的候选中止/完成后应保存 `.flv` 并进入 `直播/`，历史与打开位置使用最终路径；疑似直播确认为有限文件的应移回 `视频/`，越出当前分类时选中被清空并提示重新选择；跨目录移动不得覆盖已有成品或未保存录制。
12. 分段媒体：m3u8 合并产物、播放列表缓存与 `.segments` 分片都进入 `分段媒体/`，详情保留 m3u8/HLS 信息；确认本轮没有 HLS 持续直播录制能力，不能把分段媒体下载表述成 HLS 直播录制。
13. 重试与续传：直链下载中断后隔几秒再下载，确认复用原 `.part` 并发送 Range（服务器提供 ETag 时），而不是从头下载；直播重试应生成新文件，不覆盖上一次录制。
