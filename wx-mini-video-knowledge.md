# 维护指南

用户操作见 [README.md](README.md)，术语见 [GLOSSARY.md](GLOSSARY.md)，发布历史见 [CHANGELOG.md](CHANGELOG.md)，macOS 使用见 [docs/macos-usage.md](docs/macos-usage.md)。本文件记录实现、配置、构建、排障与验收；决策背景见 `docs/adr/`，历史计划见 `docs/archive/`。

## 架构

Windows 优先的单二进制 Go TUI，Bubble Tea + Echo 代理适配。

| 路径 | 职责 |
| --- | --- |
| `main.go`、`pkg/system/console_windows.go` | 启动、Terminal 选择、提权子进程入口 |
| `pkg/certificate/` | 根证书生成/复用、信任检查、安装与旧证书清理 |
| `internal/app/` | 代理、目标、候选与下载器的运行时编排 |
| `internal/tui/` | 单主界面、目标选择、媒体视图、主题与鼠标命中 |
| `internal/tui/ui/` | 外观模式与 `ui.json`、系统明暗、主题色板、布局度量与显示宽度 |
| `internal/tui/logo/` | 终端图片能力探测、Sixel/kitty 编码与每帧绘制（失败静默降级） |
| `internal/interceptor/miniprogram.go` | 来源过滤、媒体与 HTTP-FLV 直播识别、播放列表缓存 |
| `internal/miniprogram/` | 目标、发现列表、候选存储与下载分类目录 |
| `internal/minidownload/` | 直链下载（含直播录制）、m3u8 与 ffmpeg |
| `third_party/echo/` | 本地 patch 的代理库（MITM listener 回收） |

### 启动与代理

Windows 的 `PrepareConsole` 在初始化前只选终端：优先已安装的 `wt.exe`，已在 Windows Terminal 中则沿用当前窗口，否则保留原控制台；主程序始终以当前用户运行，系统代理写入当前用户 `HKCU`。只有安装根证书或清理旧共享根证书时，才通过 `ShellExecuteExW` 的 `runas` 启动提权子进程，读取退出码后核对信任库，失败及时报错。取舍见 [ADR 0009](docs/adr/0009-user-scoped-windows-runtime.md)，修订 [ADR 0004](docs/adr/0004-windows-terminal-startup.md)。

macOS 通过系统授权管理活动网络服务的 HTTP/HTTPS 代理，捕获原地址后设置、退出恢复；PAC、SOCKS、认证代理或活动 VPN 默认路由冲突时拒绝覆盖，每 2 秒检查默认路由接口变化。私钥存用户 `Library/Application Support` 受限目录，信任安装到当前用户默认钥匙串；签名、公证与真实 Mac 微信验收尚未完成，见 [ADR 0006](docs/adr/0006-macos-portable-distribution.md)。

目标选择卡不启动代理；打开目标覆盖层或确认目标后才启动：先绑定端口，再检查/安装证书、设置系统代理，失败时关闭代理并恢复快照。正常退出恢复原代理；恢复或关闭失败时 TUI 保留错误界面，按 `r` 重试、`f` 强制退出，不静默丢弃。

### 目标与媒体

