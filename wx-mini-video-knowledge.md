# 维护指南

用户操作见 [README.md](README.md)，术语见 [GLOSSARY.md](GLOSSARY.md)，发布历史见 [CHANGELOG.md](CHANGELOG.md)。本文件集中记录实现、配置、构建、排障和验收；已完成计划与原始规格保存在 [docs/archive/](docs/archive/)。

## 架构

Windows 优先的单二进制 Go TUI，使用 Bubble Tea 和 Echo 代理适配。代码入口：

| 路径 | 职责 |
| --- | --- |
| `main.go`、`pkg/system/console_windows.go` | Terminal 启动、提权子进程入口与启动错误提示 |
| `pkg/certificate/` | 根证书生成/复用、信任检查、证书安装与旧证书清理（含 Windows 提权子进程） |
| `internal/app/` | 代理、目标、候选和下载器的运行时编排 |
| `internal/tui/` | 单主界面与品牌区、目标选择、媒体视图、主题与外观、鼠标命中、下载与进度 |
| `internal/tui/ui/` | 外观模式与 `ui.json`、系统明暗、主题色板、布局度量与显示宽度 |
| `internal/tui/logo/` | 终端图片能力探测、Sixel/kitty 编码、每帧绘制与清理（失败静默降级） |
| `internal/interceptor/miniprogram.go` | 请求来源过滤、媒体与 HTTP-FLV 直播流识别、播放列表缓存 |
| `internal/miniprogram/` | 目标、发现列表与候选存储（含直播标记） |
| `internal/minidownload/` | 直链下载（含直播录制）、m3u8 与 ffmpeg（随包/受管） |
| `third_party/echo/` | 本地 patch 的代理库（MITM listener 回收） |

### 启动与代理

macOS 平台实现、便携通用测试包与后续签名/公证入口见 [macOS 测试版说明](docs/macos-usage.md)，需求与待验收范围见 [macOS 版本需求](docs/macos-requirements.md)。在仓库根目录运行 `go run ./build/macos`，生成同一 ZIP 中的双架构 `.app`、外部配置与说明、`.app` 外的双架构 `ffmpeg/`、GPLv3 全文与第三方声明，保留可执行权限，不打包下载数据。当前仅提供未签名测试包；用户要求先完成本地适配，真实 Mac 微信验收后续进行。

macOS 通过普通用户进程调用系统授权提示管理活动网络服务的 HTTP/HTTPS 代理，捕获原地址与开关后设置，退出恢复；PAC、SOCKS、认证代理或活动 VPN 默认路由冲突时拒绝覆盖。每两秒检查活动默认路由接口，变化后停止嗅探、恢复代理，按 `r` 重启；同一接口内热点切换不保证识别。私钥使用用户 `Library/Application Support` 下的受限持久目录，信任安装到当前用户默认钥匙串；签名、公证及系统授权的实际行为均须在 Mac 验收。

Windows 的 `PrepareConsole` 在配置、证书、代理初始化前只处理终端选择：优先使用已安装的 `wt.exe`，已在 Windows Terminal 中则沿用当前窗口，找不到 Terminal 时保留原控制台；不再整体提权。主程序始终以运行微信的当前用户运行，系统代理写入当前用户 `HKCU`。只有安装根证书或清理旧共享根证书时，才通过 `ShellExecuteExW` 的 `runas` 启动本程序的提权子进程（`RunHelper`）；授权后保留进程句柄并等待结束（最长两分钟），读取退出码后检查信任库确认结果。失败退出或成功退出但证书状态不符会及时报错，不再反复轮询至超时；子进程弹窗展示具体原因，取消 UAC 单独提示。两分钟上限不包含用户处理 UAC 的时间；真正超时会提示子进程可能仍在运行。重启保留工作目录、参数和 `WX_MINI_VIDEO_CONFIG`。取舍见 [ADR 0009](docs/adr/0009-user-scoped-windows-runtime.md)，修订 [ADR 0004](docs/adr/0004-windows-terminal-startup.md)。

目标选择卡尚未启动代理；打开目标覆盖层或确认目标后才启动。服务先绑定监听端口，再检查和安装证书、设置系统代理；启动失败关闭已创建的代理并恢复已捕获的系统代理快照。没有快照时不改动系统代理，恢复失败会保留状态供再次清理，并将错误返回调用方。

