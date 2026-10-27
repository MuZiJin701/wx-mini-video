# 微信小程序媒体下载器

把**电脑版微信小程序**里加载的图片、视频和音频保存到电脑，也支持部分直播录制。

[下载最新版](https://github.com/MuZiJin701/wx-mini-video/releases/latest) · [English](https://github.com/MuZiJin701/wx-mini-video/blob/main/README.en.md)

## 下载

| 你的电脑 | 下载哪个文件 |
| --- | --- |
| Windows 64 位 | [Windows 完整包](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.9/wx-mini-video-windows-amd64.zip) |
| macOS 13 及以上，Intel 或 Apple Silicon | [Mac 通用包（测试版）](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.9/wx-mini-video-macos-universal-unsigned.zip) |

**下载后先完整解压，不要直接在压缩包里运行。** 合并媒体所需的工具已随包附带，无需另外安装。

Mac 版仍在测试，首次打开可能受到系统拦截，请参照 [Mac 使用说明](https://github.com/MuZiJin701/wx-mini-video/blob/main/docs/macos-usage.md)。

## 4 步开始使用

### 1. 打开程序

Windows 双击解压后的 `wx-mini-video.exe`；Mac 双击 `wx-mini-video.app`，保留解压出来的整个文件夹。

程序会自动开始发现小程序。首次使用时，按程序提示完成证书安装授权；这是识别微信加载内容所需的设置。等界面显示“代理运行中”再继续。

### 2. 选择要下载的小程序

在**电脑版微信**中打开目标小程序，然后点击程序“已发现的小程序”列表中对应的一项。

- 不知道 AppID 也没关系，先等程序发现即可。AppID 就是小程序的编号。
- 列表有多项时，先关闭其他小程序，再重新打开目标小程序，观察新出现或最近更新的编号。
- 已经知道 AppID，可以直接输入，再按 `Enter` 确认。

<details>
<summary>查看选择小程序的运行截图</summary>

下图为 Windows 版实际运行截图，列表取决于你打开的小程序。

![选择小程序：点击已发现列表中的对应项](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/首页.png)

</details>

### 3. 重新打开内容，点击下载

确认小程序后，**回到微信重新打开图片，或播放视频、音频**，让内容出现在资源列表中。

点击想保存的一行，再点“下载”。可以用“图片、视频、音频”等标签筛选；不确定时先看“全部”。

`✓` 表示来源与所选小程序匹配；`?` 表示来源还未确认，下载前请核对内容。

![Windows 版下载完成截图：所选资源显示“已下载”，下方显示保存位置](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/下载完成.png)

### 4. 打开文件夹，正常退出

点“打开目录”查看文件。默认保存在程序旁的 `downloads` 文件夹，按**小程序编号 → 图片、视频等类型**分类存放。

用完点右下角“退出”，或按 `Ctrl+Q`，让程序恢复原来的系统代理设置。下载或录制中退出时，请按提示确认并等待保存完成。

## 录制直播

在“直播”标签中选择资源，点“下载”开始录制；想结束时点“停止”或按 `x`，已录内容会保存为 `.flv` 文件。

“停止”只用于结束本程序正在进行的下载或录制。**列表里出现直播资源，并不代表已经开始录制**；先点“下载”，等界面显示“录制直播中”后，停止按钮才会启用。它不会停止微信中的播放，也不会关闭资源发现；用完请点“退出”。

图片、视频、音频和分段媒体（m3u8）下载中也可以停止。普通下载会保留未完成文件；m3u8 合并被中止后，已产生的文件不一定能播放，不能当作下载完成。小文件可能很快就下载完，此后停止按钮会重新禁用。

直播不会像普通视频一样自动下载完。**目前仅支持 HTTP-FLV 直播录制；m3u8/HLS 直播不能持续录制。** m3u8 资源在“分段媒体”中，按当前播放列表下载并合并。

## 常用操作

主要操作都可以点击按钮，也可以使用快捷键。

| 想做什么 | 按键 |
| --- | --- |
| 选择资源、下载 | `↑/↓`，然后 `Enter` 或 `d` |
| 停止下载或录制 | `x` |
| 打开下载目录 | `Ctrl+O` |
| 刷新或重试启动 | `Ctrl+R` |
| 更换小程序 | `s`（下载或录制中不可用） |
| 查看详情 / 日志 | `i` / `l` |
| 退出 | `Ctrl+Q` |

鼠标滚轮只滚动资源列表，不改变选中项。“清空”只清理列表，**不会删除已下载文件**。输入 AppID 时，字母按文本输入，退出请用 `Ctrl+Q`。

## 遇到问题

| 问题 | 先这样试 |
| --- | --- |
| 没发现小程序，或列表没有内容 | 确认显示“代理运行中”，重新打开微信里的小程序；选定后再打开图片或播放音视频。仍没有内容，按 `Ctrl+R` 重试。 |
| 直播一直没有下载完 | 点“停止”保存录制；直播需要手动结束。没有任务时，停止按钮呈灰色。 |
| 下载或保存失败 | 点“日志”查看原因。若提示文件已保留，按提示位置找到文件，暂时不要删除；更多方法见下方排障指南。 |
| 意外关闭后网络不正常 | 重新运行程序，等“代理运行中”出现后，用 `Ctrl+Q` 正常退出；恢复失败时点“重试恢复并退出”。 |

升级前先退出旧程序，并保留自己的 `wx-mini-video.yaml` 配置和 `downloads` 文件夹。

[详细排障与配置](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#排障) · [更新记录](https://github.com/MuZiJin701/wx-mini-video/blob/main/CHANGELOG.md) · [第三方组件与许可](THIRD-PARTY-NOTICES.md)

仅保存你有权获取的内容，并遵守相关法律和平台规则。
