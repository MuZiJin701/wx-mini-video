"""从固定源码构建独立 ffmpeg；所有工具、缓存和产物仅写入 dist。"""

import argparse
import difflib
import hashlib
import json
import os
import pathlib
import platform
import shlex
import shutil
import subprocess
import sys
import tarfile
import tempfile
import urllib.parse
import urllib.request
import zipfile

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parent.parent
TARGETS = {"windows-amd64": "mingw64", "darwin-amd64": "darwin64-x86_64-cc",
           "darwin-arm64": "darwin64-arm64-cc"}
if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")


def sha256(path):
    with pathlib.Path(path).open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def download(url, digest, cache):
    if urllib.parse.urlsplit(url).scheme != "https" or len(digest) != 64:
        raise ValueError("来源必须为 HTTPS，并固定 SHA-256")
    cache.mkdir(parents=True, exist_ok=True)
    path = cache / (digest[:12] + "-" + url.rsplit("/", 1)[-1])
    if path.is_file() and sha256(path) == digest:
        return path
    print("下载并校验:", url, flush=True)
    request = urllib.request.Request(url, headers={"User-Agent": "wx-mini-video-ffmpeg-builder"})
    temporary = None
    try:
        with urllib.request.urlopen(request, timeout=60) as response:
            if urllib.parse.urlsplit(response.url).scheme != "https":
                raise ValueError("拒绝降级为 HTTP 的下载")
            with tempfile.NamedTemporaryFile(dir=cache, delete=False) as output:
                temporary = pathlib.Path(output.name)
                shutil.copyfileobj(response, output)
        if sha256(temporary) != digest:
            raise ValueError("来源文件 SHA-256 不匹配: " + url)
        temporary.replace(path)
        return path
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)


def extract_source(archive, destination):
    print("解包源码:", archive.name, flush=True)
    destination.mkdir()
    with tarfile.open(archive) as source:
        source.extractall(destination, filter="data")
    roots = list(destination.iterdir())
    if len(roots) != 1 or not roots[0].is_dir():
        raise ValueError("源码归档必须包含一个顶层目录")
    return roots[0]


def run(args, cwd, env, log):
    print("运行:", shlex.join(map(str, args)), flush=True)
    with log.open("ab") as output:
        output.write(("\n$ " + shlex.join(map(str, args)) + "\n").encode())
        output.flush()
        command = list(map(str, args))
        # Windows CreateProcess 搜索程序时不使用传入的 PATH，先显式解析便携工具。
        command[0] = shutil.which(command[0], path=env.get("PATH")) or command[0]
        result = subprocess.run(command, cwd=cwd, env=env,
                                stdout=output, stderr=subprocess.STDOUT)
    if result.returncode:
        print(log.read_text(errors="replace")[-12000:], file=sys.stderr)
        raise RuntimeError(f"命令失败（{result.returncode}），完整日志: {log}")


def unpack_tool(archive, intermediate, destination, sevenzip, env, log):
    run([sevenzip, "x", "-y", "-o" + str(intermediate), archive], ROOT, env, log)
    inner, = intermediate.glob("*.tar")
    run([sevenzip, "x", "-y", "-o" + str(destination), inner], ROOT, env, log)


def patch_windows_archiver(openssl, work):
    template = openssl / "Configurations/unix-Makefile.tmpl"
    original = template.read_text(encoding="utf-8")
    old = "my $max_per_call = 500;"
    if original.count(old) != 1:
        raise ValueError("OpenSSL 归档规则已变化，需重新核实 Windows 补丁")
    # 长对象文件名使 500 项归档命令超过 Windows 命令行上限，分为每批 100 项。
    patched = original.replace(old, "my $max_per_call = 100;")
    template.write_text(patched, encoding="utf-8", newline="\n")
    patch = work / "openssl-windows-ar.patch"
    patch.write_text("".join(difflib.unified_diff(original.splitlines(True), patched.splitlines(True),
                     fromfile="a/Configurations/unix-Makefile.tmpl", tofile="b/Configurations/unix-Makefile.tmpl")), encoding="utf-8")
    return patch