正常退出恢复原代理，包括地址中含空格的多协议配置；如果原地址的每一项都完整匹配本程序监听地址，则视为异常退出残留，退出时禁用它。其他端口或混有其他地址的配置保留原样。恢复或关闭失败时 TUI 保留错误界面，按 `r` 重试，按 `f` 明确强制退出；最终清理错误也会显示，不静默丢弃。

### 目标与媒体

1. 默认代理为 `127.0.0.1:2023`。从 HTTPS `servicewechat.com/<AppID>/<版本>/page-frame.html` 格式的 `Referer` 解析请求来源 AppID，完整匹配目标后才计入目标候选并缓存媒体；来源缺失、格式不符或其他 AppID 的媒体类响应作为**来源未确认**候选（`?` 标记）按类型并入候选列表（不写播放列表缓存、不自动下载），用户可查看和下载。目标候选不通过共享 CDN 或跨请求 URL 关联扩大范围。
2. 发现阶段只登记 AppID（含首次出现与最近出现时间，仅本次运行保存），按 AppID 去重；目标覆盖层按最近出现时间倒序展示，选中即确认，不再有名称补填步骤（v0.2.0 删除小程序名称功能）。发现阶段不产生目标候选、不缓存播放列表，但来源未确认媒体照常收集（与目标无关），确认目标后按类型显示（`?` 标记）；这样打开小程序时自动加载的第一个页面不会因目标尚未确认而丢失。
3. `s`/`Ctrl+D` 打开目标覆盖层；取消不会清空当前目标或候选，确认新 AppID 后才清空候选及进度，保留文件、历史、部分下载和缓存。存储用目标代次拒绝旧目标响应；来源未确认媒体使用独立清空代次，确认或切换目标不失效，清空列表使在途结果失效，提交候选和写缓存时再次检查；按 `c` 清空也会使清空前尚未接纳的响应失效，保留当前目标和本地文件。下载中不允许切换目标。
4. 直接响应按 Content-Type 或 URL 后缀识别媒体。FLV（`video/x-flv`、`video/flv`、`application/x-flv` 或 `.flv` 地址）按视频识别并保存 `.flv`；没有 Content-Length 的 FLV 响应或 `.flv` 地址标记为疑似 **HTTP-FLV 直播流**（`Candidate.Live`），真实响应会修正 JSON/H5 推测。匹配目标的 JSON 接口可提供图片、直链视频和直链音频；JSON m3u8 链接跳过，等待实际播放列表响应。JSON 提取的跨域链接仅保留 User-Agent、Referer、Accept、Accept-Language，不转发接口 Cookie、Authorization 或其他自定义鉴权头；同源指 scheme 与 host（含端口）一致。后续真实媒体响应会更新其重放请求头与媒体元数据，保留 JSON 标题。URL 所在对象的 `title`、`videoTitle`、`audioTitle`、`mediaTitle`、`caption`、`subject`、`name` 可作为候选标题。JSON 另支持相邻字段拼接（如 `url` + `urlToken`）、无后缀但字段名强相关的媒体地址；目标 AppID 已匹配时，`application/octet-stream` 等未知类型且不小于 256 KB 的响应作为“未知文件”兜底候选，不自动缓存或下载。另对小程序的 H5 页面/脚本（HTML/JS，≤ 4 MB，含 `miniProgram`、`jweixin`、`ckplayer` 或 `<video` 标记）扫描内联的视频、m3u8 与音频地址，作为来源未确认候选按类型显示；Referer 取页面自身；仅同源媒体复用页面 Cookie/Authorization，跨源只保留公开请求元数据，真实媒体响应的鉴权头不会被后续页面扫描覆盖，因此打开详情页即可出现候选，不依赖播放器真正发请求。扫描候选标记「H5 页面/脚本扫描（未播放验证）」，下载前未验证地址是否仍有效。
5. 直链先写带资源标识的 `.part` 和校验元数据；仅有强 ETag 时以 Range/If-Range 续传，服务器返回新实体时重下，没有校验依据时也重下。旧版同名 `.part` 保留但不复用。HTTP 分段响应从 Content-Range 取得完整长度；提交前核对响应声明的长度，不再凭嗅探长度或已有部分大小直接完成。**HTTP-FLV 直播流**（`.flv` 地址或 `video/x-flv` 响应且没有 Content-Length）在响应头与 FLV 魔数确认后按持续录制处理：不发 Range、不校验总长度，按 `recording` 状态持续写入 `.part`；用户按 `x` 或确认退出时取消下载上下文，已录内容提交为 `.flv`（返回 `ErrRecordingStopped`，仍写入 `history.jsonl`）；服务端结束或断开则正常提交。普通下载被用户停止时保留 `.part`（返回 `ErrDownloadStopped`）。成功记录追加到下载根目录的 `history.jsonl`，包含路径、目标 AppID（`app_id`）、请求来源 AppID、资源类型、归属、来源域名、大小和 UTC 时间。
6. m3u8 使用已捕获播放列表，缺失时允许远程回补，最后由 ffmpeg 输出 mp4；音频 m3u8 也沿用此流程。捕获缓存保留原始相对引用，在下载阶段统一解析；预取缓存按候选及播放列表内容隔离，避免更新后的播放列表误用原位置的分片，并按来源限定请求头；ffmpeg 获取分片时，经仅在下载期间运行的 `127.0.0.1` 随机端口入口读取缓存播放列表、分片及子播放列表，由 Go 按来源重放鉴权头，入口仅接受带随机标识且已登记的资源路径。主播放列表沿用此方式获取其子播放列表，不作为普通分片预取。跨源重定向同样不携带凭据。

