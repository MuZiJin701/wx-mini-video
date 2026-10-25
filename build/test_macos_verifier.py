import importlib.util
import pathlib
import os
import subprocess
import sys
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("macos_verifier", pathlib.Path(__file__).with_name("verify-macos.py"))
verifier = importlib.util.module_from_spec(spec)
spec.loader.exec_module(verifier)


class StartupDiagnosticsTests(unittest.TestCase):
    @unittest.skipIf(os.name == "nt", "PTY drain requires POSIX")
    def test_exit_drains_output_larger_than_the_pty_buffer(self):
        import pty
        master, slave = pty.openpty()
        process = subprocess.Popen(
            [sys.executable, "-c", "import os; os.write(1, b'x' * 1048576)"],
            stdin=subprocess.DEVNULL, stdout=slave, stderr=slave,
        )
        try:
            output = bytearray()
            self.assertEqual(verifier.wait_for_tui_exit(process, master, output, timeout=10), 0)
            self.assertGreater(len(output), 65536)
        finally:
            if process.poll() is None:
                process.kill()
                process.wait()
            os.close(master)
            os.close(slave)

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
