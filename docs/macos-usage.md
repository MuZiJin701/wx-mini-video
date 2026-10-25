# macOS 测试版

适用目标：macOS 13 及以上，Apple Silicon 与 Intel 共用一个 ZIP。保留 Windows 版的终端界面、目标 AppID 严格归属、来源未确认媒体按类型显示、图片、直链视频、直链音频及 m3u8 下载流程。

[下载 v0.1.9 Mac 通用 ZIP](https://github.com/MuZiJin701/wx-mini-video/releases/download/v0.1.9/wx-mini-video-macos-universal-unsigned.zip)。Windows 与 Mac 包在同一 [v0.1.9 Release](https://github.com/MuZiJin701/wx-mini-video/releases/tag/v0.1.9) 并列提供，平台状态分别标注。应用内版本号见 `main.go`（当前 v0.1.9），Mac 附件的具体构建提交与 SHA-256 见发布说明。

文件名为 `wx-mini-video-macos-universal-unsigned.zip` 的包是本地构建的未签名测试包，仅供用户验证，不表示已通过 Developer ID 签名、公证或真实 Mac 微信验收。正式公开发布包使用不含 `unsigned` 的文件名，并须先完成签名、公证和双架构验收。v0.1.9 随包 ffmpeg 已改为固定官方源码自建，对应源码归档在同一 Release 提供。

## 开始使用

1. 完整解压 ZIP，保留 `wx-mini-video.app`、`wx-mini-video.yaml`、`README.md`、`ffmpeg/`（含双架构 ffmpeg 与 GPLv3 许可）和 `THIRD-PARTY-NOTICES.md` 在同一文件夹中。只把 `.app` 拖到别处会丢失随包 ffmpeg。
2. 双击 `wx-mini-video.app`。如 macOS 阻止未签名测试版打开，按系统“隐私与安全性”中针对该应用提供的允许方式处理；此过程不应要求关闭系统安全保护。
3. 若提示找不到配置文件，先核对完整解压；首次运行或移动文件夹后，请在 Finder 中将 `.app` 拖出解压文件夹，再拖回原文件夹，重新双击。macOS 可能临时移动新下载的应用，此步骤及不同系统版本的实际行为仍待验证；不要将配置或下载文件放入 `.app` 内部。
4. 按系统提示允许控制 Terminal，随后在终端界面输入目标 AppID，或按 `Ctrl+D` 发现并选择小程序。进入发现或选择目标后才启动代理。
5. 首次启动代理时允许将本机独立根证书添加到当前用户默认钥匙串并建立信任。启用和恢复系统代理时会请求管理员授权；终端主体保持普通用户运行。
6. 代理运行后重新打开目标图片或播放视频、音频。按 `1` 查看全部；来源未确认媒体以 `?` 标记并入对应分类。`d` 或 `Enter` 下载，`o` 打开下载目录。m3u8 使用随包 `ffmpeg/` 中对应处理器的 ffmpeg，无需联网准备；随包文件缺失或损坏时自动回退受管下载，也可在 `download.ffmpeg` 指定已有可执行文件。
7. 按 `q` 退出，并允许恢复启动前的代理。恢复失败时按 `r` 重试；仅在理解需要自行恢复网络设置后按 `f` 强制退出。

媒体默认保存在解压文件夹内自动创建的 `downloads/`。升级时替换 `.app`，保留原配置和 `downloads/`。本机独立根证书与私钥保存在 `~/Library/Application Support/wx-mini-video/certificates/root-ca.pem`，不随文件夹移动或分发。

## 网络与证书排障

- 启动前发现其他 HTTP/HTTPS 代理、PAC、自动代理发现、SOCKS 或活动 VPN 默认路由时，会提示处理冲突。需认证的代理配置也会拒绝覆盖，避免无法恢复其凭据。
- 运行中活动网络接口变化时停止嗅探并恢复已修改的网络服务，按 `r` 重启代理。切换 Wi-Fi/有线网络及恢复空代理地址的真实系统行为需要验收；同一接口内的 Wi-Fi 热点切换不保证被识别。
- 若没有媒体，确认目标 AppID 后重新加载。来源不匹配的媒体会以 `?` 标记按类型显示，不会成为目标候选；手动输入 AppID 也不会改变这一归属。
- 若信任未建立，可在“钥匙串访问”中导入 `downloads/WxMiniVideoRoot.cer`，核对指纹后设置信任。不要导入或分享 `root-ca.pem`，其中包含私钥。安装后再次按 `r` 重试代理。
- 正常退出保留证书信任。停用时先退出并确认代理恢复，再在“钥匙串访问”中按本工具显示/导出的实际证书指纹核对，只删除对应证书。若曾清理旧共享证书，应核对旧指纹 `D70CD039051F77C30673B8209FC15EFA650ED52C`，不按名称批量删除。
- 异常终止后可重新运行工具进入发现，代理启动后按 `q` 清理同地址残留；仍无法恢复时，在“系统设置 → 网络 → 当前网络服务 → 详细信息 → 代理”中仅移除指向本工具的 HTTP/HTTPS 地址，保留其他设置。

## 验证反馈

项目提供 [macOS 原生 CI](https://github.com/MuZiJin701/wx-mini-video/actions/workflows/macos.yml)，分别验证 Intel 与 Apple Silicon 的原生测试、随包/受管 ffmpeg 校验与 m3u8 合并、便携包及终端内启动。以对应提交的 Actions 结果为准；CI 不操作真实系统代理或安装证书，也不覆盖 Finder 双击、系统授权和真实微信验收。

请分别在两种处理器上记录机型、macOS 版本、微信版本，以及首次双击、Terminal 授权、证书授权、图片/直链视频/直链音频/m3u8、网络切换和退出恢复的结果。验证完整解压后、移动目录后、中文及空格路径中的配置和 `downloads/` 均位于原便携目录。

反馈错误摘要即可；不要发送私钥、Cookie、鉴权头或带签名的完整媒体 URL。签名、公证与编译成功均不代表真实微信嗅探已经通过。

## 维护者构建与正式签名

随包 ffmpeg 已切换为固定 FFmpeg/OpenSSL 官方源码自建版本；对应源码归档与二进制在同一 Release 提供，发布前会运行 `python3 build/verify-release-sources.py` 校验源码索引。未具备 Mac 环境时可在干净构建后运行 `python3 build/verify-macos.py --package-only` 检查 ZIP 内容及权限；这不包含原生运行验收。

在仓库根目录运行，Windows 和 macOS 均可构建未签名通用测试包，不需要新增 Go 依赖：

```text
go run ./build/macos
```

脚本使用当前 Go 工具链构建两种处理器的 Mach-O 并合成通用文件，ZIP 保留可执行权限；打包应用、配置、本说明、`.app` 外的双架构 `ffmpeg/`、GPLv3 全文与第三方声明，不含下载数据或私钥。构建时联网下载并校验 ffmpeg。若发布工具链的最低系统要求升高，应同步检查 macOS 支持范围。

正式发布需在具备 Developer ID 和已配置 `notarytool` 钥匙串凭据的 Mac 上运行：

```sh
export MACOS_SIGN_IDENTITY='Developer ID Application: YOUR NAME (TEAMID)'
export MACOS_NOTARY_PROFILE='your-existing-keychain-profile'
bash build/sign-macos.sh
```

此命令会签名应用并将软件上传 Apple 公证服务，获得票据后附加至应用并重打包；执行前须具备对应发布授权。脚本只接收签名身份名与现有钥匙串配置名，不要求把私钥或密码写入仓库。当前环境没有执行此流程。

签名与公证只覆盖 `.app`；`ffmpeg/` 位于包外，未签名、未公证，Gatekeeper 对其首次执行的行为仍需真机验证。随包 ffmpeg 为 LGPLv3 独立程序（自建，静态 OpenSSL），再分发材料见 `THIRD-PARTY-NOTICES.md`。

完整项目说明与版本记录见 [项目主页](https://github.com/MuZiJin701/wx-mini-video)。