支持格式：图片 `jpg/png/webp/gif/bmp/avif`；直链视频 `mp4/webm/mov/flv`；直链音频 `mp3/m4a/aac/wav/ogg/flac/opus/amr`。图片和直链媒体保存原文件；不提供转码、音轨提取或播放功能。归属取舍见 [ADR 0008](docs/adr/0008-target-attribution.md)，替代 [ADR 0003](docs/adr/0003-target-appid-filtering.md)。

### 界面与外观（v0.2.0）

TUI 为单主界面：品牌区（品牌图标、kicker/标题/副标题、右侧外观分段控件）→ 渐变分隔线 → 主区 → 底栏（GitHub 链接、退出按钮）。未选目标时主区是目标选择卡（AppID 输入、开始嗅探、自动发现）；已选目标后是目标条（AppID、代理、ffmpeg 状态、切换）、分类页签、候选表格与状态/进度行。目标覆盖层、详情抽屉、日志浮层、退出确认、旧证书确认都是覆盖层；页签、表格行、按钮和外观控件支持鼠标点击，键盘操作全部保留。

- 外观模式：跟随系统/浅色/深色，`←/→` 或点击切换，立即生效；偏好原子写入下载目录的 `ui.json`。Windows 读 `HKCU\...\Themes\Personalize\AppsUseLightTheme`，macOS 读 `defaults read -g AppleInterfaceStyle`，读取失败回退深色；跟随系统时每 2 秒轮询。
- 品牌 logo：启动、Bubble Tea 接管 stdin 之前探测终端图片能力（kitty 图形查询 + CSI 16t 像素格 + DA1）；只有收到正面回复才绘制内嵌 512×512 PNG（Sixel/kitty），输出层每帧补画，覆盖层打开、缩放与退出时关闭并清理。探测失败、`NO_COLOR`、tmux/screen、Apple Terminal、非 TTY 或窗口过矮时降级为块字符品牌图标；真图不改变布局高度上限（5 行）。可在真实终端运行 `go run ./build/logoprobe` 验证，重建图标运行 `go run ./build/genlogo`。
- 已下载候选在当前进程内以行尾「已下载」标记（需宽度 ≥84 才显示状态列），详情抽屉显示本地路径；重复下载生成新文件。下载中退出需要二次确认，已完成文件保留。
- 直播流识别：列表「类型」列与详情显示「直播流」，详情给出「HTTP-FLV 直播流（URL/响应识别）」依据；下载时状态行显示「录制直播中 …（不会自动结束，按 x 停止并保存）」，行尾状态显示「录制中」。`x` 停止当前下载：直播录制取消后保存已录内容并标记已下载，普通下载保留 `.part`；下载中按 `q` 确认退出会先停止下载/录制、保存现场再恢复代理退出。
- 运行日志、debug 日志、`history.jsonl`、`ui.json`、根证书、受管 ffmpeg 与 `.ffmpeg` 失败日志仍在下载根目录；媒体、m3u8 缓存/分片和 `.part` 跟随目标目录。

### 下载目录与命名（v0.2.0）

