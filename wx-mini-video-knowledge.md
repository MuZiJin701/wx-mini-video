# wx-mini-video 基础知识 / Project Knowledge

`wx-mini-video` 是面向 Windows 的通用微信小程序媒体嗅探与下载工具。

`wx-mini-video` is a Windows-first, general-purpose media sniffer and downloader for WeChat Mini Programs.

## 架构

`wx-mini-video` 是单进程 Go TUI 工具。入口 `main.go` 加载配置后进入 Bubble Tea TUI；用户手动输入目标 AppID，或按 `F2` 启动代理发现小程序。选择目标后才开始媒体嗅探；`internal/app` 维护代理、目标、候选资源和下载器。

核心数据流：

1. PC 微信请求经过本地代理 `127.0.0.1:2023`。
2. `internal/interceptor/miniprogram.go` 从微信小程序 `Referer` 解析请求来源 AppID，先完整匹配目标，再按响应 URL、Content-Type 和 JSON 响应体提取媒体候选；来源不明或不匹配时不读取媒体正文、不回补或缓存媒体。
3. `internal/miniprogram.Store` 同步维护目标、目标代次和按 AppID 去重的发现列表；候选按 AppID 和 URL 去重。提交候选与写入播放列表缓存时再次检查目标代次，防止切换前在途响应恢复旧列表。缓存写入检查与候选准入共用锁，网络读取不持有该锁。
4. TUI 默认显示“视频”分类，可切换全部、图片、视频、m3u8；候选详情可用 `i` 查看。
5. JSON 响应中媒体 URL 所在对象的 `title`、`videoTitle`、`mediaTitle`、`caption`、`subject` 或 `name` 字段会作为候选标题。
6. 下载器保存图片/直链视频；直链先写入 `.part`，可在失败后复用；m3u8 先并发缓存分片，再调用 ffmpeg 合并为 mp4。

## 配置

默认配置模板在 `internal/config/config.template.yaml`。常用项：

- `download.dir`: 下载目录，空值表示便携应用目录下的 `downloads/`；媒体、缓存、日志、证书和受管 ffmpeg 都在此目录。
- `download.ffmpeg`: 显式 ffmpeg.exe 完整路径；留空时使用下载目录的受管 ffmpeg。旧配置中的 `ffmpeg` 会视为留空，绝不从 PATH 查找。
- `download.mode`: `auto`、`prefetch`、`remote-ffmpeg`。
- `download.segmentConcurrency`: m3u8 分片并发数，默认 `6`。
- `proxy.system`: 是否设置 Windows 系统代理，默认 `true`。
- `target.appID`: 目标 AppID，启动 TUI 可预填、输入或通过小程序发现选择。空值允许进入发现，不表示接收全部媒体。
- `target.name`: 可选小程序名称，仅用于显示；发现后可以补填。名称映射仅保留在当前运行内。

运行中的代理使用同一候选存储中的目标状态；更新目标会清空候选及进度，而不删除本地文件。`s` 返回发现选择界面，下载进行中拒绝切换并显示说明。发现期间不产生媒体候选或媒体缓存；只发现一个 AppID 也不自动选择。

## 分发

基础包不内置 ffmpeg。首次下载 m3u8 时，若未配置显式路径，程序会下载固定版本并校验 SHA-256 后使用下载目录的受管 ffmpeg；网络或 HTTP 5xx 失败最多重试三次。

构建命令：

```powershell
cmd /c build\build.bat windows
```

生成：

```text
dist\wx-mini-video-windows-amd64\
dist\wx-mini-video-windows-amd64.zip
```

zip 用于 GitHub Releases，不提交到源码仓库。
构建脚本随后校验 zip 内容必须精确为 `README.md`、`wx-mini-video.exe` 和 `wx-mini-video.yaml`。

下载成功记录追加到 `downloads/history.jsonl`，字段包括完成路径、来源域名、文件大小和 UTC 时间。

## 排障

- 小程序网络错误：通常是系统代理残留。重新以管理员身份运行 exe，进入后按 `q` 正常退出。
- 证书失败：以管理员身份运行，或手动安装下载目录导出的 `WxMiniVideoRoot.cer` 到“受信任的根证书颁发机构”。
- 资源太多：先按 `c` 清空，再打开目标图片或播放目标视频。
- m3u8 慢：长视频分片多，优先看 TUI 的分片数、失败数和 ffmpeg 合并进度。失败数高时把 `segmentConcurrency` 调低到 `4`。
- m3u8 合并失败：查看 `下载目录\\.ffmpeg\\<候选ID>.log`；该日志会移除已捕获请求头值。

## 当前限制

- 只接受 HTTPS `servicewechat.com/<AppID>/<版本>/page-frame.html` 格式的 `Referer` 并完整匹配目标 AppID；不推断共享 CDN 或跨请求 URL 关联。缺失来源的目标媒体可能漏掉，真实 PC 微信的来源覆盖仍需验收。
- 发现列表只能列出代理实际观察到的小程序，名称缺失时显示“名称未识别”并允许补填，不依赖小程序管理凭证或外部名称查询。
- 只识别常见图片、直链视频和普通 VOD/回放 m3u8。
- Windows 是主要分发目标；其他系统未作为验收环境。

## 真实 PC 微信验收

1. 以管理员身份启动本地构建，按 `F2` 进入发现，分别打开两个小程序；确认 AppID 去重，尚未选择时没有媒体候选或播放列表缓存。
2. 选择目标、补填名称并确认，再重新打开图片、播放直链视频和 m3u8；核对详情中的 `Referer` 与目标 AppID 是否匹配。若未观察到来源信息，记录该媒体路径的覆盖限制，不通过放宽过滤解决。
3. 同时操作另一个小程序；其媒体不能出现在当前目标列表。来自匹配 JSON 接口的图片、直链视频仍可出现，JSON m3u8 继续跳过。
4. 按 `s` 切换目标并重新加载媒体；旧候选和进度清空，本地文件保留。下载进行中按 `s` 应提示无法切换，并继续下载。
5. 退出发现或媒体界面，确认系统代理恢复。验收记录不得包含 Cookie、鉴权头值或带签名的完整媒体 URL。