1. 默认代理 `127.0.0.1:2023`。只有 `https://servicewechat.com/<AppID>/<版本>/page-frame.html` 来源与目标 AppID 完整匹配的媒体才计入**目标候选**并缓存播放列表；来源缺失、格式不符或其他 AppID 的媒体类响应作为**来源未确认**候选（`?`）按类型并入分类，可查看和下载，不写播放列表缓存。取舍见 [ADR 0008](docs/adr/0008-target-attribution.md)。
2. 发现阶段只登记 AppID 与首次/最近出现时间（仅本次运行），按 AppID 去重；目标覆盖层按最近出现排序，选中即确认（v0.2.0 删除小程序名称）。发现阶段不产生目标候选，但照常收集来源未确认媒体，确认目标后显示，首屏页面不会丢失。
3. `s`/`Ctrl+D` 打开目标覆盖层，取消不清空当前目标；确认新 AppID 才清空目标候选与进度，保留文件、历史、部分下载和来源未确认媒体。下载中不允许切换目标；`c` 清空使清空前在途响应失效。
4. 直接响应按 Content-Type 或 URL 后缀识别媒体。FLV（`video/x-flv`、`video/flv`、`application/x-flv` 或 `.flv` 地址）归为视频并保存 `.flv`；无 `Content-Length` 的 FLV 响应或 `.flv` 地址标记为疑似 **HTTP-FLV 直播流**（`Candidate.Live`），真实响应会修正 JSON/H5 的推测。JSON 候选可带标题；跨域链接仅保留 User-Agent、Referer、Accept、Accept-Language，不转发接口 Cookie/Authorization。未知类型且 ≥256 KB 的响应可作「未知文件」兜底。对小程序 H5 页面/脚本（≤4 MB，含 `miniProgram`、`jweixin`、`ckplayer` 或 `<video` 标记）扫描内联媒体地址，作为来源未确认候选（标记「H5 页面/脚本扫描（未播放验证）」）。
5. 直链先写带资源标识的 `.part` 与校验元数据，仅在有强 ETag 时续传；HTTP 分段响应从 Content-Range 取得总长度并校验。默认命名在重试时会按候选标识反查遗留 `.part`，避免时间戳变化导致重新下载全部内容；复用前按 `FLV` 魔数跳过未保存的直播录制，并为新录制生成独立文件名——选名在同一循环同时避开成品与同候选 `.part`（含 `_2` 变体），续传直接使用遗留基名，避免二次改名绕过保护；指定文件名与疑似直播（含 `.flv` 地址）也不复用旧文件。下载时才确认为直播时，产物后缀会修正为 `.flv`。**HTTP-FLV 直播**在响应头与 FLV 魔数确认后持续录制：不发 Range、不校验总长度、状态为 `recording`；服务端结束则正常提交；用户按 `x` 或确认退出时取消上下文，把已录内容提交为 `.flv`（`ErrRecordingStopped`，仍写 `history.jsonl`）。异常断流（`ctx` 未取消时的 `unexpected EOF`、断网等）同样提交已录内容并写历史，返回 `ErrRecordingInterrupted` 并在状态行报告原因；提交失败则保留 `.part` 并原样报错。只有真正的上下文取消才折算为「已停止」，停止录制时的保存失败不会被 `ErrDownloadStopped` 掩盖（普通下载停止保留 `.part`）。成功记录追加到下载根目录 `history.jsonl`，含路径、目标/来源 AppID、类型、归属、来源域名、大小与 UTC 时间。决策见 [ADR 0013](docs/adr/0013-live-stream-recording.md)。
6. m3u8 使用已捕获播放列表，缺失时允许远程回补，由 ffmpeg 合并为 mp4。捕获缓存保留相对引用，下载阶段统一解析；预取按候选与播放列表内容隔离，按来源重放鉴权头。ffmpeg 经仅在下载期间运行的随机端口入口读取缓存，入口只接受已登记路径；跨源重定向不携带凭据。

支持格式：图片 `jpg/png/webp/gif/bmp/avif`；直链视频 `mp4/webm/mov/flv`；直链音频 `mp3/m4a/aac/wav/ogg/flac/opus/amr`。不提供转码、音轨提取或播放。

### 界面、外观与下载目录

TUI 为单主界面：品牌区（真图 logo 或块字符图标、外观分段控件）→ 渐变分隔线 → 主区（目标选择卡，或目标条 + 分类页签 + 候选表格 + 状态/进度行）→ 底栏。目标覆盖层、详情抽屉、日志浮层、退出确认与证书确认都是覆盖层；页签、行、按钮、外观控件支持鼠标，模态覆盖层（退出确认、旧证书确认）的按钮、悬停与取消也响应鼠标，其余区域在模态期间忽略。外观模式（跟随系统/浅色/深色）用 `←/→` 切换，原子写入 `downloads/ui.json`；Windows 读 `HKCU\...\Themes\Personalize\AppsUseLightTheme`，macOS 读 `defaults read -g AppleInterfaceStyle`，失败回退深色，跟随系统每 2 秒轮询。真图 logo 在 Bubble Tea 接管 stdin 前探测（kitty 查询 + CSI 16t + DA1），仅正面回复才绘制内嵌 512×512 PNG，覆盖层/缩放/退出时关闭；否则降级块字符。列表「已下载」标记仅在当前进程有效，详情显示本地路径。直播候选显示「直播流」，录制中行尾显示「录制中」，状态行显示已录大小与 `x` 提示，中断时显示「直播中断并保存」与原因。下载/录制中确认退出会先停止并保存现场；若收尾失败（如保存已录内容失败）会取消自动退出并保留错误界面与 `.part`，处理后可再次退出。取舍见 [ADR 0011](docs/adr/0011-single-main-surface-ui.md)。