`downloads/<目标 AppID>/<类型>/`，类型目录为 `图片`、`视频`、`音频`、`m3u8`、`其他`（未识别二进制）。m3u8 合并产物、播放列表缓存 `<候选ID>.m3u8` 与 `.segments` 分片都放在 `m3u8/`；合并成功的分片清理语义不变。文件名默认 `类型_YYYYMMDD-HHMMSS_候选ID前8位.扩展名`，本地时区，m3u8 合并产物固定 `.mp4`，HTTP-FLV 直播录制保存 `.flv`；同名冲突追加 `_2`、`_3`。目标候选按其 AppID 归档；来源未确认候选按下载时当前目标 AppID 归档，`history.jsonl` 同时记录请求来源 AppID。AppID 目录名做安全清洗并拒绝路径穿越，保留大小写。旧版平铺在下载根目录的媒体、`.part` 与缓存不迁移、不删除、不复用。`o`/`Ctrl+O` 优先打开当前目标目录，缺失或未选目标时打开下载根目录。取舍见 [ADR 0012](docs/adr/0012-per-appid-download-layout.md)。

## 配置

完整模板见 [internal/config/config.template.yaml](internal/config/config.template.yaml)。发布包默认读取 exe 旁的 `wx-mini-video.yaml`；`WX_MINI_VIDEO_CONFIG` 可指定其他配置文件。修改后重启程序。

| 配置 | 默认值 | 含义 |
| --- | --- | --- |
| `download.dir` | 空 | 配置目录下的 `downloads/`；普通发布包即 exe 旁。显式相对路径按当前工作目录解析，建议使用绝对路径。 |
| `download.ffmpeg` | 空 | 优先级最高；指定本地可执行文件完整路径。为空时依次使用随包 ffmpeg（完整包）和受管 ffmpeg。旧值 `ffmpeg` 视为空，不查找系统 PATH。 |
| `download.mode` | `auto` | 媒体播放列表分片数 ≥20 时并发预取，否则由 ffmpeg 经本机入口获取；`prefetch` 强制预取媒体分片，`remote-ffmpeg` 由 ffmpeg 获取。主播放列表始终由 ffmpeg 选择子播放列表。 |
| `capture.showUnattributed` | `true` | 是否收集来源未确认媒体（以 `?` 标记并入类型分类）。关闭后来源未确认媒体既不收集也不显示。 |
| `download.segmentConcurrency` | `6` | 预取分片并发数 |
| `download.segmentRetries` | `3` | 分片失败重试次数 |
| `download.keepSegments` | `false` | 合并成功后是否保留分片缓存 |
| `proxy.system` | `true` | 设置并恢复 Windows 或 macOS 系统代理（macOS 待真机验收） |
| `proxy.hostname`、`proxy.port` | `127.0.0.1`、`2023` | 本地监听地址 |
| `proxy.skipInstallRootCert` | `false` | 跳过根证书检查和自动安装；不替代证书信任配置 |
| `proxy.upstreamProxy` | 空 | 上游代理地址 |
| `target.appID` | 空 | 目标选择卡的预填值；空 AppID 不表示接收全部媒体 |
| `cert.file`、`cert.key` | 空、空 | 必须成对配置的 PEM 证书与 RSA 私钥路径；显式配置优先，无配置时生成独立证书，不再自动借用 mitmproxy 证书 |
| `cert.name` | `Echo` | 仅兼容保留；信任检查使用证书指纹，不再使用此名称 |

下载目录也保存缓存、日志、外观偏好 `ui.json`、仅含公开证书的 `WxMiniVideoRoot.cer` 和受管下载的 ffmpeg；随包 ffmpeg 位于程序旁的 `ffmpeg/`，私钥单独保存于本机用户目录。移动便携目录时一并保留 `downloads/`；使用外部配置或显式目录时需另行保留该目录。取舍见 [ADR 0002](docs/adr/0002-portable-runtime-data.md)、[ADR 0005](docs/adr/0005-local-ca-lifecycle.md) 和 [ADR 0012](docs/adr/0012-per-appid-download-layout.md)。

ffmpeg 查找优先级为显式 `download.ffmpeg`、随包 ffmpeg、受管 ffmpeg。随包版本：Windows 与 macOS 均为固定官方源码自建（FFmpeg 9.0.2 + OpenSSL 3.5.9，LGPLv3，静态链接），Windows 为 `ffmpeg\ffmpeg.exe`，macOS 为 `.app` 外 `ffmpeg/ffmpeg-amd64`、`ffmpeg/ffmpeg-arm64`，不做 lipo。随包文件做存在/可执行检查，并以 3 秒超时执行 `-version` 探测；缓存路径、大小和修改时间对应的结果，替换文件后重测；缺失或损坏时回退受管下载。受管下载使用候选源列表：Windows 首选 gyan 9.0.2、备用 BtbN 2026-09-30 月末 8.1 LGPL；macOS 沿用 Martin 两个地址。每个源固定 URL 与 SHA-256，可换源重试；旧 marker 因摘要变化自动失效。版本和摘要在 `internal/minidownload/ffmpeg.go`，更新时需同时修改。自建流程与对应源码见 [ADR 0010](docs/adr/0010-source-built-ffmpeg.md) 和 `build/ffmpeg/README.md`。

