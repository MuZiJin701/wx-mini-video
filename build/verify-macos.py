"""在 macOS 验证便携包及终端内启动；不触发系统授权或启动代理。"""

import os
import pathlib
import plistlib
import select
import struct
import subprocess
import tempfile
import sys
import time
import zipfile


def check_tui(executable, cwd):
    import fcntl
    import pty
    import termios
    master, slave = pty.openpty()
    process = None
    output = bytearray()
    try:
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 30, 120, 0, 0))
        env = dict(os.environ, TERM="xterm-256color", WX_MINI_VIDEO_MAC_TUI="1")
        env.pop("WX_MINI_VIDEO_CONFIG", None)
        process = subprocess.Popen(
            [str(executable)], cwd=cwd, env=env,
            stdin=slave, stdout=slave, stderr=slave,
        )
        deadline = time.monotonic() + 30
        while b"AppID" not in output and time.monotonic() < deadline:
            if select.select([master], [], [], 0.2)[0]:
                try:
                    chunk = os.read(master, 65536)
                except OSError:
                    break
                if not chunk:
                    break
                output.extend(chunk)
            if process.poll() is not None:
                break
        assert b"AppID" in output, output.decode("utf-8", errors="replace")
        os.write(master, b"\x1b")
        assert process.wait(timeout=10) == 0, output.decode("utf-8", errors="replace")
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
    if "--package-only" in sys.argv[1:]:
        print("已通过 ZIP、双架构、随包 ffmpeg、许可、构建来源、应用图标与权限检查；未执行 macOS 原生启动")
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
        check_tui(executable, directory)
        moved = portable.with_name("移动后的便携目录")
        portable.rename(moved)
        check_tui(moved / executable_name, directory)
    print("已通过 ZIP、双架构、随包 ffmpeg、许可、构建来源、应用图标、权限、中文空格路径和移动目录后的 TUI 启动/退出检查")


if __name__ == "__main__":
    main()