def build(target, jobs):
    host = "windows" if os.name == "nt" else sys.platform
    arch = {"x86_64": "amd64", "AMD64": "amd64", "arm64": "arm64"}.get(platform.machine())
    if target != host + "-" + str(arch):
        raise ValueError("必须在对应架构的 Windows/macOS 原生环境构建")
    locks = json.loads((HERE / "sources.json").read_text())
    cache = ROOT / "dist/ffmpeg-build-cache"
    output = ROOT / "dist/ffmpeg-selfbuilt" / target
    output.mkdir(parents=True, exist_ok=True)
    work = pathlib.Path(tempfile.mkdtemp(prefix="_work-" + target + "-", dir=output.parent))
    print("构建工作目录:", work, flush=True)
    recipe = work / "recipe"
    shutil.copytree(HERE, recipe, ignore=shutil.ignore_patterns("__pycache__"))
    sources = {name: download(locks[name]["url"], locks[name]["sha256"], cache)
               for name in ("ffmpeg", "openssl")}
    ffmpeg = extract_source(sources["ffmpeg"], work / "ffmpeg-source")
    openssl = extract_source(sources["openssl"], work / "openssl-source")
    env = os.environ.copy()
    env["OPENSSL_CONF"] = os.devnull
    log = work / "build.log"
    prefix = (work / "openssl-install").as_posix()
    make = ["make"]
    ffmpeg_make = make
    cc = "clang"
    patches = []
    if host == "windows":
        patches.append(patch_windows_archiver(openssl, work))
        tool = locks["windows_toolchain"]
        tool_archive = download(tool["url"], tool["sha256"], cache)
        sources["windows_toolchain"] = download(tool["source_url"], tool["source_sha256"], cache)
        sevenzip = shutil.which("7z") or r"C:\Program Files\7-Zip\7z.exe"
        run([sevenzip, "x", "-y", "-o" + str(work / "toolchain"), tool_archive], ROOT, env, log)
        bin_dir = work / "toolchain/w64devkit/bin"
        msys = work / "msys/msys64"
        for name, destination in (("msys_base", work / "msys"), ("msys_perl", msys)):
            archive = download(locks[name]["url"], locks[name]["sha256"], cache)
            unpack_tool(archive, work / (name + "-packed"), destination, sevenzip, env, log)
        shell = msys / "usr/bin/sh.exe"
        perl = msys / "usr/bin/perl.exe"
        make_archive = download(locks["msys_make"]["url"], locks["msys_make"]["sha256"], cache)
        unpack_tool(make_archive, work / "msys_make-packed", msys, sevenzip, env, log)
        msys_make = msys / "usr/bin/make.exe"
        if not shell.is_file() or not perl.is_file() or not msys_make.is_file() or not (bin_dir / "gcc.exe").is_file():
            raise ValueError("固定便携 MSYS2/Perl/Make 或 w64devkit 工具链不完整")
        env.pop("PERL5LIB", None)
        env["PATH"] = os.pathsep.join((str(bin_dir), str(perl.parent), env["PATH"]))
        env["MSYSTEM"] = "MINGW64"
        env["LC_ALL"] = "C"
        # OpenSSL 的 Makefile 使用相对路径，原生 make 即可；FFmpeg configure
        # 会写入 MSYS 风格的 /d/... 源路径，必须用 MSYS make 解析。
        make = [str(bin_dir / "make.exe"), "SHELL=" + shell.as_posix()]
        ffmpeg_make = [str(msys_make), "SHELL=" + shell.as_posix()]
        cc = "gcc"
    else:
        shell, perl = pathlib.Path("/bin/sh"), pathlib.Path("/usr/bin/perl")
        env["MACOSX_DEPLOYMENT_TARGET"] = "13.0"
    run([cc, "--version"], work, env, log)
    run([perl, "--version"], work, env, log)
    run([perl, "Configure", TARGETS[target], "no-shared", "no-module", "no-tests", "no-asm",
         "no-autoload-config", "--prefix=" + prefix, "--libdir=lib"], openssl, env, log)
    run(make + ["-j" + str(jobs)], openssl, env, log)
    run(make + ["install_sw"], openssl, env, log)
    build_dir = work / "ffmpeg-build"
    build_dir.mkdir(exist_ok=True)
    cflags, ldflags = "-I" + prefix + "/include", "-L" + prefix + "/lib"
    extra_libs = []
    if host == "windows":
        ldflags += " -static"
        extra_libs = ["--target-os=mingw32", "--arch=x86_64",
                      "--extra-libs=-lws2_32 -lgdi32 -lcrypt32 -ladvapi32 -luser32"]
    else:
        cflags += " -mmacosx-version-min=13.0"
        ldflags += " -mmacosx-version-min=13.0"
    flags = ["--cc=" + cc, "--disable-autodetect", "--disable-gpl", "--disable-nonfree",
             "--enable-version3", "--enable-openssl", "--enable-static", "--disable-shared",
             "--disable-doc", "--disable-debug", "--disable-ffplay", "--disable-ffprobe",
             "--disable-avdevice", "--disable-x86asm", "--pkg-config=false",
             "--extra-cflags=" + cflags, "--extra-ldflags=" + ldflags] + extra_libs
    configure = os.path.relpath(ffmpeg / "configure", build_dir).replace("\\", "/")
    run([shell, configure] + flags, build_dir, env, log)
    name = "ffmpeg.exe" if host == "windows" else "ffmpeg"
    run(ffmpeg_make + ["-j" + str(jobs), name], build_dir, env, log)
    binary = build_dir / name
    openssl_exe = pathlib.Path(prefix) / "bin" / ("openssl.exe" if host == "windows" else "openssl")
    validation = work / "validation.json"
    run([sys.executable, recipe / "verify.py", binary, "--openssl", openssl_exe,
         "--output", validation], ROOT, env, log)
    license_text = "FFmpeg: LGPLv3 (GPLv3 + LGPLv3 supplemental terms). OpenSSL: Apache-2.0.\n\n"
    for path in (ffmpeg / "COPYING.GPLv3", ffmpeg / "COPYING.LGPLv3", openssl / "LICENSE.txt"):
        license_text += path.read_text() + "\n\n"
    if host == "windows":
        license_text += (bin_dir.parent / "COPYING.MinGW-w64-runtime.txt").read_text() + "\n\n"
    license_text += "IJG acknowledgement: This software is based in part on the work of the Independent JPEG Group.\n"
    binary_zip = output / "ffmpeg.zip"
    with zipfile.ZipFile(binary_zip, "w", zipfile.ZIP_DEFLATED) as archive:
        archive.write(binary, name)
        archive.writestr("LICENSE.txt", license_text)
    record = {"target": target, "sources": locks, "configure": flags,
              "host": {"system": platform.platform(), "python": sys.version, "machine": platform.machine()},
              "binary_sha256": sha256(binary), "binary_archive_sha256": sha256(binary_zip),
              "validation": json.loads(validation.read_text()), "work_directory": str(work),
              "recipe_sha256": {str(path.relative_to(recipe)): sha256(path)
                                for path in recipe.rglob("*") if path.is_file()}}
    (work / "build-info.json").write_text(json.dumps(record, indent=2) + "\n")
    source_archive = output / "corresponding-source.tar.gz"
    with tarfile.open(source_archive, "w:gz") as archive:
        for component, path in sources.items():
            archive.add(path, arcname="sources/" + component + "/" + path.name)
        for path in recipe.rglob("*"):
            if path.is_file():
                archive.add(path, arcname="recipe/" + str(path.relative_to(recipe)))
        for path in [log, validation, work / "build-info.json", openssl / "configdata.pm",
                     build_dir / "config.h", build_dir / "config_components.h",
                     build_dir / "ffbuild/config.log", build_dir / "ffbuild/config.mak"] + patches:
            archive.add(path, arcname="records/" + str(path.relative_to(work)))
    record["source_archive_sha256"] = sha256(source_archive)
    (output / "build-info.json").write_text(json.dumps(record, indent=2) + "\n")
    print("已完成本地构建和验证:", output, flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("target", choices=TARGETS)
    parser.add_argument("--jobs", type=int, default=min(os.cpu_count() or 2, 4))
    args = parser.parse_args()
    if args.jobs < 1:
        parser.error("jobs 必须大于零")
    build(args.target, args.jobs)