## 证书风险与清理

默认在首次启动代理前，为当前 Windows 用户在本机生成独立 RSA 根证书与私钥（有效期五年），保存到 `%LOCALAPPDATA%\wx-mini-video\certificates\root-ca.pem` 并复用。目录及文件使用受保护的 Windows ACL，仅当前账户和 SYSTEM 获得访问权限；同一账户运行的其他进程以及具有提升权限的管理员不因此被完全隔离。证书与私钥损坏、过期或不匹配时停止启动，不覆盖原文件或静默换证书。升级、移动 exe 不改变证书；换电脑或 Windows 用户后需重新生成并安装信任，私钥不随便携包移动。

显式 `cert.file`/`cert.key` 优先，必须成对配置且证书具有 CA 签发能力、私钥匹配并可被 Echo 使用；无配置时使用本工具生成的证书。不再自动借用 mitmproxy 证书，需要复用时显式指定。证书安装、保存和公开证书导出失败都会停止启动；信任检查比对实际证书的 SHA-1 指纹，不按名称判断。自动安装通过提权子进程执行 `certutil -addstore Root` 写入 Windows 本地计算机根信任库，影响不限于目标小程序；首次安装或清理旧证书时会请求管理员授权，退出只恢复代理，不卸载证书。

**历史版本的默认 SunnyNet 根证书使用公开共享私钥。新版不再内置该证书对，但仅替换 exe 不能撤销旧系统信任。** 启动代理前检查本机与当前用户根证书库中的固定旧指纹，TUI 说明可能影响其他依赖者后，按 `y` 明确确认精确删除；按 `n` 保留并继续，取消或删除失败时持续提示风险。安装独立证书不会自动撤销旧证书的信任。旧版私钥已公开这一事实也不会随源文件删除而消失。

停用本工具并确认其他代理工具不再依赖该证书后，可手动清理：

1. 先正常退出程序，确认系统代理已恢复。
2. 运行 `certlm.msc`（本机）和 `certmgr.msc`（当前用户），分别打开“受信任的根证书颁发机构”→“证书”。
3. 旧共享证书名称为 `SunnyNet`，SHA-1 指纹为 `D70CD039051F77C30673B8209FC15EFA650ED52C`。核对指纹后仅删除这一项，不按名称批量删除。
4. 新版独立证书及显式自定义证书须核对各自实际指纹和其他使用方；不要套用旧共享证书的删除步骤。

上述操作由用户按需执行；项目自动化测试不安装或删除系统证书。

## 排障

