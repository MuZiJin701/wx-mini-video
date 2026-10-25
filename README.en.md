# WeChat Mini Program Media Downloader

Save images, direct videos, HTTP-FLV live streams, direct audio, and m3u8 media loaded by desktop WeChat Mini Programs on Windows (stable) and macOS (unsigned test build).

[Download latest](https://github.com/MuZiJin701/wx-mini-video/releases/latest) · [简体中文](https://github.com/MuZiJin701/wx-mini-video/blob/main/README.md)

> This document describes the unreleased v0.2.0 source (discovery on startup, unified target area, six media categories, per-type download folders, HTTP-FLV live recording). The v0.1.9 packages linked below do not include these new features; build from source to try the current implementation. To upgrade, replace the exe and keep `wx-mini-video.yaml`, `ffmpeg/`, and `downloads/`.

Windows and macOS packages are available together in the [v0.1.9 Release](https://github.com/MuZiJin701/wx-mini-video/releases/tag/v0.1.9):

| Platform | Download | Status |
| --- | --- | --- |
| Windows amd64 | [Windows ZIP](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.9/wx-mini-video-windows-amd64.zip) | Stable |
| macOS 13+ · Apple Silicon / Intel | [Universal Mac ZIP](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.9/wx-mini-video-macos-universal-unsigned.zip) | Unsigned test build |

Bundled ffmpeg is built from pinned official source (FFmpeg 9.0.2 + OpenSSL 3.5.9); corresponding source archives ship in the same Release (see the [third-party notice](THIRD-PARTY-NOTICES.md#对应源码corresponding-source)). Extract the whole Mac archive and open the `.app`; Developer ID signing, notarization, and real Mac WeChat acceptance remain pending. See the [macOS instructions](https://github.com/MuZiJin701/wx-mini-video/blob/main/docs/macos-usage.md).

## Get started

1. Download `wx-mini-video-windows-amd64.zip`, **extract the whole archive**, and double-click `wx-mini-video.exe`; `ffmpeg\ffmpeg.exe` is already included.
2. The app starts the proxy and keeps discovering Mini Programs as soon as it opens, showing "starting the proxy, discovering Mini Programs…" and the proxy status. A `target.appID` in the config only pre-fills the input field; it is **not** confirmed automatically at startup. The first proxy start generates and installs a root certificate unique to the current user; administrator (UAC) authorization is requested only when installing the certificate or removing the old shared root. If startup fails, click "Retry · Ctrl+R" or press `Ctrl+R`; `r` also works when the input is not focused. The footer "Quit · Ctrl+Q" button closes the app.
3. **Choose a target**: type a known AppID in the unified target area and press `Enter` (cursor editing is supported), or open the Mini Program in desktop WeChat, wait for the discovered list to update, use `Tab` to focus that list, then select with `↑/↓` and press `Enter`. Clicking a discovered item confirms it directly. Discovery never selects a target automatically, overwrites your input, or takes focus. `Tab`/`Shift+Tab` cycle through Appearance → Input → Discovered list (when populated) → Confirm → Cancel (when changing targets).
4. After confirmation the full input field and discovered list collapse, leaving the target bar and the "Change target · s" button. Reload images or play video/audio in desktop WeChat. The media view opens on All; `1`–`6` switch All / Images / Videos / Audio / Live / Segmented media. Select with `↑/↓`, press `d`/`Enter` to download, and `o` to open the current Mini Program's folder. `✓` means the source is confirmed, `?` unconfirmed. HTTP-FLV live streams are marked "live"; downloading records continuously — press `x` to stop and save. If the stream drops, recorded content is saved automatically and the reason is reported.
5. Press `q` or click the footer "Quit · Ctrl+Q" button to quit and restore the previous system proxy; quitting during a download or recording asks for confirmation. The app stops and saves the in-progress work first; if saving fails, quitting is cancelled and the error UI stays open so you can handle it and quit again. If proxy restoration fails, press `r` to retry or `f` to force exit. In the target area `Esc` cancels a target change (or quits when no target is confirmed); while the input field has focus `q` is plain text, so use the footer button or `Ctrl+C`/`Ctrl+Q` to quit.

## Interface and operation

Since v0.2.0 the TUI has a single main surface: brand area (bitmap logo when supported, otherwise a block-character mark, plus the "Appearance <mode> ▾" dropdown) → gradient rule → main area (unified target area, or target bar + category tabs + action buttons + candidate table + status line) → footer (GitHub link and "Quit · Ctrl+Q" button). The detail drawer (`i`), log overlay (`l`), appearance dropdown, quit confirmation, legacy-certificate confirmation, and proxy-restore failure screen all have clickable close/cancel/retry/quit buttons.

The media view places eight clickable action buttons above the table, each labelled with its shortcut: Download · d, Stop · x, Details · i, Logs · l, Refresh · r, Open folder · o, Clear · c, Change target · s. At 80×24 or larger, buttons have three-line borders and centered labels, with panel backgrounds for secondary actions; smaller windows use single-line chips. Stop always stays visible: disabled when idle and clickable during a download or recording. An empty list disables Download and Details; downloading/recording disables another download and target changes. The target area also always offers Refresh/Retry (Ctrl+R) and Folder (Ctrl+O). The footer Quit button keeps its border at every supported size.

The mouse wheel over the resource list scrolls the viewport without changing the selected resource. Clicking selects a candidate, and keyboard arrows bring the selection back into view. Scrolling over buttons, the footer, or the appearance dropdown does not scroll the resource list behind them.

Confirmation follows the active control: `Enter` in the input confirms only the typed text, and empty input shows an error; `Enter` in the discovered list confirms only its selected item. The Confirm target button prefers a deliberately selected discovered item; editing the input switches the button back to the typed value. With no deliberate list selection and empty input, the button uses the current list item. Clicking Confirm or Cancel performs that button's action regardless of the previous keyboard focus. Printable characters such as `r`, `q`, and digits are text while the input is focused.

Generate interface previews from the source root with `go run -tags preview ./build/uipreview`; image conversion is described in the [maintainer guide (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#开发验证). Previews use the actual `View` renderer and sample states. Real WeChat traffic, terminal mouse interaction, and resizing still need acceptance testing on actual machines. Preview files stay local.

### Appearance dropdown

The "Appearance <current mode> ▾" control in the brand area is clickable. In the target area, `Tab` can focus it: `←/→` cycle through the three modes directly, `Enter` or a click opens the dropdown, `↑/↓` plus `Enter` or a click selects Follow system / Light / Dark, and `Esc` closes it. In the media view, clicking the control opens the same dropdown. While the input field has focus, `←/→` only move the text cursor.

The preference is written atomically to `downloads/ui.json` and reused on the next start. Follow-system mode reads the system appearance on the UI refresh timer, so no restart is needed. Windows falls back to dark if reading fails; macOS uses light when no dark preference is read. Select a mode manually to override this. The bitmap logo requires terminal graphics support (Sixel in Windows Terminal 1.22+, or kitty) and fits at 60×20 or larger, using a three-row image below 32 rows. Without graphics it falls back to a block-character mark; very narrow windows or heights below 20 hide the mark. The Windows exe uses the same brand artwork for its file icon. Windows Terminal's tab icon is controlled by its profile and does not automatically inherit the exe icon; this app does not modify global Terminal settings.

## Media categories and live streams

The category tabs are always `1 All / 2 Images / 3 Videos / 4 Audio / 5 Live / 6 Segmented media`, and the media view opens on All.

- **Live is separate and mutually exclusive**: HTTP-FLV live candidates appear only under All and Live, never among ordinary Videos. `.flv` URLs extracted from JSON/H5 are first marked as suspected live and displayed as "live"; once the real response proves a finite FLV file with a length, the candidate returns to Videos and selection follows candidate identity. If a live-status correction moves the selected item out of the current category, the selection is cleared with a notice so another resource cannot be downloaded by accident.
- **Segmented media**: the m3u8 category is displayed as "Segmented media"; details keep the m3u8/HLS format information and the note that the playlist is merged into mp4. HLS video, audio, and playlists all stay in this group.
- **Unknown files** appear only under All; no extra category is added.
- **Scope**: this release supports continuous recording of **HTTP-FLV only**. Continuous HLS (m3u8) live recording is **not supported**; a playlist without `#EXT-X-ENDLIST` may only be saved as the current snapshot and merged into mp4, which is not a complete live recording.

## Downloaded files

```text
downloads/
├── <target AppID>/
│   ├── 图片/ (images)   视频/ (videos)   音频/ (audio)
│   ├── 直播/      # HTTP-FLV continuous recordings
│   ├── 分段媒体/  # merged m3u8 mp4, playlist cache, .segments chunks
│   └── 其他/      # unknown binaries
├── wx-mini-video.log, wx-mini-video-debug.log
├── history.jsonl   # target AppID, kind, source, size per success
├── ui.json         # appearance preference
├── WxMiniVideoRoot.cer
└── ffmpeg.exe (ffmpeg on macOS) and .ffmpeg/ failure logs
```

- Files land under their final type folder: HTTP-FLV recordings in `直播/`, ordinary finite videos (including FLVs confirmed as finite files) in `视频/`, merged m3u8 output plus playlist cache and `.segments` chunks in `分段媒体/`, and unknown files in `其他/`. All is only an aggregate view and has no folder.
- Default file names are `type_YYYYMMDD-HHMMSS_shortID.ext`, e.g. `视频_20261025-130512_9f8e7d6c.mp4`; live recordings use the `直播_` prefix and segmented media uses `分段媒体_`, and HTTP-FLV recordings are saved as `.flv`. Collisions get `_2`, `_3`.
- Candidates whose type is only resolved during download move to the correct folder: a suspected live stream confirmed as a finite file moves back to `视频/`, and a stream confirmed as live at download time moves to `直播/` with a `.flv` suffix. History, progress, and the opened location all use the final real path, and cross-folder moves never overwrite existing output or unsaved recordings. If creating the final folder or moving the output fails, the file stays in its original folder with an explicit "archiving failed" error: it is not reported as a success and is not written to history.
- Unconfirmed (`?`) candidates are filed under the target active at download time, and `history.jsonl` keeps the request source AppID.
- Logs, `history.jsonl`, `ui.json`, the exported root certificate, managed ffmpeg, and `.ffmpeg/` stay in the download root; keep them with `downloads/` when moving or upgrading.
- Media saved flat in the root and the old `m3u8/` folder by older versions are **not migrated or deleted automatically**, and re-downloading creates a new file. The program never clears `downloads/` on startup.

## Common keys

| Key | Action |
| --- | --- |
| `↑/↓` | Select a candidate (media view) or a discovered Mini Program (when that list has focus) |
| `k/j` | Select a candidate in the media view (same as `↑/↓`) |
| `d`, `Enter` | Download the selection; live streams start recording (in the target area, `Enter` confirms the focused action) |
| `x` | Stop the current download; a live recording is saved as `.flv` |
| `Tab`, `Shift+Tab` | Cycle focus in the target area; switch categories in the media view |
| `1`–`6` | Categories: All / Images / Videos / Audio / Live / Segmented media |
| `i`, `l` | Toggle the detail drawer / log overlay (`Esc` or a click closes) |
| `c` | Clear the list (local files are kept) |
| `r`, `Ctrl+R` | Retry proxy startup after a failure; otherwise refresh candidates and the discovered list (use `Ctrl+R` while the input has focus) |
| `o`, `Ctrl+O` | Open the current Mini Program folder (download root when no target or missing) |
| `s` | Expand the target area to change targets (unavailable while downloading/recording) |
| `q`, `Ctrl+Q`, `Ctrl+C` | Quit and restore the proxy (confirmation while downloading/recording; `q` is text while the input has focus) |
| `←/→` | Move the text cursor when the input has focus; switch appearance when the appearance control has focus |
| `Home/End`, `Ctrl+A/E`, `Backspace`, `Delete`, `Ctrl+U` | Edit the AppID input |
| `Esc` | Close details/logs/the appearance dropdown; cancel a target change keeping the current target; quit when no target is confirmed |
| Mouse | Click tabs, rows (double-click downloads), buttons, and the appearance dropdown; click a discovered item to confirm it; the resource-list wheel scrolls the viewport while preserving selection, and discovery/logs retain their own scrolling behavior |

## Before you start

- Only one target is sniffed at a time. Discovery starts automatically but does not equal selection: `target.appID` only pre-fills the input, the runtime target stays empty until you confirm, and during discovery only Mini Programs and unconfirmed media are recorded — no target candidates are created (per [ADR 0008](https://github.com/MuZiJin701/wx-mini-video/blob/main/docs/adr/0008-target-attribution.md)). Reload content after confirming a target to populate its candidates. Switching targets clears target candidates and progress but keeps local files and unconfirmed media; changing targets is blocked while downloading or recording.
- **Target candidates** require the request source AppID to exactly match the target (`✓`). H5/web-view pages, native players, and CDNs often lack the Mini Program page-frame Referer, so such media is collected as unconfirmed (`?`) under its type category and may be downloaded at your discretion. The tool also scans H5 pages/scripts for inline media URLs, so opening a Mini Program's H5 detail page in WeChat can produce candidates without playback (marked "H5 page/script scan, unverified"). Set `capture.showUnattributed: false` to disable unconfirmed media.
- Images, direct videos, and direct audio are saved as-is without transcoding. **HTTP-FLV live streams** (`.flv` URL or `video/x-flv` without `Content-Length`) have no total size or end point and cannot resume; press `x` or confirm quit to stop and save `.flv`. If the stream drops unexpectedly, the recorded part is still saved and added to history; when saving fails, the real error is shown and the `.part` is kept for recovery. Retrying never overwrites an unsaved live recording, and streams confirmed as live only at download time are also saved as `.flv` under `直播/`. Play or remux the recording with ffmpeg/VLC. **Continuous HLS (m3u8) live recording is out of scope for this release.**
- Windows and macOS each ship one complete package; m3u8 needs no network preparation. If the bundled ffmpeg is missing or corrupted the app downloads a managed fallback, and `download.ffmpeg` can point to an existing executable. The self-built ffmpeg in release packages is a separate LGPLv3 program; see the [third-party notice](THIRD-PARTY-NOTICES.md).
- The root certificate and private key are generated and reused in a restricted per-user location and are never shipped; re-establish trust after changing machine or Windows user. Exit only restores the proxy and does not remove the certificate. Direct downloads resume with Range when the server provides validation information; after a failure, retrying reuses the `.part` left by the same candidate (live recordings do not reuse old files). JSON cross-origin media does not carry interface credentials.

## Troubleshooting

| Problem | What to do |
| --- | --- |
| No Mini Program discovered | Discovery starts automatically when the app opens; make sure "proxy running" is shown, then reopen the Mini Program. Press `Ctrl+R` to refresh/retry, or type the AppID in the target area. |
| A Mini Program was found but not selected | Expected: discovery does not select. Use `Tab` to focus the discovered list, select with `↑/↓`, and press `Enter`, or click the item directly. |
| The UI says proxy startup failed | Click "Retry · Ctrl+R" or press `Ctrl+R` (`r` also works outside the input); handle certificate/UAC prompts if it keeps failing; the footer "Quit · Ctrl+Q" button exits. |
| No target media in the list | Press `1` to view All (including `?` unconfirmed media); reload media after confirming the target. |
| Live download never ends | Expected: HTTP-FLV has no end point. Press `x` to stop and save `.flv`; remux to mp4 with ffmpeg if needed. |
| Can HLS/m3u8 live streams be recorded? | Not in this release. HLS playlists are saved as Segmented media and merged from the current playlist; without `#EXT-X-ENDLIST` this may be a snapshot. |
| Is recorded content kept after a drop? | Yes: on an unexpected drop the recorded part is saved as `.flv` and written to history, with the reason shown in the status line and logs. If saving fails, the real error and the `.part` are kept for retry or manual recovery. |
| Will a retry re-download everything? | Direct downloads reuse the same candidate's `.part` and resume (the server must provide validation such as ETag). Live recordings start a new file on every retry and never overwrite an unsaved recording. |
| Saving failed while quitting | Quitting is cancelled automatically; the error and `.part` are kept so you can handle them and press `q` again (`l` opens the log). |
| Download finished but archiving failed | The file is kept in the original folder named in the error (usually because the final category folder cannot be created or is occupied) and is not written to history. Clear the obstruction and download again to file it into the correct folder. |
| Cannot find downloaded files | Press `o` to open the current folder and look under `图片/视频/音频/直播/分段媒体/其他`; files saved flat by older versions stay in the `downloads/` root and are not moved automatically. |
| What about old downloads and the old m3u8 folder? | The new version does not migrate or delete them and never clears `downloads/` on startup; tidy them up manually if needed. |
| Appearance or logo not applied | Check write permission on `downloads/`; the bitmap logo needs a Sixel/kitty terminal (e.g. Windows Terminal 1.22+), and the block mark is the expected fallback; very narrow windows hide the appearance control. |
| Network broken after a crash | Restart, wait for "proxy running", then press `q` (use `Ctrl+C` or the footer "Quit · Ctrl+Q" button while the input has focus). |
| Certificate installation failed | Approve UAC; see [certificate troubleshooting (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#排障). |
| m3u8 download or merge failed | Make sure `ffmpeg/` is intact; the error names the `.ffmpeg/` log; see [troubleshooting (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md#排障). |
| Diagnosing sniffing | Start with `--debug`; the log is written to `wx-mini-video-debug.log`. Check for sensitive information before sharing. |

[Maintainer guide (Chinese)](https://github.com/MuZiJin701/wx-mini-video/blob/main/wx-mini-video-knowledge.md) · [Version history](https://github.com/MuZiJin701/wx-mini-video/blob/main/CHANGELOG.md)

Only download content you have the right to access, and follow applicable laws and platform rules.
