# WeChat Mini Program Media Downloader

Save images, direct videos (including HTTP-FLV live stream recording), direct audio, and m3u8 media loaded by desktop WeChat Mini Programs on Windows (stable) and macOS (unsigned test build).

[Download latest](https://github.com/MuZiJin701/wx-mini-video/releases/latest) · [简体中文](https://github.com/MuZiJin701/wx-mini-video/blob/main/README.md)

> This document describes the current source (v0.2.0, with the redesigned UI and per-AppID download layout). Until v0.2.0 is released, the Release channel still serves v0.1.9; to upgrade, replace the exe and keep `wx-mini-video.yaml`, `ffmpeg/`, and `downloads/`.

Windows and macOS packages are available together in the [v0.1.9 Release](https://github.com/MuZiJin701/wx-mini-video/releases/tag/v0.1.9):

| Platform | Download | Status |
| --- | --- | --- |
| Windows amd64 | [Windows ZIP](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.9/wx-mini-video-windows-amd64.zip) | Stable |
| macOS 13+ · Apple Silicon / Intel | [Universal Mac ZIP](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.9/wx-mini-video-macos-universal-unsigned.zip) | Unsigned test build |

> Bundled ffmpeg in this release is built from pinned official source. Corresponding source archives are provided in the same Release as `corresponding-source-windows-amd64.tar.gz`, `corresponding-source-darwin-amd64.tar.gz`, and `corresponding-source-darwin-arm64.tar.gz`; see the [third-party notice (Chinese)](THIRD-PARTY-NOTICES.md#对应源码). Build and verification steps are described in the [source-build guide (Chinese)](build/ffmpeg/README.md).

Extract the whole Mac archive and open the `.app`; media is saved to `downloads/` beside it. Developer ID signing, notarization, and real Mac WeChat acceptance remain pending. First launch may require per-app system approval or the documented app move step. See the [macOS instructions (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/docs/macos-usage.md). The steps below apply to Windows.

## Get started

1. Download `wx-mini-video-windows-amd64.zip`, **extract the whole archive**, and double-click `wx-mini-video.exe`. The archive also contains `ffmpeg\ffmpeg.exe`; no separate download or configuration is needed.
2. The app runs as the current user and prefers an installed Windows Terminal. Starting the proxy for the first time generates and installs a root certificate unique to the current Windows user; administrator (UAC) authorization is requested only when installing the certificate or removing the old shared root, not for normal startup or proxy setup. If an old shared root is detected, choose whether to remove it when prompted.
3. **Choose a Mini Program**: enter a known AppID on the target card and press `Enter` to start sniffing. If you do not know the AppID, press `Ctrl+D`, open the Mini Program in desktop WeChat, select its AppID with `↑/↓`, and press `Enter` to confirm directly — v0.2.0 no longer asks for a Mini Program name.
4. **Reload the media**: once the proxy is running, reopen the image or replay the video or audio. Select a candidate with `↑/↓`, press `d` or `Enter` to download, and `o` to open the current Mini Program's folder. The list starts on Videos; press `1` for All or `5` for Audio. Confirmed candidates show `✓`; unconfirmed ones show `?` and appear in the same type categories. HTTP-FLV live streams are labelled `直播流` (live stream); a download keeps recording without a natural end, so press `x` to stop and save.
5. Press `q` when finished to restore the previous system proxy; quitting during a download asks for confirmation. If restoration fails, the error screen stays open: press `r` to retry or `f` to explicitly force exit. On the target card, use `Esc` or `Ctrl+C` to quit.

If a certificate operation fails, a dialog shows the specific reason. After closing it, the main interface reports the failure. Cancelling UAC is reported separately; a failed helper process no longer causes a two-minute wait.

## Interface and appearance

Since v0.2.0 the TUI has a single main surface: a brand area at the top (a bitmap logo when the terminal supports images, otherwise a block-character mark, plus the appearance switch), a gradient rule, the main area, and a footer. With no target selected the main area is the target card; after confirmation it becomes the target bar, category tabs, candidate table, and status line. The target overlay, detail drawer (`i`), log overlay (`l`), quit confirmation, and certificate confirmation are overlays. Tabs, table rows, buttons, and the appearance control accept mouse clicks, while every keyboard shortcut remains available.

The **appearance** control supports **follow system / light / dark**, switched with `←/→` or a click, effective immediately and remembered in `downloads/ui.json`. Windows reads the system app theme; macOS reads the system appearance; when unavailable the UI falls back to dark.

![Media list: select a candidate and download](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/使用界面.png)

<details>
<summary>View workflow screenshots</summary>

Enter a target AppID, or press `Ctrl+D` for the target overlay:

![Target card](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/首页.png)

Once the proxy is running, open the Mini Program; the overlay lists discovered AppIDs with their last-seen time:

![Waiting for Mini Program requests](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/自动探测.png)

Selecting an AppID confirms the target immediately (no name step):

![Select a discovered Mini Program](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/已探测到.png)

When the download finishes, the row shows "downloaded"; press `o` to view the file:

![Completed download](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/下载完成.png)

</details>

## Downloaded files

All media is grouped by Mini Program and resource type:

```text
downloads/
├── <target AppID>/
│   ├── 图片/          # images
│   ├── 视频/          # direct videos
│   ├── 音频/          # direct audio
│   ├── m3u8/          # merged mp4, playlist cache, and segment temp files
│   └── 其他/          # unknown binaries
├── wx-mini-video.log
├── wx-mini-video-debug.log
├── history.jsonl      # target AppID, kind, attribution, source, size per success
├── ui.json            # appearance preference
├── WxMiniVideoRoot.cer
└── ffmpeg.exe (ffmpeg on macOS) plus .ffmpeg/ failure logs
```

- Default file names are `type_YYYYMMDD-HHMMSS_shortID.ext`, for example `视频_20261025-130512_9f8e7d6c.mp4`; merged m3u8 output is always `.mp4`, while HTTP-FLV live recordings are saved as `.flv`. Name collisions get `_2`, `_3`.
- Unconfirmed (`?`) candidates are filed under the target AppID that was active when downloading; the detail drawer and `history.jsonl` keep the request source AppID for reference.
- Media that previous versions saved flat in `downloads/` is not migrated or deleted; new downloads only go to the categorized folders, and re-downloading creates a new file instead of overwriting.
- Logs, `history.jsonl`, `ui.json`, the exported root certificate, managed ffmpeg, and `.ffmpeg/` failure logs stay in the download root; keep them with `downloads/` when moving or upgrading.

## Common keys

| Key | Action |
| --- | --- |
| `↑/↓`, `k/j` | Select a candidate / row |
| `d`, `Enter` | Download the selection |
| `x` | Stop the current download; a live recording is saved as `.flv` |
| `Tab`, `Shift+Tab` | Switch categories |
| `1/2/3/4/5` | All / Images / Videos / m3u8 / Audio (including unconfirmed media) |
| `←/→` | Switch appearance: follow system / light / dark |
| `s`, `Ctrl+D` | Open the target overlay (`Esc` cancels and keeps the current target and candidates) |
| `c` | Clear the list and discard in-flight results from before the clear; keeps local files |
| `i` | Toggle the detail drawer |
| `l` | Toggle the log overlay |
| `o`, `Ctrl+O` | Open the current Mini Program folder (download root when no target or the folder is missing) |
| `r`, `Ctrl+R` | Retry proxy startup after a failure; otherwise refresh candidates |
| `q`, `Ctrl+Q` | Quit and restore the proxy (confirmation during a download) |
| Mouse | Click tabs, rows (double-click downloads), buttons, and the appearance control; wheel scrolls the list or logs |

## Before you start

- Only one target is sniffed at a time. Discovery does not create target candidates; unconfirmed media seen before confirmation is kept and appears under its type categories after confirmation (so the first page loaded when the Mini Program opens is not lost). Reload content after confirming a target to collect target candidates. Switching targets clears target candidates and progress but keeps local files and unconfirmed media.
- **Target candidates** require the request source AppID to exactly match the target (marked `✓`). Media with a missing or malformed source is not a target candidate; it is collected as **unconfirmed** media (`?`) under the matching image/video/m3u8/audio category, because H5/web-view pages, native players, or CDNs often omit the page-frame Referer. The tool also scans Mini Program H5 pages/scripts for inline media URLs, so **opening a detail page can produce candidates without actual playback** (marked "H5 page/script scan, unverified"). You may download unconfirmed media at your own judgement; entering an AppID manually never promotes them. Set `capture.showUnattributed: false` to disable collecting and showing them. While the proxy runs you may see media from other WeChat content on this machine (for example WeChat Official Account images); unconfirmed media is classified by response type only, and its source is not reliable.
- Images, direct videos, and direct audio are saved as-is without transcoding. **HTTP-FLV live streams** (common in Mini Program live commerce) are labelled `直播流` when the URL ends in `.flv`, or the response is `video/x-flv` without a `Content-Length`; the detail drawer shows the detection basis. Downloading is **continuous recording** with no total size and no natural end: press `x` (or confirm quit) to stop, and the recorded part is saved as `.flv` (playable in ffmpeg/VLC or remuxable). Live streams cannot be resumed; downloading again starts a new recording. Windows and macOS both ship one complete package that includes ffmpeg; m3u8 needs no network preparation. If the bundled ffmpeg is quarantined, deleted, or corrupted, the app downloads and verifies a managed ffmpeg as a fallback. You can also set `download.ffmpeg` to an existing executable (highest priority). The bundled ffmpeg is a separate GPLv3 program; see `THIRD-PARTY-NOTICES.md` for licensing and source availability. On macOS keep the whole extracted folder; `ffmpeg/` sits outside the `.app`.
- The root certificate and private key are generated and reused in a restricted per-user location and are never shipped. Re-establish trust after changing machine or Windows user. Installing the root certificate or removing the old shared root requests administrator authorization once; the certificate is written to the Windows Local Machine root store and is not removed on exit. The old shared root must be removed separately; see [certificates and migration](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#证书风险与清理).
- Direct downloads are isolated per resource; they resume only when the server provides validation information, otherwise they restart. Old `.part` files that are not bound to a resource are kept but no longer resumed. JSON cross-origin media does not carry interface credentials; if a download fails for that reason, replay the media to capture its own request.
- The bitmap brand logo relies on terminal graphics support (Sixel in Windows Terminal 1.22+, or kitty and similar). When unsupported, under `NO_COLOR`, through tmux/screen/SSH, or in a short window, the app falls back to the block-character mark with no functional impact. Restart after changing font size or display scaling so the probe can measure again.

## Troubleshooting

| Problem | What to do |
| --- | --- |
| No Mini Program discovered | Make sure the proxy is running and reopen the Mini Program; press `r` to refresh or retry, or `Ctrl+D` to enter an AppID manually. |
| No target media in the list | Check the `?` unconfirmed media in the Videos category (H5/web-view videos usually appear there); reload media after confirming the AppID. Requests without a source never become target candidates. |
| Live stream download never ends | Expected: an HTTP-FLV live stream has no total length or end marker, so downloading means recording. Press `x` to stop and save the recorded part (`.flv`); remux with ffmpeg if you need mp4. |
| Cannot find downloaded files | Press `o` to open the current Mini Program folder; files live under `downloads/<AppID>/图片|视频|音频|m3u8|其他/`. Media saved flat by older versions stays in the `downloads/` root and is not moved automatically. |
| Appearance is not remembered | Check write permission on the download folder; the preference lives in `downloads/ui.json` and falls back to follow-system when unreadable. |
| No bitmap logo | The terminal lacks Sixel/kitty support, or you are behind tmux/screen/SSH, set `NO_COLOR`, or have a short window; the block-character mark is the expected fallback. |
| Network broken after a crash | Restart the app, press `Ctrl+D`, wait for the proxy to run, then press `q`. Quitting while still on the target card does not restore the proxy. |
| "Failed to read original system proxy" at startup | This version reads the registry directly and treats a missing value as unset; if it persists, make sure no old build is running. Only certificate install/cleanup requests administrator authorization. |
| Certificate installation failed | Approve the UAC prompt; if it still fails, see [certificate troubleshooting (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#排障). |
| m3u8 download or merge failed | Make sure `ffmpeg/` was not quarantined or deleted; the app downloads a managed ffmpeg when the bundled one is missing or corrupted, and you can set `download.ffmpeg`. The error names the ffmpeg log path; see [troubleshooting (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#排障). |
| Diagnosing sniffing | Start with `wx-mini-video.exe --debug`; the diagnostic log is written to `wx-mini-video-debug.log` in the download folder. Check it for sensitive information before sharing. |

[Maintainer guide: configuration, building, and validation (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md) · [Version history](https://github.com/MuZiJin701/wx-mini-video/blob/main/CHANGELOG.md)

Only download content you have the right to access, and follow applicable laws and platform rules.
