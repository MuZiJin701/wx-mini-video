"""公开发布前校验已人工核对的 Corresponding Source 下载入口及摘要。"""

import hashlib
import json
import pathlib
import re
import sys
import urllib.parse
import urllib.request


def verify_sources(root):
    records = json.loads((root / "build/ffmpeg-corresponding-sources.json").read_text(encoding="utf-8"))
    platforms = {"windows-amd64", "darwin-amd64", "darwin-arm64"}
    if set(records) != platforms:
        raise ValueError("对应源码索引必须覆盖 Windows 与 Mac 的全部随包架构")
    runtime_sources = (root / "internal/minidownload/ffmpeg.go").read_text(encoding="utf-8")
    # 按代码中的首选源顺序核对，避免更新二进制后继续使用上一构建的源码声明。
    primary_hashes = re.findall(r'SHA256:\s*"([0-9a-f]{64})"', runtime_sources)
    if len(primary_hashes) != 4:
        raise ValueError("ffmpeg 来源列表已变化，请复核源码索引校验")
    expected = dict(zip(("windows-amd64", "darwin-amd64", "darwin-arm64"),
                        (primary_hashes[0], primary_hashes[2], primary_hashes[3])))
    for platform in sorted(platforms):
        record = records[platform]
        url = record.get("source_url") or ""
        digest = record.get("source_sha256") or ""
        if (record.get("binary_archive_sha256") != expected[platform]
                or record.get("reviewed") is not True
                or urllib.parse.urlsplit(url).scheme != "https"
                or not re.fullmatch(r"[0-9a-f]{64}", digest)):
            raise ValueError(f"{platform} 对应源码未就绪：需核实完整源码/依赖/构建材料并填写 HTTPS 入口及 SHA-256；禁止公开发布")
    for platform in sorted(platforms):
        record = records[platform]
        request = urllib.request.Request(record["source_url"], headers={"User-Agent": "wx-mini-video-source-verifier"})
        hasher = hashlib.sha256()
        with urllib.request.urlopen(request, timeout=60) as response:
            if urllib.parse.urlsplit(response.url).scheme != "https":
                raise ValueError("对应源码下载不得降级为 HTTP")
            while block := response.read(1024 * 1024):
                hasher.update(block)
        if hasher.hexdigest() != record["source_sha256"]:
            raise ValueError(f"{platform} 对应源码下载 SHA-256 不匹配")
    print("对应源码入口及下载摘要校验通过；源码完整性及与二进制的对应关系以人工核对记录为准")


if __name__ == "__main__":
    try:
        verify_sources(pathlib.Path(__file__).resolve().parent.parent)
    except (ValueError, OSError) as error:
        sys.exit(str(error))
