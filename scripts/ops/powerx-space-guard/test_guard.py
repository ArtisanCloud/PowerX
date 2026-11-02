import importlib.util
import os
from pathlib import Path
import tempfile
import time
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("space_guard", Path(__file__).with_name("guard.py"))
guard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(guard)


class RetentionSafetyTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.base = Path(self.tmp.name).resolve()

    def tearDown(self):
        self.tmp.cleanup()

    def make_file(self, name, age_days=10):
        p = self.base / name
        p.write_bytes(b"diagnostic log\n" * 400)
        old = time.time() - age_days * 86400
        os.utime(p, (old, old))
        return p

    def test_active_files_symlinks_hardlinks_and_open_files_are_excluded(self):
        active = self.make_file("current.log")
        good = self.make_file("closed.log.gz")
        opened = self.make_file("open.log.gz")
        hard = self.make_file("hard.log.gz")
        os.link(hard, self.base / "hard-alias.log.gz")
        (self.base / "link.log.gz").symlink_to(good)
        s = opened.stat()
        items = guard.candidates(self.base, r".*\.log\.gz", {(s.st_dev, s.st_ino)})
        self.assertEqual([p.name for _, p, _ in items], [good.name])
        self.assertTrue(active.exists())

    def test_budget_keeps_the_latest_day_even_under_pressure(self):
        newest = self.make_file("new.log.gz", 0.5)
        old = self.make_file("old.log.gz", 2)
        items = guard.candidates(self.base, r".*\.log\.gz", set())
        chosen = guard.select_expired(items, time.time(), 7, 0)
        self.assertEqual([p for p, _, _ in chosen], [old])
        self.assertTrue(newest.exists())

    def test_age_expiry_applies_even_when_under_budget(self):
        old = self.make_file("old.log.gz", 8)
        self.make_file("recent.log.gz", 6)
        items = guard.candidates(self.base, r".*\.log\.gz", set())
        chosen = guard.select_expired(items, time.time(), 7, guard.GIB)
        self.assertEqual([p for p, _, _ in chosen], [old])

    def test_changed_file_and_new_open_handle_fail_closed(self):
        p = self.make_file("closed.log.gz")
        s = p.stat()
        p.write_bytes(b"changed")
        with self.assertRaises(RuntimeError):
            guard.safe_unlink(p, s)
        s = p.stat()
        with patch.object(guard, "open_inodes", return_value={(s.st_dev, s.st_ino)}):
            with self.assertRaises(RuntimeError):
                guard.safe_unlink(p, s)
        self.assertTrue(p.exists())

    def test_preview_does_not_delete_or_create_state(self):
        p = self.make_file("old.log.gz")
        report = {"files": []}
        with patch.object(guard, "RULES", [("test", str(self.base), r".*\.log\.gz", guard.GIB)]):
            guard.retain_logs(False, 7, report)
        self.assertTrue(p.exists())
        self.assertEqual(len(report["files"]), 1)
        self.assertFalse(report["files"][0]["deleted"])
        self.assertEqual(list(self.base.iterdir()), [p])

    def test_apply_removes_only_the_selected_closed_file(self):
        old = self.make_file("old.log.gz")
        current = self.make_file("active.log")
        report = {"files": []}
        with patch.object(guard, "RULES", [("test", str(self.base), r".*\.log\.gz", guard.GIB)]):
            guard.retain_logs(True, 7, report)
        self.assertFalse(old.exists())
        self.assertTrue(current.exists())
        self.assertTrue(report["files"][0]["deleted"])

    def test_every_audit_source_keeps_expired_and_over_budget_files(self):
        for label in guard.AUDIT_SOURCES:
            with self.subTest(source=label):
                old = self.make_file(label + "-old.log.gz", 40)
                pressured = self.make_file(label + "-pressure.log.gz", 2)
                report = {"files": []}
                with patch.object(guard, "RULES", [(label, str(self.base), r".*\.log\.gz", 0)]), patch.object(guard, "safe_unlink") as unlink:
                    guard.retain_logs(True, 7, report)
                unlink.assert_not_called()
                self.assertTrue(old.exists())
                self.assertTrue(pressured.exists())
                self.assertTrue(report["audit_archive"][label]["over_budget"])
                self.assertTrue(all(not e["deleted"] for e in report["files"]))
                self.assertTrue(all(e["blocked_reason"] == "verified_offhost_archive_required" for e in report["files"]))
                old.unlink()
                pressured.unlink()

    def test_postgres_native_rotation_cannot_expire_unverified_archives(self):
        text = Path(__file__).with_name("postgresql-common.logrotate").read_text()
        directives = [line.strip() for line in text.splitlines() if line.strip() and not line.lstrip().startswith("#")]
        self.assertIn("rotate -1", directives)
        self.assertFalse(any(line.startswith("maxage ") for line in directives))


if __name__ == "__main__":
    unittest.main()
