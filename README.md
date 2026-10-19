# 微信小程序媒体下载器

在 Windows 上保存 PC 微信小程序加载的图片、直链视频、直链音频和 m3u8 媒体。

[下载最新版](https://github.com/MuZiJin701/wx-mini-video/releases/latest) · [English](https://github.com/MuZiJin701/wx-mini-video/blob/main/README.en.md)

## 开始使用

1. 下载 `wx-mini-video-windows-amd64.zip`，**完整解压**后双击 `wx-mini-video.exe`。
2. 同意管理员权限（UAC）提示。程序优先使用已安装的 Windows Terminal；首次启动代理时会安装本地根证书。
3. **选择小程序**：已知 AppID 就直接输入，名称可选；不知道就按 `Ctrl+D`，在 PC 微信打开目标小程序，用 `↑/↓` 选择 AppID，按 `Enter` 补填名称，再按 `Enter` 确认。
4. **重新加载媒体**：确认代理运行后，重新打开目标图片或播放视频、音频。用 `↑/↓` 选中候选，按 `d` 或 `Enter` 下载，按 `o` 打开下载目录。列表默认显示视频，按 `1` 查看全部，按 `5` 查看音频。
5. 使用完按 `q` 退出，恢复启动前的系统代理。AppID 输入界面用 `Esc` 或 `Ctrl+C` 退出。

默认文件保存在程序旁的 `downloads/`。升级时替换 exe，保留 `wx-mini-video.yaml` 和 `downloads/`。

![媒体列表：选择候选并下载](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/使用界面.png)

<details>
<summary>查看操作截图</summary>

输入目标 AppID，或按 `Ctrl+D` 使用发现：

![AppID 输入界面](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/首页.png)

代理运行后，在 PC 微信打开小程序：

![等待小程序请求](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/自动探测.png)

选择目标，再确认名称：

![选择已发现的小程序](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/已探测到.png)

下载完成后按 `o` 查看文件：

![图片下载完成](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/下载完成.png)

</details>

## 常用按键

| 按键 | 操作 |
| --- | --- |
| `↑/↓`、`k/j` | 选择候选 |
| `d`、`Enter` | 下载选中项 |
| `Tab`、`Shift+Tab` | 切换分类 |
| `1/2/3/4/5` | 全部 / 图片 / 视频 / m3u8 / 音频 |
| `c` | 清空列表，便于重新捕获 |
| `s` | 返回目标选择；下载中不可切换 |
| `i`、`l` | 显示或隐藏详情、日志 |
| `o` | 打开下载目录 |
| `q` | 退出发现或媒体界面，恢复代理 |

## 使用前了解

- 一次只嗅探一个目标。发现阶段不收集媒体；确认目标后需要重新加载内容。切换目标会清空列表和进度，保留本地文件。
- 只接收请求来源 AppID 与目标完整匹配的媒体。来源缺失或格式不符时会漏掉，即使手动输入 AppID 也不能绕过此限制。小程序名称可补填，不参与过滤。
- 图片、直链视频和直链音频直接保存原文件，不转码。m3u8 需要 ffmpeg：基础包不内置，首次下载时自动下载并校验；已有 ffmpeg 可在配置中填写 `download.ffmpeg` 的完整路径。
- **证书风险：当前默认根证书使用公开的共享私钥，安装会修改 Windows 系统信任，退出不会卸载。** 详见[风险与证书清理](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#证书风险与清理)。

## 遇到问题

| 问题 | 处理方法 |
| --- | --- |
| 没有发现小程序 | 确认代理运行，重新打开小程序；按 `r` 刷新或重试，或 `Ctrl+D` 手动输入 AppID。 |
| 列表没有目标媒体 | 按 `1` 查看全部，确认 AppID 后重新加载媒体；来源缺失的请求仍会被过滤。 |
| 异常退出后网络不通 | 重开程序，按 `Ctrl+D`，等到显示“代理运行中”后按 `q`。仅停在输入界面再退出不会恢复代理。 |
| 证书安装失败 | 确认同意 UAC；仍失败时见[证书排障](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#排障)。 |
| m3u8 下载或合并失败 | 重新播放后重试；错误中给出 ffmpeg 日志路径，更多处理见[排障](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#排障)。 |

[维护指南：配置、构建与验收](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md) · [版本记录](https://github.com/MuZiJin701/wx-mini-video/blob/main/CHANGELOG.md)

仅保存你有权获取的内容，并遵守相关法律和平台规则。
