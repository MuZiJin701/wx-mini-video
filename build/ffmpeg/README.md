# 从固定源码构建 ffmpeg

本目录的构建、验证脚本和合成样本独立采用 [MIT](LICENSE) 授权，不附加项目主程序的 Commons Clause。FFmpeg 和 OpenSSL 源码保持各自许可，不适用本目录 MIT 许可。

当前是替换第三方预编译包之后的构建与验收入口。随包版本和受管下载兜底已切换/保留；Windows amd64 本地、macOS amd64/arm64 Actions 均完成构建并通过十二项实际合并验证，对应源码归档随 v0.1.9 Release 提供。决策见 [ADR 0010](../../docs/adr/0010-source-built-ffmpeg.md)。

## 固定输入

所有 URL 和 SHA-256 记录在 `sources.json`：

- FFmpeg 9.0.2 官方发布源码，已核对官方发布密钥 `FCF986EA15E6E293A5644F10B4322F04D67658D8` 的签名。
- OpenSSL 3.5.9，SHA-256 与 OpenSSL 官方下载页指向的发布摘要一致。
- Windows 编译器为便携 w64devkit 2.10.1，二进制及完整工具链源码均固定摘要；固定 MSYS2 20260927、MSYS Perl 5.42.3-1 与 MSYS Make 4.4.1-2 提供完整、使用 Unix 路径的构建环境。它们只解压到 `dist/`，不修改系统 PATH、不安装全局软件。已有 7-Zip 用于解包，构建日志记录编译器和 Perl 版本。
- macOS 使用当前原生 Xcode Command Line Tools、系统 sh/perl/make，编译目标最低版本设为 macOS 13.0；须分别在 Intel 和 Apple Silicon 环境运行。

需要 Python 3.12 或更高版本。构建保留 FFmpeg 默认内部编解码器、封装和协议支持，只关闭外部库自动发现、GPL/nonfree、设备捕获、文档和额外命令行工具。唯一显式启用的外部功能库是静态 OpenSSL；启用 LGPLv3，不加入 x264/x265 编码库。关闭汇编以减少工具依赖，性能是否足够以真实媒体验证为准。Windows 上 FFmpeg 由 MSYS Make 驱动，因为 configure 生成的是 MSYS 风格 `/d/...` 源路径；OpenSSL 使用 w64devkit 原生 Make。

## 本地及 CI 命令

在项目根目录执行与主机一致的一条命令：

```text
python -B build/ffmpeg/build.py windows-amd64
python3 -B build/ffmpeg/build.py darwin-amd64
python3 -B build/ffmpeg/build.py darwin-arm64
```

可用 `--jobs 4` 指定并行编译数。构建失败会保留独立工作目录和日志，不清理用户下载目录、不覆盖现有应用包。

`.github/workflows/ffmpeg.yml` 只允许手动运行，使用 Windows、Mac Intel、Mac ARM 原生环境；只上传构建及源码附件，不创建 Release、不切换默认来源、不修改系统证书和代理。

产物位于 `dist/ffmpeg-selfbuilt/<平台>/`：

- `ffmpeg.zip`：独立二进制和合并的 LGPLv3/GPLv3、OpenSSL Apache-2.0 许可及 IJG 致谢。
- `corresponding-source.tar.gz`：原始 FFmpeg/OpenSSL 源码归档、Windows 工具链源码、本目录构建和测试材料、实际配置文件、构建及验证日志。
- `build-info.json`：输入摘要、实际 configure 参数、输出摘要及验证记录；三架构产物已验证，源码索引已随 v0.1.9 Release 填写并核对。

源归档保留原始、校验过的上游源码。Windows 对 OpenSSL 的静态库归档规则做一个构建补丁：每次传给 ar 的对象文件由 500 个降到 100 个，避免长参数超过 Windows 命令行上限；不修改密码学实现，实际补丁保存在 `records/openssl-windows-ar.patch`，构建脚本会自动应用。Windows 许可文本还包含 w64devkit 提供的 MinGW 运行时声明。

重新构建时，将 `recipe/` 放到一个空工作区的 `build/ffmpeg/`，将 `sources/` 下的归档复制到该工作区的 `dist/ffmpeg-build-cache/`，再运行同一命令。仍需对应操作系统的编译环境；Windows 工具链的固定二进制若无缓存，会从固定 URL 下载。构建日志记录编译器版本，不承诺跨工具链得到逐字节相同的二进制。

Actions 附件只有 30 天保留期，不能充当长期公开源码下载入口。正式切换前须把已验收的二进制、对应源码及记录一起归档到稳定发布位置，并把入口、摘要和人工核对状态写回发布索引；现有 `build/verify-release-sources.py` 的阻断继续保留。

## 验证

```text
python -B -m unittest discover -s build/ffmpeg -p "test_*.py"
```

构建完成后自动运行 `verify.py`；CI 还会运行 `verify_artifacts.py` 校验产物、`build-info.json` 和对应源码归档的一致性。单独复验：

```text
python -B build/ffmpeg/verify_artifacts.py dist/ffmpeg-selfbuilt/windows-amd64
```

`verify_artifacts.py` 检查 `ffmpeg.zip` 条目与许可、二进制摘要、十二项验证记录、对应源码归档条目和三个摘要是否与 `build-info.json` 一致；不通过时 CI 不会上传附件。

`verify.py` 检查：

- 检查版本、许可、编译选项及 file/http/https/tcp/tls/crypto/data 协议。
- 三组数据：H.264 + AAC 的 TS、HEVC + AAC 的 fMP4，以及 AES-128 加密 H.264 HLS。
- 每组分别测试本地播放列表、HTTP、仅允许 TLS 1.2 和仅允许 TLS 1.3 的本地 HTTPS 服务，共十二项实际合并。
- HTTP(S) 服务要求测试 Cookie/Authorization；通过重定向读取播放列表，密钥请求必须保留 query。
- 对每个输出完整解码并检查 H.264/HEVC 和 AAC 音视频流，避免只以“命令退出成功”作为依据。

测试只创建临时 HTTPS 证书，不加入系统信任库；不依赖微信、外网媒体或用户 Cookie。两个 `.ts` 样本是 64×64 黑帧与 440 Hz 合成音，时长两秒，无用户数据；分别由现有开发用 ffmpeg 的 libx264/libx265 和 AAC 编码生成，只作为输入数据，不分发这些编码器。生成时使用 `-g 5 -preset ultrafast -b:a 24k`，HEVC 另用 `pools=1:frame-threads=1`。

该验证不能代替真实微信下载，也不能代替 macOS 13 真机、签名和公证验收。
