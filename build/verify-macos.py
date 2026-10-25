"""在 macOS 验证便携包及终端内启动。

TUI 验收使用临时目录中的隔离配置与 HOME：关闭系统代理、跳过根证书安装、
下载目录与证书目录都落在临时目录内，不修改用户的系统代理或证书信任库。
"""

import json
import os
import pathlib
import plistlib
import select
import socket
import struct
import subprocess
import tempfile
import sys
import time
import zipfile


def free_tcp_port():
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def write_isolated_config(directory):
    """写入隔离测试配置，返回 (配置路径, 下载目录, 临时 HOME)。

    启动即发现会自动启动本地代理；必须关闭系统代理与根证书安装，
    并把下载目录、证书目录放到临时目录，避免验收脚本触碰用户系统设置。
    """
    base = pathlib.Path(directory)
    downloads = base / "downloads"
    home = base / "home"
    home.mkdir(parents=True, exist_ok=True)
    config = base / "wx-mini-video-test.yaml"
    config.write_text(
        "download:\n"
        f"  dir: {json.dumps(str(downloads))}\n"
        "capture:\n"
        "  showUnattributed: false\n"
        "proxy:\n"
        "  system: false\n"
        '  hostname: "127.0.0.1"\n'
        f"  port: {free_tcp_port()}\n"
        "  skipInstallRootCert: true\n"
        '  upstreamProxy: ""\n'
        "target:\n"
        '  appID: ""\n'
        "cert:\n"
        '  file: ""\n'
        '  key: ""\n'
        '  name: "Echo"\n',
        encoding="utf-8",
    )
    return config, downloads, home


def startup_diagnostics(output, downloads):
    log = pathlib.Path(downloads) / "wx-mini-video.log"
    text = output.decode("utf-8", errors="replace")[-4000:]
    if log.is_file():
        text += "\n隔离运行日志:\n" + log.read_text(encoding="utf-8", errors="replace")[-4000:]
    return text


def wait_for_tui_exit(process, master, output, timeout=15):
    # Keep draining the PTY while the renderer finishes; waiting alone can fill its buffer.
    deadline = time.monotonic() + timeout
    while process.poll() is None and time.monotonic() < deadline:
        if select.select([master], [], [], 0.2)[0]:
            try:
                chunk = os.read(master, 65536)
            except OSError:
                break
            if not chunk:
                break
            output.extend(chunk)
    return process.wait(timeout=max(0.1, deadline - time.monotonic()))


def check_tui(executable, cwd, config, downloads, home):
    import fcntl
    import pty
    import termios
    master, slave = pty.openpty()
    process = None
    output = bytearray()
    try:
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 30, 120, 0, 0))
        env = dict(
            os.environ,
            TERM="xterm-256color",
            WX_MINI_VIDEO_MAC_TUI="1",
            WX_MINI_VIDEO_CONFIG=str(config),
            HOME=str(home),
        )
        env.pop("WX_MINI_VIDEO_DEBUG", None)
        process = subprocess.Popen(
            [str(executable)], cwd=cwd, env=env,
            stdin=slave, stdout=slave, stderr=slave,
        )
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            if select.select([master], [], [], 0.2)[0]:
                try:
                    chunk = os.read(master, 65536)
                except OSError:
                    break
                if not chunk:
                    break
                output.extend(chunk)
            text = output.decode("utf-8", errors="replace")
            if "代理启动失败" in text:
                raise AssertionError("隔离配置下代理启动失败:\n" + startup_diagnostics(output, downloads))
            if "代理运行中" in text:
                break
            if process.poll() is not None:
                break
        text = output.decode("utf-8", errors="replace")
        assert "AppID" in text, text
        assert "代理运行中" in text, "启动即发现未进入代理运行状态:\n" + startup_diagnostics(output, downloads)
        # 证书必须生成在临时 HOME 内，证明没有触碰用户的证书目录。
        certificates = home / "Library/Application Support/wx-mini-video/certificates"
        assert certificates.is_dir(), f"证书目录未落在隔离 HOME: {certificates}"
        assert downloads.is_dir(), f"隔离下载目录未创建: {downloads}"
        os.write(master, b"\x1b")
        assert wait_for_tui_exit(process, master, output) == 0, startup_diagnostics(output, downloads)
    finally:
        if process is not None and process.poll() is None:
            process.kill()
            process.wait()
        os.close(master)
        os.close(slave)