下载目录为 `downloads/<目标 AppID>/<图片|视频|音频|m3u8|其他>/`；m3u8 合并产物、播放列表缓存与 `.segments` 分片都在 `m3u8/`。文件名 `类型_YYYYMMDD-HHMMSS_候选ID前8位.扩展名`（m3u8 为 `.mp4`，直播为 `.flv`），冲突追加 `_2`、`_3`。来源未确认候选按下载时目标 AppID 归档；AppID 目录名清洗非法字符并拒绝路径穿越。日志、`history.jsonl`、`ui.json`、根证书、受管 ffmpeg 与 `.ffmpeg/` 日志留在下载根目录；旧版平铺文件不迁移、不删除、不复用。`o`/`Ctrl+O` 优先打开当前目标目录。取舍见 [ADR 0012](docs/adr/0012-per-appid-download-layout.md)。

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
| `target.appID` | 空 | 目标选择卡预填值；空值不表示接收全部媒体 |
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
| 异常退出后网络不通 | 重开 exe → `Ctrl+D` → 等显示“代理运行中”→ `q`；仍异常时在系统代理设置移除本工具残留地址 |
| 证书失败 | 查看错误弹窗并确认 UAC；仍失败可将 `downloads/WxMiniVideoRoot.cer` 安装到根信任库，或启动代理后访问 `http://127.0.0.1:2023/cert` |
| 没有候选或音频 | 核对目标 AppID，按 `1` 看全部、`5` 看音频，重新加载媒体；`?` 媒体不会成为目标候选 |
| 来源未确认媒体 | H5/web-view、原生播放器与 CDN 常无 page-frame Referer；列表上限 500 条，按 `c` 清空，来源不可靠 |
| 直播下载不结束 | 正常：HTTP-FLV 无结束点；按 `x` 停止并保存 `.flv`，需要 mp4 时用 ffmpeg 转封装 |
| 直播断流后内容 | 已录内容会自动保存为 `.flv` 并写历史，状态行/日志给出原因；保存失败时错误与 `.part` 均保留，可重试或手动恢复 |
| 确认退出时保存失败 | 自动取消退出并保留错误界面与 `.part`，`l` 查看日志，处理后可再次按 `q` 退出 |
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

测试覆盖来源过滤与副作用、发现/选择/切换、分类与下载、Windows 启动编排、TUI 布局/鼠标/外观持久化、下载目录分桶与命名、logo 探测与编码，以及 HTTP-FLV 分类、疑似直播标记、chunked 录制到用户停止、异常断流保存、停止时保存失败不被掩盖、真实时钟下的重试续传、未保存录制不被重试覆盖、成品与 `.part` 双重命名冲突、晚确认直播的后缀修正与 `.flv` 命名；鼠标回归覆盖目标卡「自动发现」启动代理与两个模态覆盖层的按钮/悬停；退出回归覆盖保存失败时取消自动退出。修改 `third_party/echo/` 后运行 `go test -count=1 github.com/ltaoo/echo/...`；代理恢复回归使用替身，不修改真实代理或证书。自动化不能替代真实微信、Windows Terminal 交互和 Mac 真机验收。

