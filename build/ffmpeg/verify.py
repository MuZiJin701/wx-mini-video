"""用合成样本验证自建 ffmpeg 的实际合并、鉴权及 TLS 能力，不改系统信任。"""

import argparse
import functools
import http.server
import json
import os
import pathlib
import ssl
import subprocess
import sys
import tempfile
import threading
import urllib.parse

HERE = pathlib.Path(__file__).resolve().parent


def execute(binary, args, cwd=None, trust_cert=None):
    env = dict(os.environ, OPENSSL_CONF=os.devnull)
    # 不借用构建工具链的 DLL 或 Homebrew 环境，验证二进制能独立运行。
    if os.name == "nt":
        system_root = pathlib.Path(os.environ["SystemRoot"])
        env["PATH"] = os.pathsep.join((str(system_root / "System32"), str(system_root)))
    else:
        env["PATH"] = "/usr/bin:/bin:/usr/sbin:/sbin"
    env.pop("DYLD_LIBRARY_PATH", None)
    env.pop("DYLD_FALLBACK_LIBRARY_PATH", None)
    if trust_cert is not None:
        # 让 OpenSSL 默认验证路径信任本次临时证书，同时不修改系统信任库。
        env["SSL_CERT_FILE"] = str(trust_cert)
    result = subprocess.run([str(binary)] + list(map(str, args)), capture_output=True,
                            text=True, errors="replace", timeout=60, env=env, cwd=cwd)
    if result.returncode:
        raise RuntimeError(f"ffmpeg/openssl 失败（{result.returncode}）: {result.stderr[-6000:]}")
    return result.stdout + result.stderr


class FixtureHandler(http.server.SimpleHTTPRequestHandler):
    failures = None
    tls_versions = None

    def log_message(self, *_):
        pass

    def do_GET(self):
        if (self.headers.get("Authorization") != "Bearer ffmpeg-fixture"
                or self.headers.get("Cookie") != "ffmpeg_fixture=1"):
            self.failures.append("missing fixture credentials")
            self.send_error(403)
            return
        parsed = urllib.parse.urlsplit(self.path)
        if parsed.path.endswith("key.bin") and urllib.parse.parse_qs(parsed.query).get("token") != ["fixture"]:
            self.failures.append("lost key query")
            self.send_error(403)
            return
        if isinstance(self.connection, ssl.SSLSocket):
            self.tls_versions.add(self.connection.version())
        if parsed.path.endswith("redirect.m3u8"):
            self.send_response(302)
            self.send_header("Location", "playlist.m3u8?token=fixture")
            self.end_headers()
            return
        super().do_GET()