| 现象 | 检查与处理 |
| --- | --- |
| 异常退出后网络不通 | 重开 exe → `Ctrl+D` → 等待“代理运行中”→ `q`。输入界面直接退出不会触发恢复；若启动失败，检查代理端口，或在 Windows 代理设置中核对并移除本工具残留地址。 |
| 启动提示“读取原系统代理失败” | 本版已改为直接读取注册表并把缺失值视为未配置；若仍出现，确认没有运行旧版。主程序以当前用户运行，代理写入该用户 HKCU；只有证书安装/清理会请求管理员授权。 |
| 证书失败 | 查看证书操作错误弹窗的具体原因并关闭弹窗；取消 UAC 会单独提示。子进程失败退出后立即报告；若提示等待超时，先确认子进程或错误弹窗是否仍在运行。理解上节风险后，可手动将下载目录的 `WxMiniVideoRoot.cer` 安装到根信任库；代理运行时也可访问 `http://127.0.0.1:2023/cert` 获取证书（改端口后相应调整）。 |
| 没有候选或音频 | 核对目标 AppID，按 `1` 查看全部、`5` 查看音频，再重新加载媒体。缺失匹配 Referer 的媒体会以 `?` 标记按类型显示，不会成为目标候选；手动输入不能绕过。 |
| 来源未确认媒体 | H5/web-view、原生播放器和 CDN 媒体常无小程序 page-frame Referer，会以 `?` 标记混入对应类型分类；下载沿用真实响应的 Referer/Cookie。来源未确认列表上限 500 条，按 `c` 清空。代理运行期间本机其他微信内容（如公众号文章图片）也可能出现，来源不可靠。 |
| 需要诊断嗅探 | 以 `wx-mini-video.exe --debug` 启动（参数会随 Terminal 重启转发，不依赖环境变量继承），日志写入 `下载目录\wx-mini-video-debug.log`；记录归属判断与丢弃原因，URL query 与 Cookie 值不落盘，分享前仍需检查。环境变量 `WX_MINI_VIDEO_DEBUG=1` 仍可直接使用。 |
| 资源过多 | 按 `c` 清空，再打开目标内容；使用分类缩小列表。 |
| 中文缺失 | 使用 Windows Terminal，或给传统控制台选择支持中文的字体。程序不自动安装 Terminal 或更改默认终端。 |
| m3u8 预取失败较多 | 将 `download.segmentConcurrency` 从 `6` 调低到 `4`，重新播放后重试。 |
| m3u8 合并失败 | 先确认 `ffmpeg/` 未被隔离或删除；随包 ffmpeg 缺失或损坏时程序会自动下载受管 ffmpeg，也可指定 `download.ffmpeg`。查看错误给出的 `下载目录\.ffmpeg\<候选ID>.log`。日志会移除已捕获请求头值；分享前仍需检查 URL 中的签名和其他敏感信息。 |

媒体界面补充按键：`1/2/3/4/5` 分类切换，`r`/`Ctrl+R` 刷新或重试代理，`PgUp/PgDn` 翻阅日志，`Home/End` 到日志开头/末尾，`i` 详情抽屉，`l` 日志浮层，`Esc` 关闭覆盖层；`Ctrl+O` 打开下载目录，`Ctrl+Q` 退出（下载中二次确认）。目标选择卡 `Ctrl+D` 打开目标覆盖层，`Esc` 退出程序。

## 开发验证

在仓库根目录执行：

```powershell
go test -count=1 ./...
go vet ./...
git diff --check
```

测试覆盖来源过滤及副作用、发现/选择/切换、分类和下载、Windows 启动编排与参数传递。v0.2.0 起另覆盖 `internal/tui/ui` 的布局度量/显示宽度/外观偏好读写、`internal/tui` 的鼠标命中与目标覆盖层选择/退出确认、`internal/miniprogram` 与 `internal/minidownload` 的下载目录分桶和文件命名、`internal/tui/logo` 的能力探测解析与 Sixel/kitty 编码。HTTP-FLV 直播流回归覆盖 FLV 分类/疑似直播标记、interceptor 响应判定、chunked 直播录制到用户停止、有限 FLV 不被误判、`.flv` 单后缀命名，以及 TUI 的「直播流」类型/录制中状态/停止与退出流程。代理库 patch 的回归测试不在根模块的 `./...` 中，修改 `third_party/echo/` 后需运行 `go test -count=1 github.com/ltaoo/echo/...`。代理恢复回归使用替身，不更改真实系统代理或证书。自动化结果不能替代下面的真实微信验收，也不能替代真实终端中的图片显示与鼠标交互验收。

可靠性回归还覆盖独立证书生成及复用、私钥 ACL、显式配置失败、指纹匹配、迁移确认、退出重试、完整代理快照、同名资源隔离、实体校验续传和鉴权头来源隔离。机器已安装 ffmpeg 时，测试会生成短音频分片并以本机 HTTP 服务验证预取与 ffmpeg 两种鉴权 m3u8 合并；未安装时跳过这一实际合并用例，仍执行播放列表入口测试，不自动下载 ffmpeg。新增回归覆盖随包 ffmpeg 路径与优先级、来源未确认媒体/未知二进制兜底、JSON 字段拼接、debug 日志脱敏、echo listener 回收、分片 `.part`/`.done` 复用判定。

GitHub Actions 的 [macOS 工作流](.github/workflows/macos.yml) 在 `macos-15-intel` 与 `macos-15` 上分别核对 amd64 / arm64 原生环境，运行全量测试、`go vet`、通用包构建（含随包双架构 ffmpeg）及 `python3 build/verify-macos.py`，并校验包内 ffmpeg 条目与权限。推送 main、PR 或手动 `workflow_dispatch` 触发；测试包保存为 7 天 Actions 附件，不自动替换 Release。

