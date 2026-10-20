"""在 macOS 验证便携包及终端内启动；不触发系统授权或启动代理。"""

import fcntl
import os
import pathlib
import plistlib
import pty
import select
import struct
import subprocess
import tempfile
import termios
import time
import zipfile


def check_tui(executable, cwd):
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


def main():
    root = pathlib.Path.cwd()
    archive = root / "dist/wx-mini-video-macos-universal-unsigned.zip"
    prefix = "wx-mini-video-macos/"
    executable_name = "wx-mini-video.app/Contents/MacOS/wx-mini-video"
    with zipfile.ZipFile(archive) as package:
        expected = {prefix + name for name in (
            "README.md", "wx-mini-video.yaml", "wx-mini-video.app/Contents/Info.plist", executable_name,
        )}
        assert len(package.infolist()) == 4 and set(package.namelist()) == expected
        assert package.read(prefix + "README.md") == (root / "docs/macos-usage.md").read_bytes()
        assert package.read(prefix + "wx-mini-video.yaml") == (root / "internal/config/config.template.yaml").read_bytes()
        assert (package.getinfo(prefix + executable_name).external_attr >> 16) & 0o777 == 0o755
        plist = plistlib.loads(package.read(prefix + "wx-mini-video.app/Contents/Info.plist"))
        assert plist["LSMinimumSystemVersion"] == "13.0"
        binary = package.read(prefix + executable_name)
        assert struct.unpack_from(">II", binary) == (0xCAFEBABE, 2)
        commit = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
        for index, (arch, cpu) in enumerate((("amd64", 0x01000007), ("arm64", 0x0100000C))):
            actual_cpu, _, offset, size, align = struct.unpack_from(">IIIII", binary, 8 + 20 * index)
            thin = root / ("dist/wx-mini-video-darwin-" + arch)
            assert actual_cpu == cpu and align == 14 and offset % (1 << align) == 0
            assert binary[offset:offset + size] == thin.read_bytes()
            metadata = subprocess.check_output(["go", "version", "-m", str(thin)], text=True)
            assert "vcs.revision=" + commit in metadata and "vcs.modified=false" in metadata
    with tempfile.TemporaryDirectory(prefix="wx-mini-video 测试 ") as directory:
        subprocess.run(["/usr/bin/ditto", "-x", "-k", str(archive), directory], check=True)
        portable = pathlib.Path(directory) / "wx-mini-video-macos"
        executable = portable / executable_name
        assert executable.stat().st_mode & 0o777 == 0o755
        subprocess.run(["/usr/bin/lipo", str(executable), "-verify_arch", "x86_64", "arm64"], check=True)
        subprocess.run(["/usr/bin/plutil", "-lint", str(portable / "wx-mini-video.app/Contents/Info.plist")], check=True)
        check_tui(executable, directory)
        moved = portable.with_name("移动后的便携目录")
        portable.rename(moved)
        check_tui(moved / executable_name, directory)
    print("已通过 ZIP、双架构、构建来源、权限、中文空格路径和移动目录后的 TUI 启动/退出检查")


if __name__ == "__main__":
    main()
