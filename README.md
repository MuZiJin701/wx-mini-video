# 微信小程序媒体下载器

在 Windows（稳定版）与 macOS（未签名测试版）上保存 PC 微信小程序加载的图片、直链视频、直链音频和 m3u8 媒体。

[下载最新版](https://github.com/MuZiJin701/wx-mini-video/releases/latest) · [English](README.en.md)

> 本文档对应当前源码（v0.2.0，界面与下载目录为改版后行为）。v0.2.0 发布前，Release 中仍为 v0.1.9 包；升级时替换 exe，保留 `wx-mini-video.yaml`、`ffmpeg/` 和 `downloads/`。

Windows 与 macOS 包在同一个 [v0.1.9 Release](https://github.com/MuZiJin701/wx-mini-video/releases/tag/v0.1.9) 中并列提供：

| 平台 | 下载 | 状态 |
| --- | --- | --- |
| Windows amd64 | [Windows ZIP](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.9/wx-mini-video-windows-amd64.zip) | 稳定版 |
| macOS 13+ · Apple Silicon / Intel | [Mac 通用 ZIP](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.9/wx-mini-video-macos-universal-unsigned.zip) | 未签名测试版 |

> 随包 ffmpeg 由固定官方源码自建；对应源码归档与二进制在同一 Release 提供，文件名 `corresponding-source-windows-amd64.tar.gz`、`corresponding-source-darwin-amd64.tar.gz`、`corresponding-source-darwin-arm64.tar.gz`，说明见 [第三方声明](THIRD-PARTY-NOTICES.md#对应源码)。构建与验证方式见[源码构建说明](build/ffmpeg/README.md)。

Mac 包完整解压后双击 `.app`，媒体保存在解压目录的 `downloads/`。签名、公证和真实 Mac 微信验收尚未完成；首次打开可能需要系统允许或移动应用的引导。详见 [macOS 测试版说明](https://github.com/MuZiJin701/wx-mini-video/blob/main/docs/macos-usage.md)。下面的开始步骤适用于 Windows。

## 开始使用

1. 下载 `wx-mini-video-windows-amd64.zip`，**完整解压**后双击 `wx-mini-video.exe`。包内已附带 `ffmpeg\ffmpeg.exe`，无需另行下载或配置。
2. 程序以当前用户运行并优先使用已安装的 Windows Terminal。首次启动代理时会生成并安装本机当前用户独立的根证书；只有安装根证书或清理旧共享证书时才请求一次管理员授权（UAC），其余启动与代理设置不需要提权。若检测到旧共享证书，按提示选择是否清理。
3. **选择小程序**：在目标选择卡输入已知 AppID，按 `Enter` 开始嗅探；不知道 AppID 就按 `Ctrl+D`，在 PC 微信打开目标小程序，用 `↑/↓` 选择 AppID，按 `Enter` 直接确认（v0.2.0 起不再需要填写小程序名称）。
4. **重新加载媒体**：确认代理运行后，重新打开目标图片或播放视频、音频。用 `↑/↓` 选中候选，按 `d` 或 `Enter` 下载，按 `o` 打开当前小程序的下载目录。列表默认显示视频，按 `1` 查看全部，按 `5` 查看音频。来源已确认的候选显示 `✓`，来源未确认的显示 `?`，两者都按类型显示在同一分类。
5. 使用完按 `q` 退出，恢复启动前的系统代理；下载中退出会先弹确认。恢复失败会停留在错误界面，按 `r` 重试，或按 `f` 明确强制退出。目标选择卡用 `Esc` 或 `Ctrl+C` 退出。

证书操作失败时会弹窗显示具体原因；关闭弹窗后，主界面显示操作失败。取消 UAC 会单独提示，子进程已经失败退出时不会继续等待两分钟。

## 界面与外观

v0.2.0 起 TUI 为单主界面：顶部是品牌区（终端支持图片时显示真图 logo，否则显示块字符图标；右侧是外观切换），向下依次是渐变色带、主区和底栏。未选目标时主区是目标选择卡，确认目标后变为目标条 + 分类页签 + 候选表格 + 状态行。目标覆盖层、详情抽屉（`i`）、日志浮层（`l`）、退出确认与证书确认都是覆盖层；页签、表格行、按钮和外观控件可以鼠标点击，键盘操作全部保留。

右上角「外观」支持**跟随系统 / 浅色 / 深色**：用 `←/→` 或鼠标点击切换，立即生效，选择保存在 `downloads/ui.json`，下次启动沿用。Windows 读取系统应用明暗设置，macOS 读取系统外观；读取失败时按深色显示。

![媒体列表：选择候选并下载](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/使用界面.png)

<details>
<summary>查看操作截图</summary>

手动输入目标 AppID，或按 `Ctrl+D` 打开目标覆盖层：

![目标选择卡](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/首页.png)

代理运行后，在 PC 微信打开小程序；覆盖层会列出发现的 AppID 与最近出现时间：

![等待小程序请求](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/自动探测.png)

选中 AppID 即确认目标（不再补填名称）：

![选择已发现的小程序](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/已探测到.png)

下载完成后行尾显示「已下载」，按 `o` 查看文件：

![下载完成](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/下载完成.png)

</details>

## 下载的文件

所有媒体按「小程序 → 资源类型」分类保存：

```text
downloads/
├── <目标 AppID>/
│   ├── 图片/
│   ├── 视频/
│   ├── 音频/
│   ├── m3u8/          # 合并后的 mp4、播放列表缓存与分片临时目录
│   └── 其他/          # 未识别二进制
├── wx-mini-video.log
├── wx-mini-video-debug.log
├── history.jsonl      # 每个成功下载的目标 AppID、类型、来源与大小
├── ui.json            # 外观偏好
├── WxMiniVideoRoot.cer
└── ffmpeg.exe（macOS 为 ffmpeg）与 .ffmpeg/ 失败日志
```

- 文件名默认 `类型_日期-时间_候选ID前8位.扩展名`，例如 `视频_20261025-130512_9f8e7d6c.mp4`；m3u8 合并产物固定为 `.mp4`。同名冲突追加 `_2`、`_3`。
- 来源未确认（`?`）的候选按下载时的当前目标 AppID 归档；详情抽屉与 `history.jsonl` 中保留请求来源 AppID 供核对。
- 旧版平铺在 `downloads/` 根目录的文件不会迁移或删除，新下载只写入分类目录；重复下载生成新文件，不覆盖旧文件。
- 运行日志、`history.jsonl`、`ui.json`、根证书、受管 ffmpeg 与 `.ffmpeg/` 失败日志留在下载根目录；移动或升级时与 `downloads/` 一起保留。

## 常用按键

| 按键 | 操作 |
| --- | --- |
| `↑/↓`、`k/j` | 选择候选 / 列表行 |
| `d`、`Enter` | 下载选中项 |
| `Tab`、`Shift+Tab` | 切换分类 |
| `1/2/3/4/5` | 全部 / 图片 / 视频 / m3u8 / 音频（含来源未确认媒体） |
| `←/→` | 切换外观：跟随系统 / 浅色 / 深色 |
| `s`、`Ctrl+D` | 打开目标选择覆盖层（`Esc` 取消，当前目标与候选保留） |
| `c` | 清空列表并丢弃清空前的在途结果，保留本地文件 |
| `i` | 详情抽屉开关 |
| `l` | 日志浮层开关 |
| `o`、`Ctrl+O` | 打开当前小程序目录（未选目标或目录不存在时打开下载根目录） |
| `r`、`Ctrl+R` | 代理失败时重试启动；否则刷新候选 |
| `q`、`Ctrl+Q` | 退出发现或媒体界面，恢复代理（下载中需二次确认） |
| 鼠标 | 点击页签、候选行（双击下载）、按钮与外观控件；滚轮滚动列表或日志 |

## 使用前了解

- 一次只嗅探一个目标。发现阶段不产生目标候选；此阶段看到的来源未确认媒体会先被收集，确认目标后按类型显示（打开小程序时自动加载的第一个页面因此不会丢）。确认目标后需要重新加载内容，才能补全目标候选。切换目标会清空目标候选和进度，保留本地文件与来源未确认媒体。
- **目标候选**只接收请求来源 AppID 与目标完整匹配的媒体（标记 `✓`）。来源缺失或格式不符的媒体不会进入目标候选，但会作为**来源未确认**媒体（标记 `?`）按图片/视频/m3u8/音频并入对应分类：H5/web-view 页面、原生播放器或 CDN 直接请求的媒体常常没有小程序的 page-frame Referer。工具还会扫描小程序 H5 页面/脚本里的内联媒体地址，**打开详情页就可能出现候选，不必等视频真正播放**（这类候选标记为「H5 页面/脚本扫描（未播放验证）」）。来源未确认媒体可由你判断后下载；手动输入 AppID 不会把它们变成目标候选。可在配置 `capture.showUnattributed: false` 关闭来源未确认媒体（关闭后既不收集也不显示）。代理运行期间会看到本机微信其他内容（例如公众号文章图片）的媒体；来源未确认媒体只按响应类型收录，来源并不可靠。
- 图片、直链视频和直链音频直接保存原文件，不转码。Windows 与 macOS 都只提供一个完整包（已内置 ffmpeg，不再提供精简包），m3u8 无需联网准备；若随包 ffmpeg 被隔离、删除或损坏，程序会自动下载并校验受管 ffmpeg 兜底。也可在配置中填写 `download.ffmpeg` 指定已有可执行文件，优先级最高。随包 ffmpeg 是独立的 GPLv3 程序，许可与源码获取方式见 `THIRD-PARTY-NOTICES.md`。macOS 完整解压后请保留整个文件夹，`ffmpeg/` 位于 `.app` 外。
- 根证书与私钥在本机当前用户的受限目录中生成并复用，不随程序包分发；换电脑或 Windows 用户后需重新建立信任。首次安装根证书或清理旧共享证书时会请求一次管理员授权，证书写入 Windows 本地计算机根信任库，退出不会卸载。旧版共享根证书需单独清理，详见[证书与迁移](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#证书风险与清理)。
- 直链部分下载按资源隔离；只有具备服务器校验信息时才续传，否则重新下载。旧版未绑定资源的 `.part` 文件保留，但不再用于续传。JSON 跨域媒体不携带接口鉴权信息；若因此无法下载，请重新播放以捕获媒体自己的请求。
- 品牌真图 logo 依赖终端图形能力（Windows Terminal 1.22+ 的 Sixel，或 kitty 等终端）。终端不支持、设置了 `NO_COLOR`、经 tmux/screen/SSH 转发或窗口过矮时，会自动改用块字符图标，功能不受影响；运行中修改字号或缩放后建议重启程序以重新测量。

## 遇到问题

| 问题 | 处理方法 |
| --- | --- |
| 没有发现小程序 | 确认代理运行，重新打开小程序；按 `r` 刷新或重试，或 `Ctrl+D` 手动输入 AppID。 |
| 列表没有目标媒体 | 查看视频分类中标记 `?` 的来源未确认媒体（H5/web-view 视频通常在这里）；确认 AppID 后重新加载媒体。来源缺失的请求不会进入目标候选。 |
| 找不到下载的文件 | 按 `o` 打开当前小程序目录；文件在 `downloads/<AppID>/图片|视频|音频|m3u8|其他/` 下。旧版平铺文件仍在 `downloads/` 根目录，不会自动移动。 |
| 外观切换后没记住 | 检查下载目录的写入权限；偏好保存在 `downloads/ui.json`，读取失败时会回退为跟随系统。 |
| 没有显示真图 logo | 终端不支持 Sixel/kitty，或经 tmux/screen/SSH、设置了 `NO_COLOR`、窗口过矮；此时显示块字符图标属正常降级。 |
| 异常退出后网络不通 | 重开程序，按 `Ctrl+D`，等到显示“代理运行中”后按 `q`。仅停在目标选择卡再退出不会恢复代理。 |
| 启动时提示“读取原系统代理失败” | 本版已改为直接读取注册表并把缺失值视为未配置；若仍出现，确认没有运行旧版。只有安装/清理证书时会请求管理员授权。 |
| 证书安装失败 | 确认同意 UAC；仍失败时见[证书排障](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#排障)。 |
| m3u8 下载或合并失败 | 请确认 `ffmpeg/` 未被隔离或删除；程序会在随包 ffmpeg 缺失或损坏时自动下载受管 ffmpeg，也可在配置中指定 `download.ffmpeg`。错误会给出 ffmpeg 日志路径，更多处理见[排障](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#排障)。 |
| 需要排查嗅探问题 | 用 `wx-mini-video.exe --debug` 启动，诊断日志写入下载目录的 `wx-mini-video-debug.log`；分享前检查其中是否包含敏感信息。 |

[维护指南：配置、构建与验收](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md) · [版本记录](https://github.com/MuZiJin701/wx-mini-video/blob/main/CHANGELOG.md)

仅保存你有权获取的内容，并遵守相关法律和平台规则。
