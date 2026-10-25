import importlib.util
import pathlib
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("macos_verifier", pathlib.Path(__file__).with_name("verify-macos.py"))
verifier = importlib.util.module_from_spec(spec)
spec.loader.exec_module(verifier)


class StartupDiagnosticsTests(unittest.TestCase):
    def test_failure_reason_survives_long_frames_and_partial_terminal_output(self):
        with tempfile.TemporaryDirectory() as directory:
            log = pathlib.Path(directory) / "wx-mini-video.log"
            log.write_text("启动失败: keychain unavailable", encoding="utf-8")
            output = ("old frame" * 1000 + "代理启动失败").encode("utf-8")
            text = verifier.startup_diagnostics(output, directory)
            self.assertIn("代理启动失败", text)
            self.assertIn("keychain unavailable", text)

    def test_missing_log_keeps_terminal_diagnostics(self):
        with tempfile.TemporaryDirectory() as directory:
            self.assertEqual(verifier.startup_diagnostics(b"startup failed", directory), "startup failed")


if __name__ == "__main__":
    unittest.main()