Mac Release 更新时，下载通过验证的 Actions 原始通用 ZIP，不重新构建或重新压缩；核对两种架构任务产物的 ZIP 摘要及包内可执行文件构建来源，再上传并回下载复核。若两份产物不一致，先查明差异，不能据其中一个任务通过就声称同一附件已通过双架构验证。替换前备份旧附件与说明，发布页同步更新 Mac 构建提交、CI 链接和 SHA-256。

工作流另设置 `WX_MINI_VIDEO_MACOS_INTEGRATION=1`，运行 `go test -v -count=1 -timeout=10m -run '^TestDarwin' ./pkg/system ./pkg/certificate ./internal/minidownload`：真实调用 `route`、`networksetup` 和 `security` 读取网络服务及钥匙串，检查临时私钥权限，下载并校验固定版本 ffmpeg，实际执行鉴权 m3u8 合并。网络服务和信任库只读，不安装证书或更改代理；代理写入/恢复仍由替身测试覆盖。普通测试不启用该变量，不额外下载 ffmpeg。

打包检查在含中文与空格的临时目录解压应用，通过伪终端进入 AppID 输入界面后退出，并移动目录后复测；不选择目标或启动代理。它不能验证 Finder 双击、下载隔离与 App Translocation、Terminal 自动化授权、证书/管理员弹窗、真实微信流量或 macOS 13 兼容性，以上仍需人工验收。

## 分发

ffmpeg 自建迁移：已采用固定 FFmpeg/OpenSSL 官方源码自行构建，见 [ADR 0010](docs/adr/0010-source-built-ffmpeg.md) 和[构建说明](build/ffmpeg/README.md)。`.github/workflows/ffmpeg.yml` 为手动三架构构建并归档二进制、源码和验证记录；v0.1.9 随包 ffmpeg 已切换到自建版本，对应源码归档与二进制在同一 Release 提供。

Windows 与 macOS 均只发布一个完整包，不提供不含 ffmpeg 的精简包。Windows 稳定包与 macOS 未签名测试包在同一 Release 并列提供，各自标注平台状态（当前版本 v0.2.0）。macOS 在仓库根目录执行 `go run ./build/macos`，产物为 `dist/wx-mini-video-macos-universal-unsigned.zip`；通用 `.app` 同时包含 amd64 与 arm64，包内包含应用、外部配置和说明、`.app` 外的 `ffmpeg/ffmpeg-amd64`、`ffmpeg/ffmpeg-arm64`、`ffmpeg/LICENSE.txt` 与 `THIRD-PARTY-NOTICES.md`，保留执行权限，不包含下载数据。签名与公证只覆盖 `.app`，包外 ffmpeg 未签名；用户须保留整个解压文件夹。

共用 Release 入口不改变 Mac 的测试状态；正式 Mac 支持仍须完成 `build/sign-macos.sh` 的 Developer ID 签名、公证及双架构真机验收。后补附件不移动既有发布标签，也不替换已发布的 Windows 附件；发布说明须按平台记录各自的实际构建提交与 SHA-256，不能将 Mac 构建来源写成原 Windows 标签提交。上传前确认两种薄 Mach-O 的 `vcs.revision` 均等于发布说明中的 Mac 构建提交、`vcs.modified=false`；上传后下载核对 SHA-256，确认最新版本标记正确、两平台附件均存在且既有 Windows 附件未变化。当前基础版本号见 `main.go`，版本历史见 CHANGELOG。

```powershell
cmd /c build\build.bat windows
```

