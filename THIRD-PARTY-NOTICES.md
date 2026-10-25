# 第三方组件与许可声明

本文件说明 `wx-mini-video` 发布包内第三方组件的来源、许可与获取对应源码的方式。项目自身代码的许可（MIT + Commons Clause，见 `LICENSE`）与本文件所列第三方许可分别标识；把第三方程序放进同一个压缩包不改变任何一方的许可，也不表示第三方授权适用于项目代码。

## 随包 ffmpeg（Windows 与 macOS 完整包）

本发布包内的 ffmpeg 由本项目从固定官方源码构建，不再使用第三方预编译二进制。

- 组件：FFmpeg 9.0.2（自建，LGPLv3，静态链接 OpenSSL 3.5.9）
- FFmpeg 官方源码：<https://ffmpeg.org/releases/ffmpeg-9.0.2.tar.xz>
  - SHA-256：`8c3850283eb25fa026482078a04051e0be17347b09ef81a0849bec15a96e002e`
- OpenSSL 官方源码：<https://github.com/openssl/openssl/releases/download/openssl-3.5.9/openssl-3.5.9.tar.gz>
  - SHA-256：`603f5602e2eef00d77fbd429d34dcd5822bb301757a1bc9cdb24c670f1eb859a`
- Windows 构建工具链：固定 w64devkit 2.10.1，以及固定 MSYS2、Perl 与 Make；实际下载地址、SHA-256 和完整构建参数记录在对应源码归档的 `recipe/sources.json` 与 `records/` 中。
- 构建配置：`--disable-autodetect --disable-gpl --disable-nonfree --enable-version3 --enable-openssl --disable-x86asm`，不链接 x264/x265 等外部编码库；使用 `-c copy` 完成 m3u8 合并。
- 随包文件：
  - Windows：`ffmpeg/ffmpeg.exe`，SHA-256 `28ac632a227f7797197c44d0a3a2d43b8b97972594d5b77ec26d541cc1a17bdd`
  - macOS Intel：`ffmpeg/ffmpeg-amd64`，SHA-256 `a5be5105d84494c7e0a369a03dd5734ad940318d809f9b0a456b990daec3180a`
  - macOS Apple Silicon：`ffmpeg/ffmpeg-arm64`，SHA-256 `73190b699607d053d009492efeba7926a2280c7802549b08618599516488224e`
- 许可：FFmpeg 为 LGPLv3（同时保留 GPLv3/LGPLv3 补充条款文本），OpenSSL 为 Apache-2.0，Windows 构建还包含 MinGW 运行时声明，另附 IJG 致谢。完整文本见包内 `ffmpeg/LICENSE.txt`。FFmpeg 官方许可说明见 <https://ffmpeg.org/legal.html>。

## 对应源码（Corresponding Source）

本项目是上述自建 ffmpeg 二进制的再分发者，依据 GPLv3 §6(d) 在同一 Release 中提供与各二进制精确对应的完整源码归档：

| 平台 | 源码归档 | SHA-256 |
| --- | --- | --- |
| Windows amd64 | `corresponding-source-windows-amd64.tar.gz` | `c79b14b0464280bea45a6ed5a33b990f870942aaa65a44f269cc332dafbf6b9f` |
| macOS Intel | `corresponding-source-darwin-amd64.tar.gz` | `c42c6b99446554bedd4e1d7853004efec91c81787845c9b823a7a0da2f97eb90` |
| macOS Apple Silicon | `corresponding-source-darwin-arm64.tar.gz` | `554551cb0162cdbc17515f9605d08f33cec159f0f8f07e3b3f0b3331dfa5075e` |

每个源码归档包含：固定 FFmpeg/OpenSSL 原始源码归档（Windows 还包含 w64devkit 工具链源码）、实际 configure 参数、构建与验证脚本、合成样片、构建日志、验证记录、`build-info.json` 及摘要。源码入口与二进制下载位于同一 Release，获取方式相同且不额外收费。

历史版本使用的 Gyan/Martin 第三方构建已不再随包；其源码声明与旧索引仅保留在历史记录中。

## 即时下载的受管 ffmpeg（随包缺失或损坏时）

随包 ffmpeg 缺失、损坏或被隔离时，程序会从固定候选源下载受管 ffmpeg，作为运行兜底。这些二进制不在本发布包内，由客户端直接从上游获取：

- Windows：首选 gyan.dev 9.0.2 essentials，备用 BtbN 2026-09-30 月末 8.1 LGPL。
- macOS：Martin Riedl 9.0.2 的 amd64 / arm64 原始构建。

对应下载地址、SHA-256 与来源说明仍记录在代码和 `build/fetch-ffmpeg.ps1` 等处；其许可证由上游提供，本项目不再分发这些二进制，也不把本项目的 GPLv3/LGPLv3 文本误称为其许可证。

## GPLv3 对应源码说明

FFmpeg 以 LGPLv3/GPLv3 授权发布。本项目按聚合方式分发项目程序（MIT + Commons Clause）与自建 ffmpeg（LGPLv3）；ffmpeg 由项目以独立进程、命令行参数调用。接收者可以通过上述同一 Release 的源码归档获取精确对应的 FFmpeg、OpenSSL 及构建材料，也可按 GPLv3 对 ffmpeg 二进制行使相应权利。该源码入口长期随该 Release 保留；不使用实物分发的书面要约替代网络分发要求。

## 项目自身许可（独立标识）

项目 `wx_channel` 自身代码采用 **MIT License + Commons Clause**，完整文本见仓库根目录 `LICENSE`。Commons Clause 仅限制“出售”本项目软件，不改变 MIT 授予的其他权利，也不影响随包 ffmpeg 的 LGPLv3/GPLv3 授权。

## 聚合分发说明

发布包由两部分组成：(1) 项目程序 `wx-mini-video`（MIT + Commons Clause）；(2) 自建 ffmpeg（LGPLv3）。两部分不链接为单一作品，本包按**聚合（aggregation）**分发。因此：

- 项目代码继续按 MIT + Commons Clause 授权，不因包含 ffmpeg 而整体变为 LGPLv3/GPLv3。
- ffmpeg 二进制继续按 LGPLv3/GPLv3 授权，接收者对其享有相应权利，包括获取对应源码。
- 整体压缩包不改变任何组件的许可；再分发时必须同时保留本文件与 `ffmpeg/LICENSE.txt`，不得删除或替换其中的许可与来源信息。
