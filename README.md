# 微信小程序媒体下载器

在 Windows（稳定版）与 macOS（未签名测试版）上保存 PC 微信小程序加载的图片、直链视频、HTTP-FLV 直播流、直链音频和 m3u8 媒体。

[下载最新版](https://github.com/MuZiJin701/wx-mini-video/releases/latest) · [English](README.en.md)

> 本文档对应 v0.2.0 源码（含单主界面、按 AppID 分类下载与 HTTP-FLV 直播录制）。v0.2.0 发布前 Release 仍为 v0.1.9；升级时替换 exe，保留 `wx-mini-video.yaml`、`ffmpeg/` 和 `downloads/`。

Windows 与 macOS 包在同一 [v0.1.9 Release](https://github.com/MuZiJin701/wx-mini-video/releases/tag/v0.1.9) 并列提供：

| 平台 | 下载 | 状态 |
| --- | --- | --- |
| Windows amd64 | [Windows ZIP](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.9/wx-mini-video-windows-amd64.zip) | 稳定版 |
| macOS 13+ · Apple Silicon / Intel | [Mac 通用 ZIP](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.9/wx-mini-video-macos-universal-unsigned.zip) | 未签名测试版 |

随包 ffmpeg 由固定官方源码自建（FFmpeg 9.0.2 + OpenSSL 3.5.9）；对应源码归档与二进制在同一 Release 提供，说明见 [第三方声明](THIRD-PARTY-NOTICES.md#对应源码)。Mac 包完整解压后双击 `.app`；签名、公证和真实 Mac 微信验收尚未完成，详见 [macOS 测试版说明](docs/macos-usage.md)。

## 开始使用

1. 下载 `wx-mini-video-windows-amd64.zip`，**完整解压**后双击 `wx-mini-video.exe`；包内已附带 `ffmpeg\ffmpeg.exe`，无需另行配置。
2. 程序以当前用户运行并优先使用已安装的 Windows Terminal。首次启动代理会生成并安装当前用户独立的根证书；只有安装根证书或清理旧共享证书时请求一次管理员授权（UAC），其余启动与代理设置不需要提权。
3. **选择小程序**：在目标选择卡输入已知 AppID 后按 `Enter`；不知道就按 `Ctrl+D`，在 PC 微信打开目标小程序，用 `↑/↓` 选择 AppID 后按 `Enter` 直接确认（v0.2.0 起不需要填写名称）。
4. **重新加载媒体**：确认代理运行后，在 PC 微信重新打开图片或播放视频、音频。用 `↑/↓` 选中候选，`d`/`Enter` 下载，`o` 打开当前小程序目录。列表默认显示视频，`1` 查看全部、`5` 查看音频；`✓` 表示来源已确认，`?` 表示来源未确认。HTTP-FLV 直播流标记为「直播流」，下载是持续录制，按 `x` 停止并保存；断流时会自动保存已录内容并提示原因。
5. 使用完按 `q` 退出并恢复原系统代理；下载中退出需先确认。恢复失败时按 `r` 重试或 `f` 强制退出；目标选择卡用 `Esc`/`Ctrl+C` 退出。

## 界面与外观

v0.2.0 起为单主界面：品牌区（真图 logo 或块字符图标、右侧外观切换）→ 渐变色带 → 主区（目标选择卡，或目标条 + 分类页签 + 候选表格 + 状态行）→ 底栏。目标覆盖层、详情抽屉（`i`）、日志浮层（`l`）、退出确认和证书确认都是覆盖层；页签、候选行、按钮和外观控件支持鼠标点击，键盘操作全部保留。

「外观」支持**跟随系统 / 浅色 / 深色**，用 `←/→` 或鼠标点击切换，偏好保存在 `downloads/ui.json` 并在下次启动沿用；系统明暗读取失败时回退深色。品牌真图依赖终端图形能力（Windows Terminal 1.22+ 的 Sixel 或 kitty）；不支持、设置了 `NO_COLOR`、经 tmux/screen/SSH 或窗口过矮时自动降级为块字符图标。

![媒体列表：选择候选并下载（示例选中直播流）](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/使用界面.png)

<details>
<summary>查看操作截图</summary>

![目标选择卡](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/首页.png)

![发现小程序](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/已探测到.png)

![下载完成](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/下载完成.png)

![直播录制](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/直播录制.png)

</details>

## 下载的文件

```text
downloads/
├── <目标 AppID>/
│   ├── 图片/   视频/   音频/
│   ├── m3u8/    # 合并后的 mp4、播放列表缓存与分片临时目录
│   └── 其他/    # 未识别二进制
├── wx-mini-video.log、wx-mini-video-debug.log
├── history.jsonl   # 每个成功下载的目标 AppID、类型、来源与大小
├── ui.json         # 外观偏好
├── WxMiniVideoRoot.cer
└── ffmpeg.exe（macOS 为 ffmpeg）与 .ffmpeg/ 失败日志
```

- 文件名默认 `类型_日期-时间_候选ID前8位.扩展名`，如 `视频_20261025-130512_9f8e7d6c.mp4`；m3u8 合并产物为 `.mp4`，HTTP-FLV 录制保存为 `.flv`；同名冲突追加 `_2`、`_3`。
- 来源未确认（`?`）候选按下载时的目标 AppID 归档，`history.jsonl` 保留请求来源 AppID；旧版平铺在根目录的文件不迁移、不删除，重复下载生成新文件。
- 日志、`history.jsonl`、`ui.json`、根证书、受管 ffmpeg 与 `.ffmpeg/` 留在下载根目录；移动或升级时与 `downloads/` 一起保留。

## 常用按键

| 按键 | 操作 |
| --- | --- |
| `↑/↓`、`k/j` | 选择候选 / 列表行 |
| `d`、`Enter` | 下载选中项；直播流开始持续录制 |
| `x` | 停止当前下载；直播录制保存已录内容为 `.flv` |
| `Tab`、`Shift+Tab`、`1/2/3/4/5` | 切换分类（全部 / 图片 / 视频 / m3u8 / 音频） |
| `←/→` | 切换外观：跟随系统 / 浅色 / 深色 |
| `s`、`Ctrl+D` | 打开目标选择覆盖层（`Esc` 取消，保留当前目标与候选） |
| `c` | 清空列表（保留本地文件） |
| `i`、`l` | 详情抽屉 / 日志浮层开关 |
| `o`、`Ctrl+O` | 打开当前小程序目录（未选目标或目录不存在时打开下载根目录） |
| `r`、`Ctrl+R` | 代理失败时重试启动；否则刷新候选 |
| `q`、`Ctrl+Q` | 退出并恢复代理（下载中需确认） |
| 鼠标 | 点击页签、候选行（双击下载）、按钮与外观控件；滚轮滚动列表或日志 |

## 使用前了解

- 一次只嗅探一个目标。发现阶段不产生目标候选；确认目标后需要重新加载内容才能补全目标候选。切换目标会清空目标候选和进度，保留本地文件与来源未确认媒体。
- **目标候选**只接收来源 AppID 与目标完整匹配的媒体（`✓`）；H5/web-view、原生播放器或 CDN 常无小程序 page-frame Referer，这类媒体以 `?` 标记按类型并入分类，可自行判断下载。工具还会扫描 H5 页面/脚本中的内联媒体地址，打开详情页就可能出现候选（标记「H5 页面/脚本扫描（未播放验证）」）。可用 `capture.showUnattributed: false` 关闭来源未确认媒体。
- 图片、直链视频和直链音频直接保存原文件，不转码。**HTTP-FLV 直播流**（`.flv` 地址或 `video/x-flv` 响应且无 `Content-Length`）没有总长度和结束点，不能续传；按 `x` 或确认退出停止并保存 `.flv`，异常断流（断网等）也会自动保存已录内容并写入历史；保存失败时显示真实错误并保留 `.part` 供恢复。录制可用 ffmpeg/VLC 播放或转封装，HLS 直播暂不在识别范围。
- Windows 与 macOS 只提供一个完整包，m3u8 无需联网准备；随包 ffmpeg 缺失或损坏时自动下载受管 ffmpeg 兜底，也可用 `download.ffmpeg` 指定已有可执行文件。随包 ffmpeg 是独立的 GPLv3 程序，许可与源码获取见 `THIRD-PARTY-NOTICES.md`。
- 根证书与私钥在本机当前用户受限目录生成并复用，不随包分发；换电脑或 Windows 用户后需重新建立信任，退出只恢复代理、不卸载证书。直链下载在服务器提供校验信息时按 Range 续传；失败后重试会复用同一候选遗留的 `.part`（直播录制不复用旧文件）。JSON 跨域媒体不携带接口鉴权信息。

## 遇到问题

| 问题 | 处理方法 |
| --- | --- |
| 没有发现小程序 | 确认代理运行后重新打开小程序；按 `r` 刷新/重试，或 `Ctrl+D` 手动输入 AppID。 |
| 列表没有目标媒体 | 查看视频分类中 `?` 标记的来源未确认媒体；确认目标后重新加载媒体。 |
| 直播流下载一直不结束 | 正常：HTTP-FLV 直播没有结束点，按 `x` 停止并保存 `.flv`；需要 mp4 时用 ffmpeg 转封装。 |
| 直播断流后内容还在吗 | 在：断流时已录内容自动保存为 `.flv` 并写入历史，状态行与日志给出中断原因；保存失败时错误信息与 `.part` 都会保留，可重试或手动恢复。 |
| 失败后重试会重新下载吗 | 直链会复用同一候选的 `.part` 并续传（需服务器提供 ETag 等校验）；直播录制每次重试都会新建文件，避免覆盖上一次录制。 |
| 找不到下载的文件 | 按 `o` 打开当前小程序目录；旧版平铺文件仍在 `downloads/` 根目录，不会自动移动。 |
| 外观或 logo 不生效 | 外观检查 `downloads/` 写入权限；真图 logo 需要支持 Sixel/kitty 的终端，否则显示块字符图标属正常降级。 |
| 异常退出后网络不通 | 重开程序，按 `Ctrl+D`，等显示“代理运行中”后按 `q`。 |
| 证书安装失败 | 确认同意 UAC；仍失败见[证书排障](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#排障)。 |
| m3u8 下载或合并失败 | 确认 `ffmpeg/` 未被隔离或删除；错误会给出 `.ffmpeg/` 日志路径，详见[排障](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#排障)。 |
| 需要诊断嗅探 | 用 `--debug` 启动，日志写入 `wx-mini-video-debug.log`；分享前检查敏感信息。 |

[维护指南：配置、构建与验收](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md) · [版本记录](CHANGELOG.md)

仅保存你有权获取的内容，并遵守相关法律和平台规则。
