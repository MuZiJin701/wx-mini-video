# WeChat Mini Program Media Downloader

Save images, direct videos, HTTP-FLV live streams, direct audio, and m3u8 media loaded by desktop WeChat Mini Programs on Windows (stable) and macOS (unsigned test build).

[Download latest](https://github.com/MuZiJin701/wx-mini-video/releases/latest) · [简体中文](README.md)

> This document describes the v0.2.0 source (single main surface, per-AppID download folders, HTTP-FLV live recording). Until v0.2.0 is released, the Release channel still serves v0.1.9; to upgrade, replace the exe and keep `wx-mini-video.yaml`, `ffmpeg/`, and `downloads/`.

Windows and macOS packages are available together in the [v0.1.9 Release](https://github.com/MuZiJin701/wx-mini-video/releases/tag/v0.1.9):

| Platform | Download | Status |
| --- | --- | --- |
| Windows amd64 | [Windows ZIP](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.9/wx-mini-video-windows-amd64.zip) | Stable |
| macOS 13+ · Apple Silicon / Intel | [Universal Mac ZIP](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.9/wx-mini-video-macos-universal-unsigned.zip) | Unsigned test build |

Bundled ffmpeg is built from pinned official source (FFmpeg 9.0.2 + OpenSSL 3.5.9); corresponding source archives ship in the same Release (see the [third-party notice](THIRD-PARTY-NOTICES.md#对应源码)). Extract the whole Mac archive and open the `.app`; Developer ID signing, notarization, and real Mac WeChat acceptance remain pending. See the [macOS instructions](docs/macos-usage.md).

## Get started

1. Download `wx-mini-video-windows-amd64.zip`, **extract the whole archive**, and double-click `wx-mini-video.exe`; `ffmpeg\ffmpeg.exe` is already included.
2. The app runs as the current user and prefers an installed Windows Terminal. Starting the proxy for the first time generates and installs a root certificate unique to the current user; administrator (UAC) authorization is requested only when installing the certificate or removing the old shared root.
3. **Choose a Mini Program**: type a known AppID on the target card and press `Enter`; otherwise press `Ctrl+D`, open the Mini Program in desktop WeChat, select the AppID with `↑/↓`, and press `Enter` to confirm (v0.2.0 no longer asks for a name).
4. **Reload the media**: once the proxy is running, reopen images or replay video/audio in desktop WeChat. Select with `↑/↓`, press `d`/`Enter` to download, and `o` to open the current Mini Program's folder. The list starts on Videos; `1` shows all and `5` shows audio. `✓` means the source is confirmed, `?` unconfirmed. HTTP-FLV live streams are marked "live"; downloading records continuously — press `x` to stop and save.
5. Press `q` to quit and restore the previous system proxy; quitting during a download asks for confirmation. If restoration fails, press `r` to retry or `f` to force exit. On the target card, `Esc`/`Ctrl+C` quits.

## Interface and appearance

Since v0.2.0 the TUI has a single main surface: brand area (bitmap logo when supported, otherwise a block-character mark, plus the appearance switch) → gradient rule → main area (target card, or target bar + category tabs + candidate table + status line) → footer. The target overlay, detail drawer (`i`), log overlay (`l`), quit confirmation, and certificate confirmation are overlays. Tabs, rows, buttons, and the appearance control accept mouse clicks; all keyboard shortcuts remain available.

**Appearance** supports **follow system / light / dark**, switched with `←/→` or a click and remembered in `downloads/ui.json`. When the system theme cannot be read it falls back to dark. The bitmap logo needs terminal graphics support (Sixel in Windows Terminal 1.22+, or kitty); unsupported terminals, `NO_COLOR`, tmux/screen/SSH, or a short window fall back to the block-character mark.

![Media list: select a candidate and download (live stream selected)](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/使用界面.png)

<details>
<summary>View workflow screenshots</summary>

![Target card](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/首页.png)

![Discovered Mini Programs](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/已探测到.png)

![Completed download](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/下载完成.png)

![Live recording](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/直播录制.png)

</details>

## Downloaded files

```text
downloads/
├── <target AppID>/
│   ├── 图片/ (images)   视频/ (videos)   音频/ (audio)
│   ├── m3u8/   # merged mp4, playlist cache, segment temp files
│   └── 其他/   # unknown binaries
├── wx-mini-video.log, wx-mini-video-debug.log
├── history.jsonl   # target AppID, kind, source, size per success
├── ui.json         # appearance preference
├── WxMiniVideoRoot.cer
└── ffmpeg.exe (ffmpeg on macOS) and .ffmpeg/ failure logs
```

- Default file names are `type_YYYYMMDD-HHMMSS_shortID.ext`, e.g. `视频_20261025-130512_9f8e7d6c.mp4`; merged m3u8 is `.mp4`, HTTP-FLV recordings are `.flv`; collisions get `_2`, `_3`.
- Unconfirmed (`?`) candidates are filed under the target active at download time, and `history.jsonl` keeps the request source AppID. Media saved flat by older versions is not migrated or deleted; re-downloading creates a new file.
- Logs, `history.jsonl`, `ui.json`, the exported root certificate, managed ffmpeg, and `.ffmpeg/` stay in the download root; keep them with `downloads/` when moving or upgrading.

## Common keys

| Key | Action |
| --- | --- |
| `↑/↓`, `k/j` | Select a candidate / row |
| `d`, `Enter` | Download the selection; live streams start recording |
| `x` | Stop the current download; a live recording is saved as `.flv` |
| `Tab`, `Shift+Tab`, `1/2/3/4/5` | Switch categories (All / Images / Videos / m3u8 / Audio) |
| `←/→` | Switch appearance: follow system / light / dark |
| `s`, `Ctrl+D` | Open the target overlay (`Esc` cancels, keeping the current target and candidates) |
| `c` | Clear the list (local files are kept) |
| `i`, `l` | Toggle the detail drawer / log overlay |
| `o`, `Ctrl+O` | Open the current Mini Program folder (download root when no target or missing) |
| `r`, `Ctrl+R` | Retry proxy startup after a failure; otherwise refresh candidates |
| `q`, `Ctrl+Q` | Quit and restore the proxy (confirmation during a download) |
| Mouse | Click tabs, rows (double-click downloads), buttons, and the appearance control; wheel scrolls the list or logs |

## Before you start

- Only one target is sniffed at a time. Discovery does not create target candidates; reload content after confirming a target. Switching targets clears target candidates and progress but keeps local files and unconfirmed media.
- **Target candidates** require the request source AppID to exactly match the target (`✓`). H5/web-view pages, native players, and CDNs often lack the Mini Program page-frame Referer, so such media is collected as unconfirmed (`?`) under its type category and may be downloaded at your discretion. The tool also scans H5 pages/scripts for inline media URLs, so opening a detail page can produce candidates without playback (marked "H5 page/script scan, unverified"). Set `capture.showUnattributed: false` to disable unconfirmed media.
- Images, direct videos, and direct audio are saved as-is without transcoding. **HTTP-FLV live streams** (`.flv` URL or `video/x-flv` without `Content-Length`) have no total size or end point and cannot resume; press `x` or confirm quit to stop and save `.flv`, then play or remux with ffmpeg/VLC. HLS live streams are out of scope for now.
- Windows and macOS both ship one complete package; m3u8 needs no network preparation. If the bundled ffmpeg is missing or corrupted the app downloads a managed fallback, and `download.ffmpeg` can point to an existing executable. The bundled ffmpeg is a separate GPLv3 program; see `THIRD-PARTY-NOTICES.md`.
- The root certificate and private key are generated and reused in a restricted per-user location and are never shipped; re-establish trust after changing machine or Windows user. Exit only restores the proxy and does not remove the certificate. Direct downloads resume only when the server provides validation information, and old `.part` files are no longer resumed; JSON cross-origin media does not carry interface credentials.

## Troubleshooting

| Problem | What to do |
| --- | --- |
| No Mini Program discovered | Make sure the proxy is running and reopen the Mini Program; press `r` to refresh/retry, or `Ctrl+D` to enter an AppID. |
| No target media in the list | Check the `?` unconfirmed media in Videos; reload media after confirming the target. |
| Live download never ends | Expected: HTTP-FLV has no end point. Press `x` to stop and save `.flv`; remux to mp4 with ffmpeg if needed. |
| Cannot find downloaded files | Press `o` to open the current folder; files saved flat by older versions stay in the `downloads/` root. |
| Appearance or logo not applied | Check write permission on `downloads/`; the bitmap logo needs a Sixel/kitty terminal, and the block mark is the expected fallback. |
| Network broken after a crash | Restart, press `Ctrl+D`, wait for the proxy to run, then press `q`. |
| Certificate installation failed | Approve UAC; see [certificate troubleshooting (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#排障). |
| m3u8 download or merge failed | Make sure `ffmpeg/` is intact; the error names the `.ffmpeg/` log; see [troubleshooting (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#排障). |
| Diagnosing sniffing | Start with `--debug`; the log is written to `wx-mini-video-debug.log`. Check for sensitive information before sharing. |

[Maintainer guide (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md) · [Version history](CHANGELOG.md)

Only download content you have the right to access, and follow applicable laws and platform rules.
