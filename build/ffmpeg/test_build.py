import hashlib
import importlib.util
import io
import pathlib
import tarfile
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("ffmpeg_builder", pathlib.Path(__file__).with_name("build.py"))
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)


class BuildMaterialTests(unittest.TestCase):
    def test_windows_archive_patch_is_recorded_and_rejects_changed_upstream(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            template = root / "Configurations/unix-Makefile.tmpl"
            template.parent.mkdir()
            template.write_text("my $max_per_call = 500;\n")
            patch = builder.patch_windows_archiver(root, root)
            self.assertEqual(template.read_text(), "my $max_per_call = 100;\n")
            self.assertIn("-my $max_per_call = 500;", patch.read_text())
            with self.assertRaisesRegex(ValueError, "规则已变化"):
                builder.patch_windows_archiver(root, root)

    def test_command_uses_build_environment_path(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            with patch.object(builder.shutil, "which", return_value="portable/compiler") as lookup:
                with patch.object(builder.subprocess, "run") as run:
                    run.return_value.returncode = 0
                    builder.run(["compiler", "--version"], root, {"PATH": "portable-tools"}, root / "build.log")
            lookup.assert_called_once_with("compiler", path="portable-tools")
            self.assertEqual(run.call_args.args[0], ["portable/compiler", "--version"])

    def test_corrupt_download_is_not_cached(self):
        with tempfile.TemporaryDirectory() as directory:
            response = io.BytesIO(b"tampered source")
            response.url = "https://example.com/source.tar.xz"
            with patch.object(builder.urllib.request, "urlopen", return_value=response):
                with self.assertRaisesRegex(ValueError, "SHA-256"):
                    builder.download(response.url, hashlib.sha256(b"expected source").hexdigest(), pathlib.Path(directory))
            self.assertEqual(list(pathlib.Path(directory).iterdir()), [])

    def test_verified_source_cache_does_not_access_network(self):
        with tempfile.TemporaryDirectory() as directory:
            cache = pathlib.Path(directory)
            digest = hashlib.sha256(b"source").hexdigest()
            expected = cache / (digest[:12] + "-source.tar.xz")
            expected.write_bytes(b"source")
            with patch.object(builder.urllib.request, "urlopen") as network:
                self.assertEqual(builder.download("https://example.com/source.tar.xz", digest, cache), expected)
                network.assert_not_called()

    def test_source_extraction_rejects_escape(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            archive = root / "bad.tar"
            with tarfile.open(archive, "w") as source:
                member = tarfile.TarInfo("../outside.txt")
                member.size = 6
                source.addfile(member, io.BytesIO(b"escape"))
            with self.assertRaises(tarfile.FilterError):
                builder.extract_source(archive, root / "unpacked")
            self.assertFalse((root / "outside.txt").exists())


if __name__ == "__main__":
    unittest.main()
