# macOS 测试版

适用 macOS 13 及以上，Apple Silicon 与 Intel 共用一个 ZIP。保留 Windows 版的单主界面、目标 AppID 严格归属、来源未确认媒体、图片、直链视频、直链音频、m3u8 与 HTTP-FLV 直播流录制。

[下载 v0.1.9 Mac 通用 ZIP](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.9/wx-mini-video-macos-universal-unsigned.zip)。Mac 与 Windows 包在同一 [v0.1.9 Release](https://github.com/MuZiJin701/wx-mini-video/releases/tag/v0.1.9) 并列提供；应用内版本号见 `main.go`（源码 v0.2.0，发布前 Release 为 v0.1.9），附件构建提交与 SHA-256 见发布说明。[中文 README](https://github.com/MuZiJin701/wx-mini-video/blob/main/README.md)同样适用。

`wx-mini-video-macos-universal-unsigned.zip` 是本地构建的未签名测试包，仅供验证；正式发布须完成 Developer ID 签名、公证与双架构真机验收，并使用不含 `unsigned` 的文件名。

## 开始使用

1. 完整解压 ZIP，保留 `wx-mini-video.app`、`wx-mini-video.yaml`、`README.md`、`ffmpeg/`（含双架构 ffmpeg 与 GPLv3 许可）和 `THIRD-PARTY-NOTICES.md`；只移动 `.app` 会丢失随包 ffmpeg。
2. 双击 `wx-mini-video.app`。被系统阻止时按“隐私与安全性”针对该应用允许；若提示找不到配置，按引导将应用拖出再拖回解压目录后重试。不要把配置或下载放入 `.app` 内部。
3. 按系统提示允许控制 Terminal。程序打开即自动启动代理并持续发现小程序，界面显示启动状态；在统一目标选择区输入 AppID，或从持续更新的“已发现的小程序”列表用 `↑/↓` 选择后按 `Enter`（鼠标点击发现项直接确认）。配置中的 `target.appID` 只预填输入框，不会自动确认目标。
4. 首次启动代理时允许将本机独立根证书加入当前用户默认钥匙串并信任；启用/恢复系统代理会请求管理员授权，TUI 本身以普通用户运行。
5. 代理运行后重新加载图片或播放音视频：媒体视图默认显示“全部”，`1`–`6` 切换全部 / 图片 / 视频 / 音频 / 直播 / 分段媒体；`↑/↓` 选中，`d`/`Enter` 下载，`o` 打开当前小程序目录；来源未确认媒体以 `?` 标记并入分类。品牌区右侧“外观 ▾”可切换跟随系统/浅色/深色（保存在 `downloads/ui.json`）。HTTP-FLV 直播流标记为“直播流”，持续录制并按 `x` 停止保存为 `.flv`；异常断流时也会自动保存已录内容并提示原因，重试不会覆盖未保存的录制；确认退出时若保存失败会取消退出并保留错误。HLS（m3u8）持续直播录制本轮不支持，m3u8 只按“分段媒体”保存当前播放列表并合并。
6. 按 `q` 或点击页脚“退出 · q”退出并恢复原代理；恢复失败时按 `r` 重试，理解影响后可 `f` 强制退出。输入框聚焦时 `q` 是文本，可用 `Ctrl+C`/`Ctrl+Q` 或页脚按钮退出。

媒体保存在解压文件夹的 `downloads/<目标 AppID>/{图片,视频,音频,直播,分段媒体,其他}`，升级时替换 `.app` 并保留配置与 `downloads/`；旧版平铺文件与旧 `m3u8/` 目录不迁移、不自动删除，程序启动不会清空 `downloads/`。根证书与私钥在 `~/Library/Application Support/wx-mini-video/certificates/root-ca.pem`，不随文件夹移动或分发。

## 网络与证书排障

- 已有 HTTP/HTTPS 代理、PAC、SOCKS、认证代理或活动 VPN 默认路由冲突时会提示处理；程序不会覆盖需认证的配置。
- 运行中活动接口变化时停止嗅探并恢复原代理，按 `r` 重启；同一接口内的 Wi-Fi 热点切换不保证被识别。
- 没有媒体时确认目标后重新加载；来源不匹配的媒体以 `?` 显示，手动输入 AppID 不会改变归属。
- 信任未建立时，可在钥匙串访问中导入 `downloads/WxMiniVideoRoot.cer` 并核对指纹；不要导入或分享含私钥的 `root-ca.pem`。正常退出保留证书，停用时按实际指纹删除；旧共享证书指纹为 `D70CD039051F77C30673B8209FC15EFA650ED52C`。
- 异常终止后重新运行，等界面显示“代理运行中”后按 `q` 清理；仍无法恢复时在系统设置中只移除指向本工具的代理地址。

## 验证反馈

项目提供 [macOS 原生 CI](https://github.com/MuZiJin701/wx-mini-video/actions/workflows/macos.yml)，在 Intel 与 Apple Silicon 上验证原生测试、ffmpeg 校验与 m3u8 合并、便携包和终端内启动；CI 不操作真实系统代理、不安装证书，也不替代 Finder 双击、系统授权和真实微信验收。

请在两种处理器上分别记录机型、macOS/微信版本，以及首次启动、授权、图片/直链视频/音频/m3u8/HTTP-FLV 录制、网络切换与退出恢复结果；确认完整解压、移动目录、中文与空格路径下配置和 `downloads/` 仍在便携目录。反馈错误摘要即可，不要发送私钥、Cookie、鉴权头或带签名的完整 URL。

## 构建与正式签名

```text
go run ./build/macos
```

在仓库根目录运行，Windows 与 macOS 均可构建未签名通用测试包；ZIP 保留可执行权限，包含应用（含 `Contents/Resources/logo.icns` 应用图标）、配置、说明、`.app` 外的双架构 `ffmpeg/`、GPLv3 全文与第三方声明，不包含下载数据或私钥。发布前运行 `python3 build/verify-release-sources.py`；无 Mac 环境时可运行 `python3 build/verify-macos.py --package-only` 检查包内容、权限、应用图标与 plist 字段，这不等于原生验收。

正式发布需在具备 Developer ID 与 `notarytool` 凭据的 Mac 上运行：

```sh
export MACOS_SIGN_IDENTITY='Developer ID Application: YOUR NAME (TEAMID)'
export MACOS_NOTARY_PROFILE='your-existing-keychain-profile'
bash build/sign-macos.sh
```

签名与公证只覆盖 `.app`，包外 ffmpeg 未签名，Gatekeeper 首次执行行为仍需真机验证；执行发布流程须有明确授权。完整说明见 [项目主页](https://github.com/MuZiJin701/wx-mini-video)。
