# Vendored echo

本目录是 `github.com/ltaoo/echo` 的 vendored 副本，基线版本 **v0.11.1**（MIT，见 `LICENSE`）。
仓库根目录 `go.mod` 通过 `replace github.com/ltaoo/echo => ./third_party/echo` 使用本目录，`require` 版本保持 `v0.11.1`。

## 本地 patch（相对 v0.11.1）

1. 回收 MITM 子服务资源（`connect.go`、`echo.go`、`cert/manager.go`）
   - 背景：每个被 MITM 的 host 都会创建常驻 `127.0.0.1` listener 与 per-host `http.Server`，v0.11.1/v0.12.1 的 `Echo.Close()` 只关闭 TCP relay/TUN，这些 listener、goroutine 与证书缓存永不回收。
   - `ConnectHandler` 新增 `mitmMu`/`closed`：`getMitmServer` 串行化创建，避免并发 CONNECT 重复建 listener，并在关闭后拒绝新建。
   - 新增 `ConnectHandler.Close()`：关闭并从 map 删除所有 per-host listener 与 `http.Server`，并清理证书缓存；幂等。
   - `MitmServer` 内部保存 `*http.Server`，关闭时 `server.Close()` + `listener.Close()`。
   - 新增 `cert.Manager.ClearCache()`。
   - `Echo.Close()` 先调用 `connectHandler.Close()`，再 `ShutdownTCP()`/`ShutdownTUN()`。
   - 回归测试：`connect_cleanup_test.go`（Close 后 listener 已关闭、map 为空、证书缓存失效、可重复 Close）。

## 上游升级步骤

1. 从 `$(go env GOMODCACHE)/github.com/ltaoo/echo@<version>` 复制源码覆盖本目录；不要复制 `_example/`（依赖未随包分发的 WinDivert 文件）、`docs/`、`.claude/`、`deploy.sh` 与 WinDivert 二进制。
2. 重新应用上述 patch。
3. 验证：

       go build github.com/ltaoo/echo/...
       go test -count=1 github.com/ltaoo/echo/...
