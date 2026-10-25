# 从官方源码自行构建 ffmpeg，归档完整构建材料

状态：2026-10-25 已实施；Windows amd64 本地构建、macOS Intel 与 Apple Silicon Actions 构建均通过十二项实际合并验证，对应源码归档已随 v0.1.9 Release 提供。修订 [ADR 0007](0007-bundled-ffmpeg.md) 的第三方二进制选型，完整包与自动下载兜底体验保持不变。

## 原因

v0.1.9 之前使用的 Gyan 与 Martin 预编译二进制，其完整对应源码、外部库版本和构建材料未齐备，旧源码索引保持未核实。单独附上 FFmpeg 主仓库提交或改用一个标注 LGPL 的预编译包，都不能证明材料与实际二进制对应。

项目调用 ffmpeg 使用 `-c copy`，不做视频重新编码，因此无需为了保存 H.264/HEVC 媒体而链接 x264/x265 外部编码库。FFmpeg [官方说明](https://ffmpeg.org/ffmpeg.html#Streamcopy)确认 streamcopy 不进行重新编解码。

## 决策

- 使用 FFmpeg 9.0.2 官方源码和 OpenSSL 3.5.9，固定来源及摘要。FFmpeg 签名已按[官方发布验证说明](https://ffmpeg.org/download.html#release-verification)核对。
- 不极限裁剪内部媒体格式。保留 HLS、TS/fMP4、H.264/HEVC、AAC、AES-128、HTTP(S)、鉴权和重定向能力；去掉与项目无关的外部库，使用 OpenSSL 作为必要的 TLS 依赖。
- 三平台均使用固定 OpenSSL，避免 Mac 仅用系统 Secure Transport 而缺少 TLS 1.3；该限制见 [Apple 文档](https://developer.apple.com/documentation/security/secure-transport)。FFmpeg 采用 LGPLv3 构建，OpenSSL 维持 Apache-2.0；项目通过独立进程调用，不链接这些库。
- Windows amd64 使用固定便携 w64devkit；Mac Intel 与 ARM 分别用原生环境构建。源码、配置、许可、工具链信息、验证记录和二进制摘要一起归档。Windows 额外归档固定工具链完整源码，避免忽略静态运行时材料；MSYS2、Perl 与 Make 包只作为固定构建工具，不写入系统。
- 构建脚本作为对应源码材料的一部分，独立采用无 Commons Clause 的 MIT 许可；不改变主程序许可。
- 原生构建和十二项实际合并验证全部通过后，才更新应用打包和受管下载的固定来源。保留现有发布校验的阻断，不能仅凭新增构建脚本将任何架构的 reviewed 改为 true。
- v0.1.9 已完成三架构原生构建、十二项实际合并验证和对应源码归档，随 Release 同页提供；Actions 临时附件不再作为唯一源码入口。

构建、材料布局与验收入口见 [源码构建说明](../../build/ffmpeg/README.md)。

## 取舍

自行承担 FFmpeg/OpenSSL 安全更新及三架构编译维护，但能够直接确认输入、配置与输出之间的对应关系。源码和二进制须按版本长期保留，不能用滚动 latest 地址替代；每次升级重新验收，不必每次修改 Go 程序都重新编译 ffmpeg。

当前本机没有 Mac 环境，Mac 原生编译与验证须通过 Actions 执行，macOS 13 最低版本仍需真机验收。签名、公证和微信验收要求不因 ffmpeg 迁移而取消。
