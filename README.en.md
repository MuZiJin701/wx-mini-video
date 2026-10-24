# WeChat Mini Program Media Downloader

Save images, direct videos, direct audio, and m3u8 media loaded by desktop WeChat Mini Programs on Windows (stable) and macOS (unsigned test build).

[Download latest](https://github.com/MuZiJin701/wx-mini-video/releases/latest) · [简体中文](https://github.com/MuZiJin701/wx-mini-video/blob/main/README.md)

Windows and macOS packages are available together in the [v0.1.8 Release](https://github.com/MuZiJin701/wx-mini-video/releases/tag/v0.1.8):

| Platform | Download | Status |
| --- | --- | --- |
| Windows amd64 | [Windows ZIP](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.8/wx-mini-video-windows-amd64.zip) | Stable |
| macOS 13+ · Apple Silicon / Intel | [Universal Mac ZIP](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.8/wx-mini-video-macos-universal-unsigned.zip) | Unsigned test build |

> Note: the v0.1.8 assets linked above still use the old layout (no bundled ffmpeg, and the Windows auto-download URL is no longer valid). Bundled ffmpeg, the unattributed-media list, and related changes ship in the next version.

Extract the whole Mac archive and open the `.app`; media is saved to `downloads/` beside it. Developer ID signing, notarization, and real Mac WeChat acceptance remain pending. First launch may require per-app system approval or the documented app move step. See the [macOS instructions (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/docs/macos-usage.md). The steps below apply to Windows.

## Get started

1. Download `wx-mini-video-windows-amd64.zip`, **extract the whole archive**, and double-click `wx-mini-video.exe`. The archive also contains `ffmpeg\ffmpeg.exe`; no separate download or configuration is needed.
2. Accept the administrator (UAC) prompt. The app prefers an installed Windows Terminal. Starting the proxy for the first time generates and installs a root certificate unique to the current Windows user on this computer. If an old shared root is detected, choose whether to remove it when prompted.
3. **Choose a Mini Program**: enter its AppID if known; the name is optional. Otherwise press `Ctrl+D`, open the Mini Program in desktop WeChat, select its AppID with `↑/↓`, press `Enter` to fill in its name, then `Enter` again to confirm.
4. **Reload the media**: once the proxy is running, reopen the image or replay the video or audio. Select a candidate with `↑/↓`, press `d` or `Enter` to download, and `o` to open the download folder. The list starts on Videos; press `1` for All, `6` for unattributed media, or `5` for Audio. Confirmed candidates show `✓`; unattributed ones show `?`.
5. Press `q` when finished to restore the previous system proxy. If restoration fails, the error screen stays open: press `r` to retry or `f` to explicitly force exit. On the AppID entry screen, use `Esc` or `Ctrl+C` to quit.

Files are saved to `downloads/` beside the app by default. To upgrade, replace the exe and keep `wx-mini-video.yaml`, `ffmpeg/`, and `downloads/`.

![Media list: select a candidate and download](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/使用界面.png)

<details>
<summary>View workflow screenshots</summary>

Enter an AppID or press `Ctrl+D` for discovery:

![AppID entry](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/首页.png)

Once the proxy is running, open the Mini Program in desktop WeChat:

![Waiting for Mini Program requests](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/自动探测.png)

Select the target, then confirm its name:

![Select a discovered Mini Program](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/已探测到.png)

When the download finishes, press `o` to view the file:

![Completed image download](https://raw.githubusercontent.com/MuZiJin701/wx-mini-video/main/docs/images/下载完成.png)

</details>

## Common keys

| Key | Action |
| --- | --- |
| `↑/↓`, `k/j` | Select a candidate |
| `d`, `Enter` | Download the selection |
| `Tab`, `Shift+Tab` | Switch categories |
| `1/2/3/4/5` | All / Images / Videos / m3u8 / Audio (target candidates) |
| `6` | Unattributed media (H5/web-view, player, or otherwise unconfirmed sources) |
| `c` | Clear the list and discard earlier in-flight results; keep local files |
| `s` | Return to target selection; blocked during downloads |
| `i`, `l` | Toggle details and logs |
| `o` | Open the download folder |
| `q` | Quit discovery or the media screen and restore the proxy |

## Before you use it

- Only one target is active at a time. Discovery collects no media; reload content after confirming a target. Switching clears target candidates and progress while keeping local files and the unattributed list.
- **Target candidates** are accepted only when the request's source AppID exactly matches the target and are marked `✓`. Media with a missing or non-matching source no longer disappears: it goes to **unattributed media** (press `6`, marked `?`), because H5/web-view pages, native players, and CDN requests often lack the Mini Program page-frame Referer. You can review and download unattributed media yourself; manual AppID entry does not turn it into target media. Names are optional labels and do not affect filtering. Set `capture.showUnattributed: false` to hide the unattributed list (unconfirmed media is then not shown).
- Images, direct videos, and direct audio are saved unchanged. The default **full package bundles ffmpeg**, so m3u8 needs no online preparation; the **slim package** downloads and verifies a managed ffmpeg on first m3u8 use. To use an existing executable, set `download.ffmpeg` to its full path (highest priority). The bundled ffmpeg is a separate GPLv3 program; see `THIRD-PARTY-NOTICES.md` for license and source-obtaining information. On macOS keep the whole extracted folder: `ffmpeg/` lives outside the `.app`.
- The root certificate and private key are generated and reused in a restricted local user directory and are not distributed with the app. A different computer or Windows user needs a new trust setup. Installation still changes Windows system trust; quitting does not remove the certificate. Old shared roots require separate removal. See [certificates and migration (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#证书风险与清理).
- Partial direct downloads are isolated by resource and resumed only with server validators; otherwise they restart. Legacy `.part` files remain untouched and are not reused. Cross-origin media extracted from JSON receives no API credentials; replay the media to capture its own request if authentication is needed.

## Troubleshooting

| Problem | What to do |
| --- | --- |
| Mini Program not discovered | Check that the proxy is running and reopen the Mini Program. Press `r` to refresh or retry, or `Ctrl+D` for manual entry. |
| Expected media is missing | Press `6` for unattributed media (H5/web-view videos usually appear there), confirm the AppID, then reload the media. Requests without a matching source never enter the target list. |
| Network fails after an abnormal exit | Restart the app, press `Ctrl+D`, wait until the proxy is running, then press `q`. Quitting directly from the entry screen does not restore the proxy. |
| Certificate installation fails | Check that you accepted UAC; see [certificate troubleshooting (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#排障). |
| m3u8 download or merge fails | In the full package, make sure `ffmpeg/` was not quarantined or deleted and re-extract if needed; the slim package downloads a managed ffmpeg automatically, or set `download.ffmpeg`. The error includes the ffmpeg log path; see [troubleshooting (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#排障). |
| Need to debug sniffing | Set `WX_MINI_VIDEO_DEBUG=1` and restart; diagnostic logs are written to `wx-mini-video-debug.log` in the download folder. Check for sensitive information before sharing. |

[Maintenance: configuration, builds, and acceptance checks (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md) · [Changelog (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/CHANGELOG.md)

Only save content you have permission to access, and follow applicable laws and platform rules.
