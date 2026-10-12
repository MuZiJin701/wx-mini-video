# WeChat Mini Program Media Downloader

[简体中文](README.md)

A Windows-first TUI for sniffing and downloading images, direct videos, and m3u8 media loaded by desktop WeChat Mini Programs.

Supported images: `jpg`, `png`, `webp`, `gif`, `bmp`, `avif`

Supported videos: `mp4`, `webm`, `mov`, `m3u8`

## Download

Download `wx-mini-video-windows-amd64.zip` from GitHub Releases. The source repository's `dist/` directory is for local builds.

The extracted package contains `wx-mini-video.exe`, `wx-mini-video.yaml`, and `README.md`.

The package does not bundle ffmpeg. On the first m3u8 download, unless `download.ffmpeg` specifies a local executable, the app downloads and verifies a fixed version of managed `ffmpeg.exe` in the download directory. Temporary network or server errors are retried up to three times. The app does not use ffmpeg from the system `PATH`.

## Usage

1. Right-click `wx-mini-video.exe` and choose **Run as administrator**.
2. Enter the target AppID with an optional name, or press `F2` to start Mini Program discovery.
3. For discovery, open the Mini Program in desktop WeChat, select its AppID with `↑/↓`, and press `Enter` to fill in its name and then `Enter` again to confirm. Missing names appear as “名称未识别” (name not identified). Even a single discovered Mini Program requires selection.
4. After confirming the target, reopen its image or replay its video. Discovery only records Mini Program identifiers; it does not collect media or create media caches. Manual AppID entry goes directly to target sniffing.
5. Select a candidate and press `d` or `Enter` to download it.
6. Press `i` to inspect its full URL, source URL, request header summary, and local cache path.
7. Press `q` to quit and restore the previous system proxy.

The list initially shows videos. Use `Tab` or number keys to switch between All, Images, Videos, and m3u8.

Press `s` to return to target selection. Select another discovered AppID or press `F2` for manual entry. Switching clears candidates and progress while preserving downloads, history, partial downloads, and existing caches. Switching is blocked during a download.

### Finding an AppID

In mobile WeChat, open the Mini Program, tap the top-right `…`, and open its introduction page using the Mini Program name. Tap that page's `…` and choose **More information** to find its AppID. Menu wording varies by WeChat version. Administrators can also find it in the WeChat public platform under **Settings → Basic settings / Development settings**. Alternatively, use this app's `F2` discovery.

Downloads, caches, logs, certificates, and managed ffmpeg are stored in `downloads/` beside the executable by default. Moving the extracted app directory moves this runtime data with it. Set `download.dir` to use an explicit directory.

Direct downloads first write to a `.part` file. Retrying after a network interruption attempts to resume the existing partial download. Successful downloads are recorded in `downloads/history.jsonl`.

## Keyboard shortcuts

| Key | Action |
| --- | --- |
| `↑/↓`, `k/j` | Select a candidate |
| `Tab`, `Shift+Tab` | Switch categories |
| `1/2/3/4` | Show All / Images / Videos / m3u8 |
| `d`, `Enter` | Download the selected candidate |
| `i` | Show or hide candidate details |
| `c` | Clear the list |
| `s` | Return to target selection; blocked during downloads |
| `r` | Refresh the list |
| `o` | Open the download directory |
| `l` | Show or hide logs |
| `PgUp/PgDn` | Scroll logs |
| `q` | Quit and restore the previous system proxy |

## Notes

- The app identifies media through a local HTTPS proxy.
- The first run installs a local root certificate; Windows may request confirmation.
- AppID filters request sources. Only a Referer in the HTTPS `servicewechat.com/<AppID>/<version>/page-frame.html` format with an exact target AppID match is accepted. Other Mini Programs and missing or invalid sources do not produce candidates, media caches, or playlist fallback requests. Normal network forwarding continues.
- Images and direct-video links from matching JSON API responses remain supported. JSON m3u8 links are skipped until a matching playlist response is actually observed.
- Strict filtering may miss target media without source information. Discovery lists only Mini Programs actually observed by the proxy. Automatic names are not guaranteed; names can be entered manually and do not affect filtering.
- A Mini Program may load many candidates at once. Clear the list before opening the content you want.
- If an abnormal exit leaves networking broken, run the app again as administrator and quit with `q`.

## Troubleshooting

### Mini Program fails to open or reports network errors

The system proxy may not have been restored. Run the app again as administrator and quit with `q`.

### Certificate installation fails

Run as administrator. If needed, manually install `WxMiniVideoRoot.cer` from the download directory into **Trusted Root Certification Authorities**.

After the proxy starts, the certificate is also available at `http://127.0.0.1:2023/cert`.

### m3u8 downloads are slow

Long videos contain many segments. Check the TUI's segment counts, failures, and ffmpeg merge progress. If many segments fail, reduce `segmentConcurrency` from `6` to `4`.

### m3u8 merging fails

The error shows the ffmpeg log path: `<download directory>\.ffmpeg\<candidate ID>.log`. Captured request header values are removed from the log.

### Expected media does not appear

Confirm the target AppID, then reopen the image or replay the video. Requests without matching source information are excluded; sharing a CDN domain does not widen the scope. Media outside the supported image, direct-video, and m3u8 formats may not be recognized.

### Target Mini Program is not discovered

Confirm that the proxy started, then reopen the Mini Program in desktop WeChat to generate requests. Press `r` to refresh or retry startup, or `F2` to enter the AppID manually. Both entry methods use the same filtering rules; manual entry cannot bypass missing source information.

## Configuration

The configuration file is `wx-mini-video.yaml`:

```yaml
download:
  dir: ""
  ffmpeg: ""
  mode: "auto"
  segmentConcurrency: 6
  segmentRetries: 3
  keepSegments: false

proxy:
  system: true
  hostname: "127.0.0.1"
  port: 2023
  skipInstallRootCert: false

target:
  appID: ""
  name: ""
```

The defaults normally need no changes. If ffmpeg is already installed, set `download.ffmpeg` to its full executable path; otherwise the app uses managed ffmpeg.

## Development

```powershell
go test -count=1 ./...
go vet ./...
build\build.bat windows
```

Build outputs:

- `dist\wx-mini-video-windows-amd64\`
- `dist\wx-mini-video-windows-amd64.zip`

Build outputs stay local and are excluded from Git. Upload the zip to GitHub Releases for distribution. The build script runs `build\verify-package.ps1` to ensure the zip contains only `README.md`, `wx-mini-video.exe`, and `wx-mini-video.yaml`.

Maintainer documentation is in Chinese:

- `AGENTS.md`: project rules, documentation entry points, and validation requirements.
- `GLOSSARY.md`: domain terminology.
- `wx-mini-video-knowledge.md`: implementation, configuration, distribution, and troubleshooting.
- `docs/adr/`: long-term architecture decisions.
- `docs/agents/`: issue tracker, triage labels, and domain documentation conventions.
- `docs/archive/`: historical documents and completed plans.

## Disclaimer

This project is for technical discussion, learning, and research. Follow applicable laws and do not use it for unlawful purposes.
