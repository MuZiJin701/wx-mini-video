# 自带 ffmpeg 完整包需求

状态：2026-10-24 需求澄清中。用户调用 `grill-with-docs`，先确认需求并记录决策；尚未授权按未确认方案修改实现或上传完整包。

## 已确认

- Q1：完整包随附 ffmpeg，用户完整解压后，首次下载 m3u8 无需另行联网获取 ffmpeg，也无需安装工具或填写可执行文件路径。媒体下载本身仍需要网络；保留当前 TUI 和下载操作。

## 待确认

- Q2：首版平台范围，是同时提供 Windows amd64 与 macOS Intel / Apple Silicon，还是先提供其中一个平台。
- Q3：在现有 v0.1.8 Release 新增完整包并保留原包，还是替换附件或另建版本。
- 依据平台与发布方式继续确认处理器分包、随包文件布局、显式路径优先级、缺失/损坏恢复、依赖版本及更新、再分发材料和发布验收。

## 已核实的实现约束

- [ADR 0001](adr/0001-managed-ffmpeg.md) 当前规定发布包不内置 ffmpeg，首次需要时获取固定版本；新增完整包需要明确调整其适用范围。
- 当前 Windows 包只允许 README、程序和配置三个文件，Mac 包只允许说明、配置、Info.plist 与通用可执行文件。不能只把 ffmpeg 临时塞进原包而跳过包内容检查与文档更新。
- 当前受管 ffmpeg 位于下载目录，复用需要同时匹配固定来源摘要标记和可执行文件摘要；仅附加任意同名 ffmpeg 文件不会被当作已验证的受管版本。
- macOS 当前按处理器获取 ffmpeg；同一个包如何兼容两种处理器，需随平台范围确认。

## 外部来源核查

2026-10-24 轻量 HTTP 检查：代码中的 Windows BtbN 8.1.2 固定 ZIP 返回 404；Mac Martin Riedl 9.0.2 的 amd64、arm64 ZIP 均返回 200。该结果仅表示检查时的地址状态，尚未为本需求下载或执行新样本；不能继续假设旧 Windows 地址可用于构建完整包。[BtbN 保留策略](https://github.com/BtbN/FFmpeg-Builds#release-retention-policy)说明日构建只保留近 14 份，月末构建保留两年。

Mac 当前两份构建的发布者记录均包含 `--enable-gpl --enable-version3`，需按 GPLv3 构建处理。记录见 [Intel 配置](https://ffmpeg.martin-riedl.de/download/macos/amd64/1789931006_9.0.2/versions.txt)与 [Apple Silicon 配置](https://ffmpeg.martin-riedl.de/download/macos/arm64/1789931890_9.0.2/versions.txt)。本地已有原始 ZIP 均只含 ffmpeg，不带许可证或源码；与这两份二进制精确对应的完整源码、依赖和构建材料尚未定位。

依据 [FFmpeg 官方法律说明](https://raw.githubusercontent.com/FFmpeg/web/master/src/legal)及 [GPLv3 第 5、6 节](https://raw.githubusercontent.com/FFmpeg/FFmpeg/master/COPYING.GPLv3)，需按具体构建提供对应许可及源码获取材料，不能用构建脚本仓库的许可证代替产物许可证，也不能仅附一个项目主页链接就认定已落实对应源码。项目本身的 MIT + Commons Clause 与随附 ffmpeg 的许可必须分别标识；仅因放入同一个压缩包，不能直接推定整个项目需要改许可证，亦不能据独立进程调用就宣称所有分发问题已解决。选择具体构建与公开上传前仍需补齐再分发材料。
