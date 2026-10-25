# 第三方组件与许可声明

本文件说明 `wx-mini-video` 发布包内第三方组件的来源、许可与获取对应源码的方式。项目自身代码的许可（MIT + Commons Clause，见 `LICENSE`）与本文件所列第三方许可分别标识；把第三方程序放进同一个压缩包不改变任何一方的许可，也不表示第三方授权适用于项目代码。

## 随包 ffmpeg（Windows 完整包）

- 组件：FFmpeg 9.0.2（essentials 构建，仅含 `ffmpeg.exe`）
- 来源：gyan.dev 官方发布 `ffmpeg-9.0.2-essentials_build.zip`
  - 下载地址：<https://www.gyan.dev/ffmpeg/builds/packages/ffmpeg-9.0.2-essentials_build.zip>
  - 压缩包 SHA-256：`60f467265b1e312373dbcd92200c2618a74850f98d3d078e94296bb3fa2047ba`
  - 包内文件：`ffmpeg\ffmpeg.exe`（105423872 字节，SHA-256 由构建脚本从上述压缩包解出后固定）
- 对应上游源码版本：FFmpeg commit [`946fcce07b`](https://github.com/FFmpeg/FFmpeg/commit/946fcce07b)
- 构建信息：gcc 16.2.0（MSYS2 构建），64 位静态链接；configure 含 `--enable-gpl --enable-version3 --enable-static`
- 许可：GPLv3（gyan 构建自身声明 `License: GPL v3`）。包内 `ffmpeg\LICENSE.txt` 为 GPLv3 全文，内容与 GNU 发布的 `gpl-3.0.txt` 一致（SHA-256 `8ceb4b9ee5adedde47b31e975c1d90c73ad27b6b165a1dcd80c7c545eb65b903`）。

## 随包 ffmpeg（macOS 通用完整包）

- 组件：FFmpeg 9.0.2，Martin Riedl 提供的 macOS 原始构建，amd64 与 arm64 各一份，未用 `lipo` 合并，以便逐字节核对发布者原始构建。
- Intel（`ffmpeg/ffmpeg-amd64`）：
  - 下载地址：<https://ffmpeg.martin-riedl.de/download/macos/amd64/1789931006_9.0.2/ffmpeg.zip>
  - ZIP SHA-256：`7c6b4125b191cbf773832dc51f424cf2b6bb7da43007d1e066f95909e47cacd4`
- Apple Silicon（`ffmpeg/ffmpeg-arm64`）：
  - 下载地址：<https://ffmpeg.martin-riedl.de/download/macos/arm64/1789931890_9.0.2/ffmpeg.zip>
  - ZIP SHA-256：`c8ed4c4e6978a03c485edbfe4e0a5dc2380f8a30bba5150531b31b094492d924`
- 构建信息：两份构建均声明 `--enable-gpl --enable-version3`（发布者 `versions.txt` 记录）。
- 许可：GPLv3。包内 `ffmpeg/LICENSE.txt` 为 GPLv3 全文。
- 两个二进制放在 `.app` 之外，签名/公证只覆盖 `wx-mini-video.app`；用户升级时须保留整个文件夹。

## 即时下载的受管 ffmpeg（随包缺失或损坏时）

随包 ffmpeg 缺失、损坏或被隔离时，程序按固定候选源下载并校验 ffmpeg：

- Windows：首选 gyan.dev 9.0.2 essentials（同上），备用 [BtbN/FFmpeg-Builds 月末构建 `autobuild-2026-09-30-13-08`](https://github.com/BtbN/FFmpeg-Builds/releases/download/autobuild-2026-09-30-13-08/ffmpeg-n8.1.3-9-g29e619e767-win64-lgpl-8.1.zip)（8.1，LGPL，ZIP SHA-256 `4a7642b2264c03e8a0ce8a3825b933ee5580656f45695a086fe7e294045ffc0a`）。
- macOS：沿用上表 Martin Riedl 9.0.2 的 amd64 / arm64 ZIP。

这些下载的二进制不在发布包内，由客户端在用户机器上按 URL 与 SHA-256 固定校验后保存；对应的 GPLv3/LGPL 许可与本节、`LICENSE` 及包内许可文本一致。LGPL 来源的 BtbN 构建按 LGPL 条款提供，GPLv3 全文随包提供不影响其独立许可标识。

## 对应源码与公开发布状态

**当前随包构建的完整对应源码材料尚未落实，完整包不得公开发布。** 本文件中的 FFmpeg commit、构建参数、发布者主页与许可证全文只是已核实的来源信息，不是已提供完整 Corresponding Source 的声明。

GitHub Release 属于网络分发，应落实 [GPLv3 §6(d)](https://www.gnu.org/licenses/gpl-3.0.html#section6) 要求的等效对应源码获取途径，源码可以另行下载，但须在二进制下载入口旁清楚标明并持续保证可获取。§6(b) 的实物分发书面要约不作为本项目网络分发的替代方案；“提交 Issue 后再向上游索取”不表示已经满足该要求。

公开发布前须取得并核实每份二进制精确对应的 FFmpeg 源码、修改/补丁、所需外部库源码及构建/安装脚本，不能只提供 FFmpeg 主仓库快照或构建脚本仓库。人工核对完整性与对应关系后，在 `build/ffmpeg-corresponding-sources.json` 填写每个构建的源码归档 HTTPS 地址、SHA-256 和核对状态；运行 `python3 build/verify-release-sources.py`，校验二进制来源摘要、源码入口与实际下载摘要。该工具不能自动证明源码完整或代替人工核对，未经核实不得把 `reviewed` 改为 `true`。

完成后，应将源码索引与对应归档和发布说明一起提供下载。源码材料未就绪时，开发构建和自动化测试可以继续，但不能将生成的完整包当作可公开发布的产物。

BtbN 的 LGPL 备用构建由客户端直接从上游下载；这是与随包二进制分发不同的路径，其许可证由上游提供，不把本项目的 GPLv3 全文误称为该 LGPL 构建的许可证，也不承诺持有尚未归档的对应源码。

本项目不声称这些 ffmpeg 二进制由本项目编译，也不把第三方构建脚本仓库的许可当作产物许可。

## 项目自身许可（独立标识）

项目 `wx_channel` 自身代码采用 **MIT License + Commons Clause**，完整文本见仓库根目录 `LICENSE`。Commons Clause 仅限制“出售”本项目软件，不改变 MIT 授予的其他权利，也不影响随包 ffmpeg 的 GPLv3 授权。

## 聚合分发说明

发布包由两部分组成：(1) 项目程序 `wx-mini-video`（MIT + Commons Clause）；(2) 第三方独立程序 ffmpeg（GPLv3）。ffmpeg 由项目在运行时以独立进程、命令行参数调用，两部分不链接为单一作品，本包按**聚合（aggregation）**分发。因此：

- 项目代码继续按 MIT + Commons Clause 授权，不因包含 ffmpeg 而整体变为 GPLv3。
- ffmpeg 二进制继续按 GPLv3 授权，接收者对其享有 GPLv3 权利，包括获取对应源码。
- 整体压缩包不改变任何组件的许可；再分发时必须同时保留本文件与 `ffmpeg/LICENSE.txt`，不得删除或替换其中的许可与来源信息。

FFmpeg 官方许可说明见 <https://ffmpeg.org/legal.html>。
