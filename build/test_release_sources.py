import copy
import hashlib
import importlib.util
import io
import json
import pathlib
import tempfile
import unittest
from unittest.mock import patch

ROOT = pathlib.Path(__file__).resolve().parent.parent
spec = importlib.util.spec_from_file_location("release_sources", ROOT / "build/verify-release-sources.py")
verifier = importlib.util.module_from_spec(spec)
spec.loader.exec_module(verifier)


class SourceVerificationTests(unittest.TestCase):
    def test_incomplete_materials_block_release_before_network(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            (root / "build").mkdir()
            (root / "internal/minidownload").mkdir(parents=True)
            (root / "internal/minidownload/ffmpeg.go").write_bytes((ROOT / "internal/minidownload/ffmpeg.go").read_bytes())
            records = json.loads((ROOT / "build/ffmpeg-corresponding-sources.json").read_text(encoding="utf-8"))
            for record in records.values():
                record["reviewed"] = False
            (root / "build/ffmpeg-corresponding-sources.json").write_text(json.dumps(records), encoding="utf-8")
            with patch.object(verifier.urllib.request, "urlopen") as download:
                with self.assertRaisesRegex(ValueError, "禁止公开发布"):
                    verifier.verify_sources(root)
                download.assert_not_called()

    def test_source_archive_hash_is_checked(self):
        records = json.loads((ROOT / "build/ffmpeg-corresponding-sources.json").read_text(encoding="utf-8"))
        for record in records.values():
            record.update(reviewed=True, source_url="https://example.com/source.tar.xz",
                          source_sha256=hashlib.sha256(b"source archive").hexdigest())
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            (root / "build").mkdir()
            (root / "internal/minidownload").mkdir(parents=True)
            (root / "internal/minidownload/ffmpeg.go").write_bytes((ROOT / "internal/minidownload/ffmpeg.go").read_bytes())
            (root / "build/ffmpeg-corresponding-sources.json").write_text(json.dumps(records), encoding="utf-8")
            def response(*args, **kwargs):
                body = io.BytesIO(b"source archive")
                body.url = "https://example.com/source.tar.xz"
                return body
            with patch.object(verifier.urllib.request, "urlopen", side_effect=response):
                verifier.verify_sources(root)
                altered = copy.deepcopy(records)
                altered["darwin-arm64"]["source_sha256"] = "0" * 64
                (root / "build/ffmpeg-corresponding-sources.json").write_text(json.dumps(altered), encoding="utf-8")
                with self.assertRaisesRegex(ValueError, "不匹配"):
                    verifier.verify_sources(root)


if __name__ == "__main__":
    unittest.main()
