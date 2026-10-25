# 默认随完整包提供 ffmpeg，并把受管下载改为兜底

状态：已由 [ADR 0010](0010-source-built-ffmpeg.md) 修订（2026-10-25）：v0.1.9 起随包 ffmpeg 改为固定官方源码自建；受管下载兜底继续使用下列第三方来源。修订 [ADR 0001](0001-managed-ffmpeg.md)。以下保留原始决策背景。

## 背景

v0.1.8 依赖的 Windows 受管 ffmpeg 固定下载地址（BtbN 日构建，2026-08-11）已被上游清理，实测返回 404；代码内置的 SHA-256 与旧月末构建也不一致，只改 tag 仍会校验失败。用户要求新版本默认直接提供 ffmpeg，同时修复自动下载逻辑；`docs/bundled-ffmpeg-requirements.md` 的 Q1/Q2 已确认完整包覆盖 Windows 与 macOS。

## 决策

- Windows 与 macOS 均只分发**一个完整包**：Windows 包内为 `ffmpeg\ffmpeg.exe`，macOS 包内为 `.app` 外的 `ffmpeg/ffmpeg-amd64`、`ffmpeg/ffmpeg-arm64`；随包附 `ffmpeg/LICENSE.txt` 与根目录 `THIRD-PARTY-NOTICES.md`。不再提供不含 ffmpeg 的精简包。
- ffmpeg 查找优先级：显式 `download.ffmpeg` > 随包 ffmpeg > 受管下载的 ffmpeg。随包文件检查存在/可执行，并以有超时的 `-version` 探测实际可用性；同一路径、大小和修改时间的结果缓存，文件替换后重测，不在渲染路径反复启动进程或计算大文件哈希。缺失或损坏时自动回退受管下载。
- 随包 ffmpeg 通过 `os.Executable()` 定位，不使用 `Config.RootDir`（设置 `WX_MINI_VIDEO_CONFIG` 时后者会变成配置目录）；macOS 从 `.app/Contents/MacOS` 回退到 `.app` 父目录。
- Windows 随包固定 **gyan 9.0.2 essentials**（GPLv3，包内带 GPLv3 全文与 FFmpeg commit `946fcce07b`）；macOS 随包固定 **Martin Riedl 9.0.2 amd64/arm64 原始构建**，不 `lipo` 合并，以保留可核对的原始构建，运行时按 `runtime.GOARCH` 选择。
- 受管下载改为候选源列表：Windows 首选 gyan 9.0.2、备用 BtbN 2026-09-30 月末 8.1 LGPL；macOS 沿用 Martin 9.0.2 两个地址。每个源独立固定 URL 与 SHA-256，下载失败可换源；旧 marker 因摘要变化自动失效并重新获取，不阻断启动。
- 构建：`build.bat windows` 产出唯一 Windows 完整包（构建时联网下载并校验 SHA-256，缓存在 `dist/`）；`build/macos` 同样下载并校验两份 Martin ZIP。两个平台的校验脚本按各自条目集更新。

## 备选方案

- **go:embed 嵌入可执行文件**：包内文件数不变，但 Windows exe 会增大约 100–165 MB、首次释放再占一份磁盘，且同一文件包含 GPL 作品会提高 combined work 认定风险；不采用。
- **只修自动下载、不内置**：改动最小，但首次仍需下载 100 MB 以上，且依赖第三方地址长期可用，不满足“默认开箱可用”；不提供独立精简包分发。
- **自建最小 LGPL ffmpeg**：体积与许可更优，但构建与格式覆盖成本高，留待后续评估。

## 后果

- Windows 完整包约 42.6 MB、macOS 通用完整包约 72.1 MB，相对 v0.1.8 的 5 MB / 9.9 MB 明显增大；离线可完成 m3u8 合并。
- 项目成为 ffmpeg 二进制再分发者，网络分发须履行 GPLv3/LGPLv3 对应源码义务；不得以实物分发条款下的书面要约代替。v0.1.9 已改为自建 ffmpeg，并在同一 Release 提供对应源码归档；源码索引人工核对后由 `build/verify-release-sources.py` 校验。项目自身仍为 MIT + Commons Clause，两者分别标识。
- macOS 随包 ffmpeg 位于 `.app` 外，用户只拖走 `.app` 会丢失 ffmpeg；文档要求保留整个文件夹。签名与公证只覆盖 `.app`。
