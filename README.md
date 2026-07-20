# 微信小程序媒体下载器

一个 Windows TUI 工具，用于嗅探并下载 PC 微信小程序加载的图片和视频。

支持图片：`jpg`、`png`、`webp`、`gif`、`bmp`、`avif`  
支持视频：`mp4`、`webm`、`mov`、`m3u8`

## 下载

从 GitHub Releases 下载，不要从源码仓库的 `dist/` 目录取包：

- `wx-mini-video-windows-amd64.zip`

解压后会看到：

- `wx-mini-video.exe`
- `wx-mini-video.yaml`
- `README.md`

基础包不内置 ffmpeg。首次处理 `m3u8` 视频时，程序会自动下载便携版 `ffmpeg.exe` 到下载目录。

## 使用

1. 右键 `wx-mini-video.exe`。
2. 选择“以管理员身份运行”。
3. 输入小程序 AppID，名称可不填。
4. 在 PC 微信中打开对应小程序。
5. 建议先按 `c` 清空列表，再打开目标图片或播放目标视频。
6. 选中资源后按 `d` 或 `Enter` 下载。
7. 需要核对资源时按 `i` 查看完整 URL、来源 URL、请求头摘要和本地缓存路径。
8. 按 `q` 退出，程序会恢复启动前的系统代理。

资源列表默认显示“视频”分类，可用 `Tab` 或数字键切换到“全部 / 图片 / 视频 / m3u8”。

下载文件默认保存在程序旁边的 `downloads` 目录。

直链下载会先写入同名 `.part` 文件，网络中断后再次下载会尝试从已有部分继续。成功完成的下载会追加记录到 `downloads/history.jsonl`。

## 快捷键

| 按键 | 功能 |
| --- | --- |
| `↑/↓`、`k/j` | 选择资源 |
| `Tab`、`Shift+Tab` | 切换分类 |
| `1/2/3/4` | 切换到全部、图片、视频、m3u8 |
| `d`、`Enter` | 下载选中资源 |
| `i` | 显示或隐藏选中资源详情 |
| `c` | 清空资源列表 |
| `r` | 刷新列表 |
| `o` | 打开下载目录 |
| `l` | 显示或隐藏日志 |
| `PgUp/PgDn` | 翻阅日志 |
| `q` | 退出并恢复系统代理 |

## 重要说明

- 本工具通过本地 HTTPS 代理识别图片和视频资源。
- 首次运行需要安装本地根证书，Windows 可能弹出权限确认。
- AppID 目前用作本次会话标签，不保证自动判断每条请求真实属于哪个小程序。
- 小程序一次可能加载很多图片或视频，建议先按 `c` 清空，再操作目标内容。
- 如果程序异常退出后网络异常，重新以管理员身份运行本工具，再按 `q` 正常退出即可恢复代理。

## 常见问题

### 打不开小程序或提示网络错误

可能是系统代理没有恢复。重新运行 `wx-mini-video.exe`，进入界面后按 `q` 退出。

### 证书安装失败

请确认已“以管理员身份运行”。如果仍失败，可以手动安装下载目录里的 `WxMiniVideoRoot.cer` 到“受信任的根证书颁发机构”。

代理启动后，也可以访问：

```text
http://127.0.0.1:2023/cert
```

### m3u8 下载很慢

长视频通常包含大量分片，耗时会比普通 mp4 更久。程序会并发下载分片并显示进度。如果失败较多，可以把配置里的 `segmentConcurrency` 从 `6` 改成 `4`。

### 没有识别到目标资源

先按 `c` 清空列表，再重新打开目标图片或播放目标视频。部分小程序资源可能不是常见图片、视频或 m3u8 格式，暂时无法识别。

## 配置

配置文件是 `wx-mini-video.yaml`。常用项：

```yaml
download:
  dir: ""
  ffmpeg: "ffmpeg"
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

一般不需要修改配置。

## 开发构建

```powershell
go test -count=1 ./...
go vet ./...
build\build.bat windows
```

构建产物：

- `dist\wx-mini-video-windows-amd64\`
- `dist\wx-mini-video-windows-amd64.zip`

`dist\` 是本机构建目录，不提交到 Git。对外分发时，把 zip 上传到 GitHub Releases。
构建脚本会自动运行 `build\verify-package.ps1`，确保 zip 只包含 `README.md`、`wx-mini-video.exe` 和 `wx-mini-video.yaml`。

项目维护文档：

- `AGENTS.md`：项目规则、边界和证据入口
- `wx-mini-video-knowledge.md`：架构、配置、分发和排障知识
- `docs/superpowers/plans/`：当前优化和修改计划
- `docs/archive/`：历史文档

## 免责声明

本项目仅用于技术交流和学习研究。请遵守法律法规，不要用于任何非法用途。