`windows` 构建唯一 Windows 包（默认联网下载并校验 gyan ffmpeg，缓存于 `dist/ffmpeg-cache/`；设置 `WXMV_FFMPEG_DIR` 可改用自建 ffmpeg 目录），产物为 `dist\wx-mini-video-windows-amd64\` 和同名 zip，包内精确包含 `README.md`、`wx-mini-video.exe`、`wx-mini-video.yaml`、`ffmpeg\ffmpeg.exe`、`ffmpeg\LICENSE.txt`、`THIRD-PARTY-NOTICES.md`。脚本执行 `build\verify-package.ps1`。README 的图片和扩展文档使用在线链接；离线时仍可按文字步骤操作。`dist/`、下载数据和临时产物不提交。

随包 ffmpeg 为 LGPLv3 独立程序（自建，静态 OpenSSL）：发布前确认 `THIRD-PARTY-NOTICES.md` 与包内 `ffmpeg/LICENSE.txt` 已提交并随包；网络分发按 GPLv3 §6(d) 在同一 Release 提供对应源码归档，不能用实物分发的书面要约代替。三架构源码索引已填写并人工核对，公开上传任何平台附件前必须运行 `python3 build/verify-release-sources.py`，实际下载验证源码归档摘要。项目代码仍为 MIT + Commons Clause，两者分别标识，不能把 Commons Clause 施加到 ffmpeg。

发布前完成检查并提交，再构建，核对 exe 的 `vcs.revision` 与目标提交一致、`vcs.modified=false`。推送、创建 Release 和上传附件须有明确授权；上传后核对 zip 的 SHA-256 与最新版本标记。合并发布先备份旧说明和附件，新包验证后再删除被替代的 Release，保留 Git 标签；具体版本历史只记在 CHANGELOG。

## 真实 PC 微信验收

当前只确认过普通用户 Windows Terminal 中显示正常；证书提权子进程、标准用户 + 独立管理员账号、真实微信音频及请求来源覆盖仍待人工验收。macOS 已列为后续真机验收目标，当前尚未验收，检查项见 [macOS 测试版说明](docs/macos-usage.md)。

1. 双击启动，确认在 Windows Terminal 中运行；已在 Terminal 中不重复开窗；无 Terminal 时保留当前控制台。检查中文/空格路径和显式配置文件。首次安装根证书时应出现一次 UAC；取消后应给出可理解错误，重新启动可再次请求。发现两个小程序，确认 AppID 去重，未选择前无媒体候选或缓存。
2. 分别用手动输入和 `Ctrl+D` 目标覆盖层选择目标（选中即确认，不再补填名称），重新加载图片、直链视频、直链音频、m3u8 和 HTTP-FLV 直播流。检查分类、标题、后缀、原文件保存和 m3u8 输出；核对 Referer 与目标一致，来源缺失的媒体应以 `?` 标记按类型显示并可下载，记录 `✓`/`?` 标记，不放宽目标过滤。
3. 同时操作其他小程序，确认其媒体不进入目标候选；来源未确认媒体允许出现其他来源内容，但应标记 `?` 并说明来源不可靠。匹配 JSON 的图片、直链视频和音频可出现，JSON m3u8 继续跳过。
4. 按 `s` 打开目标覆盖层，先按 `Esc` 确认当前目标和候选保留；再切换到新目标，确认旧候选和进度清空、本地文件保留，旧响应不重新填充列表。下载中切换应被拒绝且下载继续。
5. 从发现和媒体界面退出，核对原代理恢复；另检查异常退出后按排障步骤恢复。证书不会随退出卸载。验收记录不包含 Cookie、鉴权头值或带签名的完整媒体 URL。
6. 确认 m3u8 首次下载无需联网准备 ffmpeg；删除 `ffmpeg/` 后确认自动回退受管下载并给出可理解提示。
7. 设置 `WX_MINI_VIDEO_DEBUG=1` 后确认生成 `wx-mini-video-debug.log`，其中不含 URL query 或 Cookie 值。
8. 在“标准用户运行微信 + 用另一个管理员账号提权”的机器上验证：程序以普通用户启动；首次安装根证书出现 UAC 后代理作用于当前用户；微信流量可被捕获；退出后代理恢复到该用户的原始设置。同时验证“同账号管理员”机器只需一次 UAC。
9. v0.2.0 界面与目录：在 Windows Terminal 中检查品牌区/真图或块字符 logo、浅色/深色/跟随系统切换与重启保留、鼠标点击页签与行、`i` 详情抽屉、`l` 日志浮层、下载中退出确认；下载图片/视频/音频/m3u8 后核对 `downloads/<AppID>/<类型>/` 结构、文件名、`history.jsonl` 新字段与 `o` 打开的目标目录；旧平铺文件保持原样。
10. HTTP-FLV 直播流：在直播页面确认候选类型显示「直播流」、详情依据为「HTTP-FLV 直播流（URL/响应识别）」；按 `d` 后状态行显示「录制直播中 …（不会自动结束，按 x 停止并保存）」、行尾「录制中」，已录大小持续增长；按 `x` 停止后文件以 `.flv` 保存、`.part` 已提交、`history.jsonl` 有记录；录制中按 `q` 确认退出应先停止并保存再恢复代理；确认同一 `.flv` 不再生成 `.flv.mp4`。HLS 直播不在本次范围，m3u8 行为保持原样。
