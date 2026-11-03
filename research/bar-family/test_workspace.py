#!/usr/bin/env python3
"""Archive trust-boundary checks; no vendor files or analysis engines needed."""
import hashlib
from pathlib import Path
import stat
import tempfile
import unittest
from unittest.mock import patch
import zipfile

import firmware_workspace as workspace


class ArchiveBoundaries(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def archive(self, entries):
        path = self.root / "input.zip"
        with zipfile.ZipFile(path, "w") as stream:
            for name, value in entries:
                stream.writestr(name, value)
        return path

    def test_parent_path_cannot_escape(self):
        archive = self.archive([("../update.zip", b"payload")])
        with self.assertRaises(ValueError):
            workspace.unpack(archive, self.root / "out")
        self.assertFalse((self.root / "update.zip").exists())

    def test_duplicate_basename_cannot_shadow(self):
        archive = self.archive([("one/update.zip", b"one"), ("two/update.zip", b"two")])
        with self.assertRaises(ValueError):
            workspace.unpack(archive, self.root / "out")
        self.assertEqual((self.root / "out/update.zip").read_bytes(), b"one")

    def test_symlink_is_not_materialized(self):
        link = zipfile.ZipInfo("update.zip")
        link.create_system = 3
        link.external_attr = (stat.S_IFLNK | 0o777) << 16
        archive = self.archive([(link, b"/etc/passwd")])
        with self.assertRaises(ValueError):
            workspace.unpack(archive, self.root / "out")
        self.assertFalse((self.root / "out/update.zip").exists())

    def test_existing_output_is_preserved(self):
        archive = self.archive([("update.zip", b"replacement")])
        out = self.root / "out"
        out.mkdir()
        (out / "update.zip").write_bytes(b"keep")
        with self.assertRaises(FileExistsError):
            workspace.unpack(archive, out)
        self.assertEqual((out / "update.zip").read_bytes(), b"keep")

    def test_output_budget_stops_before_write(self):
        archive = self.archive([("update.zip", b"payload")])
        with patch.object(workspace, "MAX_MEMBER", 3), self.assertRaises(ValueError):
            workspace.unpack(archive, self.root / "out")
        self.assertFalse((self.root / "out/update.zip").exists())

    def test_supported_payload_is_byte_exact(self):
        payload = b"\x00firmware\xff"
        archive = self.archive([("vendor/system.ext4", payload)])
        rows = workspace.unpack(archive, self.root / "out", inner=True)
        self.assertEqual((self.root / "out/system.ext4").read_bytes(), payload)
        self.assertEqual(rows[0]["sha256"], hashlib.sha256(payload).hexdigest())


if __name__ == "__main__":
    unittest.main()
