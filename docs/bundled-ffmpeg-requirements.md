# 自带 ffmpeg 完整包需求

状态：2026-10-25 已确认并实施；决策记录见 [ADR 0007](adr/0007-bundled-ffmpeg.md)，修订 [ADR 0001](adr/0001-managed-ffmpeg.md)。

## 已确认

- Q1：完整包随附 ffmpeg，用户完整解压后，首次下载 m3u8 无需另行联网获取 ffmpeg，也无需安装工具或填写可执行文件路径。媒体下载本身仍需要网络；保留当前 TUI 和下载操作。
- Q2：同时提供 Windows amd64 与 macOS Intel / Apple Silicon 完整包。Mac 延续现有测试版状态，签名、公证与真实微信验收要求不变。
- Q3：Windows 与 macOS 均只提供一个完整包；不再提供不含 ffmpeg 的精简包。随包 ffmpeg 缺失或损坏时自动下载兜底。
- Q4：Mac 保留一个含两种架构 ffmpeg 的通用 ZIP；两份 ffmpeg 以原始二进制放在 `.app` 外，不做 lipo 合并，运行时按 `runtime.GOARCH` 选择。
- Q5：随包 ffmpeg 缺失或损坏时回退自动下载受管 ffmpeg，并提示可重新解压或手动指定 `download.ffmpeg`。

## 已核实的实现约束

- Windows 完整包允许 `README.md`、`wx-mini-video.exe`、`wx-mini-video.yaml`、`ffmpeg\ffmpeg.exe`、`ffmpeg\LICENSE.txt`、`THIRD-PARTY-NOTICES.md`。Mac 包允许说明、配置、`.app`（内部三文件）、`ffmpeg/ffmpeg-amd64`、`ffmpeg/ffmpeg-arm64`、`ffmpeg/LICENSE.txt`、`THIRD-PARTY-NOTICES.md`。
- 随包 ffmpeg 通过 `os.Executable()` 定位；查找优先级为显式 `download.ffmpeg` > 随包 > 受管下载。随包文件不做每次渲染的哈希校验，只在启动时做一次存在/可执行/`-version` 探测。
- 受管 ffmpeg 位于下载目录，复用需要同时匹配固定来源摘要和可执行文件摘要；随包文件不写 marker。旧版本下载的 marker 因来源摘要变化自动失效。
- 随包 ffmpeg 是 GPLv3 独立程序，按 GPLv3 §6 提供许可证全文、第三方声明与精确对应源码的获取方式（书面要约或托管），项目代码许可与 ffmpeg 许可分别标识。

## 外部来源核查

2026-10-25 实测：原 Windows 固定地址（BtbN `autobuild-2026-08-11-13-11` 的 8.1.2）返回 404，且旧 URL 资产在月末包中的发布方 SHA-256 与代码内置值不一致，只换 tag 仍会校验失败。Windows 现选 gyan 9.0.2 essentials（200，约 109.5 MiB，GPLv3，包内自带 GPLv3 全文与 FFmpeg commit `946fcce07b`，已实测运行），备用 BtbN 2026-09-30 月末 8.1 LGPL。macOS 两个 Martin Riedl 9.0.2 地址（amd64/arm64）均 200，许可 GPLv3，ZIP 内不含许可证或源码，需由本项目补 GPLv3 全文与声明。完整包体积预计 Windows 约 42.6 MB、macOS 约 72.1 MB，低于 GitHub 单附件限制。

## 实施结果

- Windows：`build.bat windows` 下载并校验 gyan ZIP，产出唯一完整包；`verify-package.ps1` 校验收到的条目集。
- macOS：`build/macos` 下载并校验两份 Martin ZIP，解出 `ffmpeg/ffmpeg-amd64`、`ffmpeg/ffmpeg-arm64`（0755），打包 `ffmpeg/LICENSE.txt` 与 `THIRD-PARTY-NOTICES.md`；`verify-macos.py` 校验条目、权限与架构。
- 自动下载改为候选源列表并支持换源；任一候选失败不影响其它候选。