def assert_thin_mach_o(binary, cpu, label):
    magic, actual_cpu = struct.unpack_from("<II", binary)
    assert magic == 0xFEEDFACF, f"{label} 不是 64 位 Mach-O（可能被改成通用包）"
    assert actual_cpu == cpu, f"{label} cputype = {actual_cpu:#x}，期望 {cpu:#x}"


def main():
    root = pathlib.Path.cwd()
    archive = root / "dist/wx-mini-video-macos-universal-unsigned.zip"
    prefix = "wx-mini-video-macos/"
    executable_name = "wx-mini-video.app/Contents/MacOS/wx-mini-video"
    bundle_icon_bytes = (root / "build/icons/logo.icns").read_bytes()
    ffmpeg_arches = (("amd64", 0x01000007), ("arm64", 0x0100000C))
    selfbuilt = os.environ.get("WXMV_FFMPEG_DIR")
    expected_license = (
        (pathlib.Path(selfbuilt) / "LICENSE.txt").read_bytes() if selfbuilt
        else (root / "build/licenses/GPLv3.txt").read_bytes()
    )
    expected = {prefix + name for name in (
        "README.md", "wx-mini-video.yaml", "THIRD-PARTY-NOTICES.md",
        "wx-mini-video.app/Contents/Info.plist", executable_name,
        "wx-mini-video.app/Contents/Resources/logo.icns",
        "ffmpeg/ffmpeg-amd64", "ffmpeg/ffmpeg-arm64", "ffmpeg/LICENSE.txt",
    )}
    with zipfile.ZipFile(archive) as package:
        assert len(package.infolist()) == len(expected), package.namelist()
        assert set(package.namelist()) == expected, package.namelist()
        assert package.read(prefix + "README.md") == (root / "docs/macos-usage.md").read_bytes()
        assert package.read(prefix + "wx-mini-video.yaml") == (root / "internal/config/config.template.yaml").read_bytes()
        assert package.read(prefix + "ffmpeg/LICENSE.txt") == expected_license
        assert package.read(prefix + "THIRD-PARTY-NOTICES.md") == (root / "THIRD-PARTY-NOTICES.md").read_bytes()
        assert (package.getinfo(prefix + executable_name).external_attr >> 16) & 0o777 == 0o755
        bundle_icon = prefix + "wx-mini-video.app/Contents/Resources/logo.icns"
        assert package.read(bundle_icon) == bundle_icon_bytes
        plist = plistlib.loads(package.read(prefix + "wx-mini-video.app/Contents/Info.plist"))
        assert plist["LSMinimumSystemVersion"] == "13.0"
        assert plist["CFBundleIconFile"] == "logo"
        assert plist["CFBundleIconName"] == "logo"
        binary = package.read(prefix + executable_name)
        assert struct.unpack_from(">II", binary) == (0xCAFEBABE, 2)
        commit = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
        for index, (arch, cpu) in enumerate(ffmpeg_arches):
            actual_cpu, _, offset, size, align = struct.unpack_from(">IIIII", binary, 8 + 20 * index)
            thin = root / ("dist/wx-mini-video-darwin-" + arch)
            assert actual_cpu == cpu and align == 14 and offset % (1 << align) == 0
            assert binary[offset:offset + size] == thin.read_bytes()
            metadata = subprocess.check_output(["go", "version", "-m", str(thin)], text=True)
            assert "vcs.revision=" + commit in metadata and "vcs.modified=false" in metadata
        # 随包 ffmpeg 必须是两份原始单架构 Mach-O，逐字节等于已校验的固定 ZIP 内容。
        for arch, cpu in ffmpeg_arches:
            name = prefix + "ffmpeg/ffmpeg-" + arch
            ffmpeg_binary = package.read(name)
            assert (package.getinfo(name).external_attr >> 16) & 0o777 == 0o755, name
            assert_thin_mach_o(ffmpeg_binary, cpu, name)
            if selfbuilt:
                assert ffmpeg_binary == (pathlib.Path(selfbuilt) / ("ffmpeg-" + arch)).read_bytes(), f"{name} 与预构建文件不一致"
            else:
                cached = root / f"dist/ffmpeg-cache/martin-riedl-9.0.2-{arch}.zip"
                with zipfile.ZipFile(cached) as source:
                    assert ffmpeg_binary == source.read("ffmpeg"), f"{name} 与固定构建不一致（不得 lipo）"
            assert len(ffmpeg_binary) > 10 * 1024 * 1024, name
        assert package.read(prefix + "ffmpeg/ffmpeg-amd64") != package.read(prefix + "ffmpeg/ffmpeg-arm64")
    # 在 --package-only 下也验证 TUI 验收用的隔离配置契约（关闭系统代理、
    # 跳过证书安装、独立下载目录），避免脚本改回会触碰用户系统设置的配置。
    with tempfile.TemporaryDirectory(prefix="wx-mini-video 配置 ") as directory:
        config, downloads, home = write_isolated_config(directory)
        text = config.read_text(encoding="utf-8")
        assert "system: false" in text, "隔离配置必须关闭系统代理"
        assert "skipInstallRootCert: true" in text, "隔离配置必须跳过根证书安装"
        assert 'appID: ""' in text, "隔离配置不得预填或自动确认目标"
        assert downloads == pathlib.Path(directory) / "downloads", "隔离配置的下载目录必须落在临时目录"
        assert home.is_dir(), "隔离 HOME 未创建"
    if "--package-only" in sys.argv[1:]:
        print("已通过 ZIP、双架构、随包 ffmpeg、许可、构建来源、应用图标与权限检查；隔离配置契约通过；未执行 macOS 原生启动")
        return
    with tempfile.TemporaryDirectory(prefix="wx-mini-video 测试 ") as directory:
        subprocess.run(["/usr/bin/ditto", "-x", "-k", str(archive), directory], check=True)
        portable = pathlib.Path(directory) / "wx-mini-video-macos"
        executable = portable / executable_name
        assert executable.stat().st_mode & 0o777 == 0o755
        subprocess.run(["/usr/bin/lipo", str(executable), "-verify_arch", "x86_64", "arm64"], check=True)
        subprocess.run(["/usr/bin/plutil", "-lint", str(portable / "wx-mini-video.app/Contents/Info.plist")], check=True)
        assert (portable / "wx-mini-video.app/Contents/Resources/logo.icns").read_bytes() == bundle_icon_bytes
        for arch, lipo_arch in (("amd64", "x86_64"), ("arm64", "arm64")):
            ffmpeg_path = portable / f"ffmpeg/ffmpeg-{arch}"
            assert ffmpeg_path.stat().st_mode & 0o777 == 0o755, ffmpeg_path
            subprocess.run(["/usr/bin/lipo", str(ffmpeg_path), "-verify_arch", lipo_arch], check=True)
        assert (portable / "ffmpeg/LICENSE.txt").read_bytes() == expected_license
        config, downloads, home = write_isolated_config(directory)
        check_tui(executable, directory, config, downloads, home)
        moved = portable.with_name("移动后的便携目录")
        portable.rename(moved)
        check_tui(moved / executable_name, directory, config, downloads, home)
    print("已通过 ZIP、双架构、随包 ffmpeg、许可、构建来源、应用图标、权限、中文空格路径和移动目录后的 TUI 启动/退出检查（隔离配置，未修改系统代理或安装证书）")


if __name__ == "__main__":
    main()
