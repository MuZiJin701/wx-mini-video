"""校验 ffmpeg 自建产物、build-info、源码归档之间的一致性。"""

import argparse
import hashlib
import json
import pathlib
import sys
import tarfile
import zipfile

EXPECTED_CASES = {
    "local-h264-ts", "local-hevc-fmp4", "local-h264-aes",
    "http-h264-ts", "http-hevc-fmp4", "http-h264-aes",
    "tls12-h264-ts", "tls12-hevc-fmp4", "tls12-h264-aes",
    "tls13-h264-ts", "tls13-hevc-fmp4", "tls13-h264-aes",
}
if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")
REQUIRED_RECIPE = {
    "recipe/build.py", "recipe/verify.py", "recipe/test_build.py",
    "recipe/sources.json", "recipe/README.md", "recipe/LICENSE",
    "recipe/fixtures/h264-aac.ts", "recipe/fixtures/hevc-aac.ts",
}
REQUIRED_RECORDS = {
    "records/build.log", "records/validation.json", "records/build-info.json",
    "records/openssl-source/openssl-3.5.9/configdata.pm",
    "records/ffmpeg-build/config.h", "records/ffmpeg-build/config_components.h",
    "records/ffmpeg-build/ffbuild/config.log", "records/ffmpeg-build/ffbuild/config.mak",
}


def sha256(path):
    with pathlib.Path(path).open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def safe_member(name):
    path = pathlib.PurePosixPath(name)
    return not path.is_absolute() and ".." not in path.parts and not name.startswith("/")


def check_binary_zip(path, target, build_info):
    if not safe_member(path.name):
        raise ValueError("产物路径异常: " + str(path))
    expected_name = "ffmpeg.exe" if target.startswith("windows-") else "ffmpeg"
    with zipfile.ZipFile(path) as archive:
        names = archive.namelist()
        if sorted(names) != sorted([expected_name, "LICENSE.txt"]):
            raise ValueError(f"ffmpeg.zip 条目不符: {names}")
        for name in names:
            if not safe_member(name):
                raise ValueError("ffmpeg.zip 条目路径异常: " + name)
        binary = archive.read(expected_name)
        if hashlib.sha256(binary).hexdigest() != build_info["binary_sha256"]:
            raise ValueError("ffmpeg.zip 中的二进制摘要与 build-info 不符")
        license_text = archive.read("LICENSE.txt").decode("utf-8")
    for marker in ("FFmpeg", "OpenSSL", "GNU Lesser General Public License", "Apache License", "Independent JPEG Group"):
        if marker not in license_text:
            raise ValueError("ffmpeg.zip 缺少许可文本: " + marker)


def check_source_archive(path, target, build_info):
    with tarfile.open(path) as archive:
        names = set(archive.getnames())
    for name in names:
        if not safe_member(name):
            raise ValueError("源码归档条目路径异常: " + name)
    missing = sorted((REQUIRED_RECIPE | REQUIRED_RECORDS) - names)
    if not any(name.startswith("sources/ffmpeg/") for name in names):
        missing.append("sources/ffmpeg/")
    if not any(name.startswith("sources/openssl/") for name in names):
        missing.append("sources/openssl/")
    if target.startswith("windows-") and not any(name.startswith("sources/windows_toolchain/") for name in names):
        missing.append("sources/windows_toolchain/")
    if target.startswith("windows-") and "records/openssl-windows-ar.patch" not in names:
        missing.append("records/openssl-windows-ar.patch")
    if missing:
        raise ValueError("源码归档缺少条目: " + ", ".join(missing))


def verify(directory):
    directory = pathlib.Path(directory).resolve()
    build_info = json.loads((directory / "build-info.json").read_text(encoding="utf-8"))
    sources = json.loads((pathlib.Path(__file__).resolve().parent / "sources.json").read_text(encoding="utf-8"))
    target = build_info.get("target")
    if target not in ("windows-amd64", "darwin-amd64", "darwin-arm64"):
        raise ValueError("未知构建目标: " + str(target))
    if build_info.get("sources") != sources:
        raise ValueError("build-info 的固定来源与当前 sources.json 不一致")
    validation = build_info.get("validation") or {}
    if validation.get("count") != len(EXPECTED_CASES) or set(validation.get("passed") or []) != EXPECTED_CASES:
        raise ValueError("十二项媒体验证记录不完整")
    binary_zip = directory / "ffmpeg.zip"
    source_archive = directory / "corresponding-source.tar.gz"
    if sha256(binary_zip) != build_info.get("binary_archive_sha256"):
        raise ValueError("ffmpeg.zip 摘要与 build-info 不符")
    if sha256(source_archive) != build_info.get("source_archive_sha256"):
        raise ValueError("对应源码归档摘要与 build-info 不符")
    check_binary_zip(binary_zip, target, build_info)
    check_source_archive(source_archive, target, build_info)
    print(f"产物校验通过: {target}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=pathlib.Path)
    args = parser.parse_args()
    try:
        verify(args.directory)
    except (OSError, ValueError, KeyError, json.JSONDecodeError) as error:
        sys.exit(str(error))
