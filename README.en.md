# WeChat Mini Program Media Downloader

Save images, videos, and audio loaded by **Mini Programs in desktop WeChat**. Recording is also available for supported live streams.

[Download latest](https://github.com/MuZiJin701/wx-mini-video/releases/latest) · [简体中文](https://github.com/MuZiJin701/wx-mini-video/blob/main/README.md)

## Download

| Your computer | File to download |
| --- | --- |
| 64-bit Windows | [Complete Windows package](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.9/wx-mini-video-windows-amd64.zip) |
| macOS 13 or later, Intel or Apple Silicon | [Universal Mac package (test build)](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.9/wx-mini-video-macos-universal-unsigned.zip) |

**Extract the whole ZIP before running the app.** The tools needed to merge media are included; no separate installation is needed.

The Mac version is still a test build and may be blocked on first launch. See the [Mac instructions (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/docs/macos-usage.md).

## Get started in 4 steps

The interface is currently in Chinese. The button names below match the screen.

### 1. Open the app

On Windows, double-click `wx-mini-video.exe`. On Mac, open `wx-mini-video.app` and keep the entire extracted folder together.

The app starts discovering Mini Programs automatically. On first use, follow its certificate installation prompts; this setup lets it recognize content loaded by WeChat. Wait until “代理运行中” (proxy running) appears.

### 2. Choose a Mini Program

Open the Mini Program in **desktop WeChat**, then click its entry under “已发现的小程序” (discovered Mini Programs).

- You do not need to know its AppID in advance. An AppID is the Mini Program's identifier.
- If several entries appear, close other Mini Programs, reopen the one you want, and look for the new or recently updated identifier.
- If you already know the AppID, type it and press `Enter`.

<details>
<summary>See a screenshot of choosing a Mini Program</summary>

This screenshot shows the Windows app running. Your list depends on the Mini Programs you open.

![Choose a Mini Program by clicking its discovered entry](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/首页.png)

</details>

### 3. Reload the content and download

After confirming the Mini Program, **return to WeChat and reopen an image or play the video or audio** so it appears in the resource list.

Click a resource, then “下载” (Download). Use the image, video, or audio tabs to narrow the list; start with “全部” (All) if unsure.

`✓` means the source matches the selected Mini Program. `?` means the source is unconfirmed, so check the content before downloading.

![Windows app screenshot: the selected resource is marked as downloaded, with its saved location below](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/下载完成.png)

### 4. Find your files and quit

Click “打开目录” (Open folder). By default, files are saved in `downloads` beside the app, grouped by **Mini Program AppID → media type**.

When finished, click “退出” (Quit) at the bottom right or press `Ctrl+Q` to restore your previous system proxy settings. If a download or recording is active, confirm the prompt and wait for it to finish saving.

## Record a live stream

Choose a resource under “直播” (Live) and click “下载” (Download) to start recording. Click “停止” (Stop) or press `x` to finish and save a `.flv` file.

**A live stream appearing in the list does not mean it is being recorded.** Once started, its row shows “录制中” (recording), with the recorded size below.

Live streams do not finish downloading like ordinary videos. **Continuous recording supports HTTP-FLV only; HLS/m3u8 live recording is not supported.** m3u8 resources appear under “分段媒体” (Segmented media) and are downloaded and merged from the current playlist.

<details>
<summary>See a screenshot of live recording</summary>

![Windows app screenshot: the resource is marked as recording, with the recorded size and Stop hint below](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/直播录制.png)

</details>

## Common actions

You can use the buttons or these shortcuts.

| What you want to do | Keys |
| --- | --- |
| Select and download a resource | `↑/↓`, then `Enter` or `d` |
| Stop a download or recording | `x` |
| Open the download folder | `Ctrl+O` |
| Refresh or retry startup | `Ctrl+R` |
| Change Mini Program | `s` (unavailable during a download or recording) |
| View details / logs | `i` / `l` |
| Quit | `Ctrl+Q` |

**“停止 · x” (Stop) is enabled only while a download or recording is running. Being disabled before starting or after completion is normal.** It applies to images, videos, audio, segmented media, and live recordings. Small files may finish before you can click it. Stop does not stop playback in WeChat or resource discovery.

Stopping an ordinary download keeps unfinished files. Files left by interrupted m3u8 merging may not play and should not be treated as complete. Stopping a live recording saves the recorded content.

The mouse wheel scrolls the resource list without changing your selection. “清空” (Clear) clears the list and **keeps downloaded files**. While typing an AppID, letters are text; use `Ctrl+Q` to quit.

## If something goes wrong

| Problem | Try this first |
| --- | --- |
| No Mini Program or no resources appear | Check for “代理运行中” (proxy running), reopen the Mini Program in WeChat, and reload the content after confirming it. Press `Ctrl+R` to retry. |
| A live download never finishes | Click “停止” (Stop) to save it. Recording needs to be stopped manually. The Stop button is grey when no task is running. |
| Downloading or saving fails | Click “日志” (Logs) for the reason. If the message says a file was kept, find it at the location shown and keep it for now. See the troubleshooting guide below. |
| The network stops working after an unexpected exit | Restart the app, wait for “代理运行中”, then quit with `Ctrl+Q`. If restoration fails, click “重试恢复并退出” (Retry restoration and quit). |

Before upgrading, quit the old app and keep your `wx-mini-video.yaml` settings and `downloads` folder.

[Troubleshooting and configuration (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#排障) · [Change log (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/CHANGELOG.md) · [Third-party components and licenses](THIRD-PARTY-NOTICES.md)

Only save content you have the right to access, and follow applicable laws and platform rules.
