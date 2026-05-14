#!/usr/bin/env python3
import struct
import unittest

from unpack_sb_assets import unpack_sb_bytes


def stored_sb(payload: bytes) -> bytes:
    return b"=SB=" + struct.pack(">I", len(payload)) + struct.pack(">I", len(payload)) + payload


class UnpackSBAssetsTest(unittest.TestCase):
    def test_unpack_sb_bytes_recursively_unwraps_stored_containers(self) -> None:
        self.assertEqual(unpack_sb_bytes(stored_sb(stored_sb(b"clips")), "twolev.clips"), b"clips")


if __name__ == "__main__":
    unittest.main()