macOS CI 在 `macos-15-intel` 与 `macos-15` 上运行全量测试、`go vet`、通用包构建（含双架构 ffmpeg）与 `python3 build/verify-macos.py`，设置 `WX_MINI_VIDEO_MACOS_INTEGRATION=1` 时只读调用真实网络服务与钥匙串并验证鉴权 m3u8 合并。打包检查在中文/空格路径解压并启动 TUI，不覆盖 Finder 双击、系统授权、真实微信或 macOS 13 兼容性。

## 分发

Windows 与 macOS 各只发布一个完整包，不提供精简包；两平台附件在同一 Release 并列提供并标明状态。

```powershell
cmd /c build\build.bat windows          # dist\wx-mini-video-windows-amd64\ 与同名 zip
go run ./build/macos                    # dist\wx-mini-video-macos-universal-unsigned.zip
```

Windows 包内精确包含 `README.md`、`wx-mini-video.exe`、`wx-mini-video.yaml`、`ffmpeg\ffmpeg.exe`、`ffmpeg\LICENSE.txt`、`THIRD-PARTY-NOTICES.md`；macOS 通用包包含 `.app`、配置、说明、`.app` 外双架构 `ffmpeg/` 与许可声明。构建脚本联网下载并校验 ffmpeg，缓存于 `dist/ffmpeg-cache/`；`WXMV_FFMPEG_DIR` 可改用自建目录。签名/公证只覆盖 `.app`，Mac 正式发布仍需 Developer ID、公证和双架构真机验收，见 `build/sign-macos.sh` 与 [ADR 0006](docs/adr/0006-macos-portable-distribution.md)。

发布前完成检查并提交，确认 exe 的 `vcs.revision` 与提交一致、`vcs.modified=false`；上传前运行 `python3 build/verify-release-sources.py`，核对 zip SHA-256、最新版本标记和既有附件未变。随包 ffmpeg 为 LGPLv3 独立程序，网络分发须在同一 Release 提供对应源码归档与许可全文，见 `THIRD-PARTY-NOTICES.md`。推送、创建 Release、上传附件须有明确授权。

## 真实 PC 微信验收

当前只确认普通用户 Windows Terminal 中显示正常；证书提权、标准用户 + 独立管理员、真实微信音频、来源覆盖、Mac 真机仍待验收。

1. 双击启动，确认 Terminal 选择、中文/空格路径、首次证书 UAC 与取消提示；发现两个小程序时确认去重、未选择前无候选。
2. 分别用手动输入和 `Ctrl+D` 选目标，重新加载图片、直链视频、直链音频、m3u8 和 HTTP-FLV 直播；核对分类、标题、后缀、原文件保存、`✓`/`?` 与严格过滤。
3. 其他小程序媒体不进入目标候选；来源未确认媒体允许出现但标记 `?`；JSON 图片/视频/音频可出现，JSON m3u8 继续跳过。
4. `s` 打开覆盖层，`Esc` 保留当前目标与候选；确认新目标后清空候选、保留文件；下载中切换被拒绝。
5. 退出并核对代理恢复；异常退出按排障恢复；证书不随退出卸载；验收记录不含 Cookie、鉴权头或带签名 URL。
6. v0.2.0：检查品牌区真图/块字符、浅/深/跟随系统与重启保留、鼠标、详情抽屉、日志浮层、下载中退出确认；核对 `downloads/<AppID>/<类型>/`、文件名、`history.jsonl` 与 `o` 目录。
7. HTTP-FLV：候选显示「直播流」与识别依据；录制中显示已录大小、行尾「录制中」；`x` 停止后保存 `.flv` 且写入历史；录制中退出先停止保存再恢复代理；不再出现 `.flv.mp4`；制造断流（关网络或断开响应）后确认已录内容自动保存、状态行提示原因、`history.jsonl` 有记录；人为让最终文件保存失败时应看到真实保存错误且 `.part` 保留，确认退出时应取消退出并保留错误界面；重试不得覆盖上一次未保存的直播录制；只有下载时才确认的直播应保存为 `.flv`。
8. 重试与续传：直链下载中断后隔几秒再下载，确认复用原 `.part` 并发送 Range（服务器提供 ETag 时），而不是从头下载；直播重试应生成新文件，不覆盖上一次录制。
