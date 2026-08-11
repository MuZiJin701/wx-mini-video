# 项目上下文

## 目标

`wx-mini-video` 帮助 Windows 用户在 PC 微信中识别并保存小程序加载的媒体资源。它是本地运行的 Go TUI，不提供 Web 服务或容器化部署。

## 边界

- 支持常见图片、直链视频和普通 VOD/回放 m3u8。
- AppID 只作为当前会话的归类标签，不是请求归属证明。
- Windows 是验收与分发平台；发布包通过 GitHub Releases 提供。
- 不恢复旧视频号、启明星专用流程、Web 服务、Docker、启动脚本或内置 ffmpeg 分发。

## 术语

| 术语 | 含义 |
| --- | --- |
| 候选资源 | 代理从响应中识别、可在 TUI 中查看或下载的媒体项。 |
| 直链视频 | 可直接写入本地文件的视频 URL，与 m3u8 播放列表相对。 |
| m3u8 | 下载器预取分片后交由 ffmpeg 合并的播放列表。 |
| 会话 AppID | 用户启动时输入、用于隔离候选资源的标签。 |

## 不变量

- 本地代理可能改动 Windows 系统代理；正常退出必须恢复原始状态。
- 下载应保留请求所需的来源信息和请求头，避免只凭 URL 重放失败。
- 分发 zip 只包含 `README.md`、`wx-mini-video.exe`、`wx-mini-video.yaml`。

实现、配置与排障细节见 `wx-mini-video-knowledge.md`；长期架构决策见 `docs/adr/`。