def verify(binary, openssl, directory):
    if sys.platform == "darwin":
        linked = subprocess.check_output(["/usr/bin/otool", "-L", str(binary)], text=True)
        for line in linked.splitlines()[1:]:
            library = line.strip().split(" ", 1)[0]
            if not library.startswith(("/usr/lib/", "/System/Library/")):
                raise ValueError("不能依赖系统外的动态库: " + library)
    banner = execute(binary, ["-version"])
    if not banner.startswith("ffmpeg version 9.0.2"):
        raise ValueError("ffmpeg 版本不符")
    for flag in ("--disable-autodetect", "--disable-gpl", "--disable-nonfree", "--enable-version3", "--enable-openssl"):
        if flag not in banner:
            raise ValueError("ffmpeg 配置缺少 " + flag)
    if "GNU Lesser General Public License" not in execute(binary, ["-L"]):
        raise ValueError("构建未声明 LGPL")
    protocols = execute(binary, ["-hide_banner", "-protocols"]).split()
    for protocol in ("file", "http", "https", "tcp", "tls", "crypto", "data"):
        if protocol not in protocols:
            raise ValueError("缺少协议 " + protocol)
    key = directory / "tls-key.pem"
    cert = directory / "tls-cert.pem"
    execute(openssl, ["req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
                      "-subj", "/CN=localhost", "-addext", "subjectAltName=DNS:localhost,IP:127.0.0.1",
                      "-keyout", key, "-out", cert])
    cases = (("h264-ts", "h264", "mpegts", False),
             ("hevc-fmp4", "hevc", "fmp4", False),
             ("h264-aes", "h264", "mpegts", True))
    for name, codec, segment_type, encrypted in cases:
        folder = directory / name
        folder.mkdir()
        args = ["-hide_banner", "-v", "error", "-y", "-i", HERE / "fixtures" / (codec + "-aac.ts"),
                "-c", "copy", "-f", "hls", "-hls_time", "1", "-hls_playlist_type", "vod",
                "-hls_segment_type", segment_type]
        if segment_type == "fmp4":
            # MPEG-TS 的 AAC 是 ADTS 封装，复制进 fMP4 时必须转换。
            # init/分片文件名相对当前工作目录，统一放到播放列表所在目录。
            args += ["-bsf:a", "aac_adtstoasc",
                     "-hls_segment_filename", "seg%03d.m4s",
                     "-hls_fmp4_init_filename", "init.mp4"]
        if encrypted:
            (folder / "key.bin").write_bytes(os.urandom(16))
            key_info = folder / "key-info.txt"
            key_info.write_text("key.bin?token=fixture\n" + str(folder / "key.bin") + "\n")
            args += ["-hls_key_info_file", key_info]
        execute(binary, args + ["playlist.m3u8"], cwd=folder)
        local_playlist = folder / "local.m3u8"
        local_playlist.write_text((folder / "playlist.m3u8").read_text().replace("key.bin?token=fixture", "key.bin"))
    passed = []

    def remux(input_url, name, remote=False, cwd=None, insecure_tls=False, trust_cert=None):
        output = directory / (name + ".mp4")
        args = ["-hide_banner", "-v", "error", "-y"]
        if remote:
            args += ["-headers", "Authorization: Bearer ffmpeg-fixture\r\nCookie: ffmpeg_fixture=1\r\n",
                     "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "5", "-http_persistent", "0"]
            if insecure_tls:
                # 测试证书是临时自签名证书，只关闭本进程的校验，不安装信任。
                args += ["-tls_verify", "0"]
        args += ["-allowed_extensions", "ALL",
                 "-protocol_whitelist", "file,http,https,tcp,tls,crypto,data", "-i", input_url, "-c", "copy", output]
        execute(binary, args, cwd=cwd, trust_cert=trust_cert)
        if not output.is_file() or output.stat().st_size < 1000:
            raise ValueError("未生成有效媒体: " + name)
        description = execute(binary, ["-hide_banner", "-xerror", "-i", output, "-f", "null", "-"])
        codec = "hevc" if "hevc" in name else "h264"
        if "Video: " + codec not in description or "Audio: aac" not in description:
            raise ValueError("输出音视频流不符: " + name)
        passed.append(name)

    for name, *_ in cases:
        remux("local.m3u8", "local-" + name, cwd=directory / name)
    for transport, version in (("http", None), ("tls12", ssl.TLSVersion.TLSv1_2), ("tls13", ssl.TLSVersion.TLSv1_3)):
        failures, versions = [], set()
        handler = functools.partial(FixtureHandler, directory=str(directory))
        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
        server.daemon_threads = True
        # 每个服务独占测试数据；验证流程按顺序运行。
        FixtureHandler.failures, FixtureHandler.tls_versions = failures, versions
        if version is not None:
            context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
            context.minimum_version = context.maximum_version = version
            context.load_cert_chain(cert, key)
            server.socket = context.wrap_socket(server.socket, server_side=True)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            scheme = "http" if version is None else "https"
            for name, *_ in cases:
                url = f"{scheme}://127.0.0.1:{server.server_port}/{name}/redirect.m3u8"
                remux(url, transport + "-" + name, remote=True,
                      insecure_tls=version is not None,
                      trust_cert=cert if version is not None else None)
            if failures:
                raise ValueError(str(failures))
            if version is not None:
                expected = "TLSv1.2" if transport == "tls12" else "TLSv1.3"
                if versions != {expected}:
                    raise ValueError("TLS 握手版本不符: " + str(versions))
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=5)
    return {"passed": passed, "count": len(passed), "tls": ["TLSv1.2", "TLSv1.3"]}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=pathlib.Path)
    parser.add_argument("--openssl", type=pathlib.Path, required=True)
    parser.add_argument("--output", type=pathlib.Path, required=True)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="ffmpeg-media-verify-") as temporary:
        result = verify(args.binary.resolve(), args.openssl.resolve(), pathlib.Path(temporary))
    args.output.write_text(json.dumps(result, indent=2) + "\n")
    print("真实媒体合并验证通过:", result["count"], "项")
