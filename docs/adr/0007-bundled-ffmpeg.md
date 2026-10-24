# 默认随完整包提供 ffmpeg，并把受管下载改为兜底

状态：已确认（2026-10-25）。修订 [ADR 0001](0001-managed-ffmpeg.md)。

## 背景

v0.1.8 依赖的 Windows 受管 ffmpeg 固定下载地址（BtbN 日构建，2026-08-11）已被上游清理，实测返回 404；代码内置的 SHA-256 与旧月末构建也不一致，只改 tag 仍会校验失败。用户要求新版本默认直接提供 ffmpeg，同时修复自动下载逻辑；`docs/bundled-ffmpeg-requirements.md` 的 Q1/Q2 已确认完整包覆盖 Windows 与 macOS。

## 决策

- 默认分发**完整包**：Windows 包内为 `ffmpeg\ffmpeg.exe`，macOS 包内为 `.app` 外的 `ffmpeg/ffmpeg-amd64`、`ffmpeg/ffmpeg-arm64`；随包附 `ffmpeg/LICENSE.txt` 与根目录 `THIRD-PARTY-NOTICES.md`。保留 `windows-slim` 精简包构建入口，发布时精简包作为并列附件保留。
- ffmpeg 查找优先级：显式 `download.ffmpeg` > 随包 ffmpeg > 受管下载的 ffmpeg。随包文件只做“存在 + 可执行”的轻量检查，不在渲染路径做大文件哈希；缺失或损坏时自动回退受管下载。
- 随包 ffmpeg 通过 `os.Executable()` 定位，不使用 `Config.RootDir`（设置 `WX_MINI_VIDEO_CONFIG` 时后者会变成配置目录）；macOS 从 `.app/Contents/MacOS` 回退到 `.app` 父目录。
- Windows 随包固定 **gyan 9.0.2 essentials**（GPLv3，包内带 GPLv3 全文与 FFmpeg commit `946fcce07b`）；macOS 随包固定 **Martin Riedl 9.0.2 amd64/arm64 原始构建**，不 `lipo` 合并（避免构成 GPLv3 §5 修改版），运行时按 `runtime.GOARCH` 选择。
- 受管下载改为候选源列表：Windows 首选 gyan 9.0.2、备用 BtbN 2026-09-30 月末 8.1 LGPL；macOS 沿用 Martin 9.0.2 两个地址。每个源独立固定 URL 与 SHA-256，下载失败可换源；旧 marker 因摘要变化自动失效并重新获取，不阻断启动。
- 构建：`build.bat windows` 默认产出完整包（构建时联网下载并校验 SHA-256，缓存在 `dist/`），`build.bat windows-slim` 保留原三文件包；`build/macos` 同样下载并校验两份 Martin ZIP。两个包校验脚本按新条目集更新。

## 备选方案

- **go:embed 嵌入可执行文件**：包内文件数不变，但 Windows exe 会增大约 100–165 MB、首次释放再占一份磁盘，且同一文件包含 GPL 作品会提高 combined work 认定风险；不采用。
- **只修自动下载、不内置**：改动最小，但首次仍需下载 100 MB 以上，且依赖第三方地址长期可用，不满足“默认开箱可用”；仅作为精简包路径保留。
- **自建最小 LGPL ffmpeg**：体积与许可更优，但构建与格式覆盖成本高，留待后续评估。

## 后果

- Windows 完整包约 42.6 MB、macOS 通用完整包约 72.1 MB，相对 v0.1.8 的 5 MB / 9.9 MB 明显增大；离线可完成 m3u8 合并。
- 项目成为 ffmpeg 二进制再分发者，须履行 GPLv3 §6 的对应源码义务：随包 GPLv3 全文与 `THIRD-PARTY-NOTICES.md`，并按规定提供精确 Corresponding Source（书面要约或同地托管）。项目自身仍为 MIT + Commons Clause，两者分别标识；聚合分发不改变项目代码许可。
- macOS 随包 ffmpeg 位于 `.app` 外，用户只拖走 `.app` 会丢失 ffmpeg；文档要求保留整个文件夹。签名与公证只覆盖 `.app`。
