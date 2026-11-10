#!/usr/bin/env python3
"""Tests for the versioned recovery keyring and AES-256-GCM primitives (synthetic keys only)."""
import base64
import json
import os
import stat
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from recovery_crypto import (  # noqa: E402
    decrypt_payload,
    encrypt_payload,
    load_keyring,
    new_dek,
    new_keyring,
    rotate_keyring,
    unwrap_dek,
    wrap_dek,
)


def b64(raw: bytes) -> str:
    return base64.b64encode(raw).decode("ascii")


def make_keyring(path: Path) -> dict:
    keyring = new_keyring(path)
    os.chmod(path, 0o444)
    return keyring


class KeyringTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.path = self.root / "recovery_keys.json"

    def write(self, doc):
        if self.path.exists():
            os.chmod(self.path, 0o600)
        self.path.write_text(json.dumps(doc))

    def good(self, **over):
        doc = {
            "schema": "wasl-recovery-keyring-v1",
            "active": "g1",
            "keys": {"g1": {"wrap_key": b64(os.urandom(32)), "auth_key": b64(os.urandom(32))}},
        }
        doc.update(over)
        return doc

    def assert_rejects_without_secret(self, doc, secrets=()):
        self.write(doc)
        with self.assertRaises(ValueError) as ctx:
            load_keyring(self.path)
        for secret in secrets:
            self.assertNotIn(secret, str(ctx.exception))

    def test_new_keyring_shape_and_independent_keys(self):
        keyring = make_keyring(self.path)
        self.assertEqual(keyring["schema"], "wasl-recovery-keyring-v1")
        entry = keyring["keys"][keyring["active"]]
        self.assertEqual(len(base64.b64decode(entry["wrap_key"], validate=True)), 32)
        self.assertEqual(len(base64.b64decode(entry["auth_key"], validate=True)), 32)
        self.assertNotEqual(entry["wrap_key"], entry["auth_key"])
        self.assertEqual(stat.S_IMODE(self.path.stat().st_mode), 0o444)
        loaded, active, active_entry = load_keyring(self.path)
        self.assertEqual(loaded, keyring)
        self.assertEqual(active_entry, entry)

    def test_new_keyring_refuses_existing_file(self):
        make_keyring(self.path)
        with self.assertRaises(FileExistsError):
            new_keyring(self.path)

    def test_raw_nonversioned_key_rejected(self):
        self.path.write_bytes(os.urandom(48))
        with self.assertRaises(ValueError):
            load_keyring(self.path)
        self.write({"hmac_key": b64(os.urandom(32))})
        with self.assertRaises(ValueError):
            load_keyring(self.path)

    def test_invalid_fields_do_not_leak_key_bytes(self):
        short = b64(os.urandom(16))
        self.assert_rejects_without_secret(
            self.good(keys={"g1": {"wrap_key": short, "auth_key": b64(os.urandom(32))}}), [short])
        bad = "!!!notbase64" + b64(os.urandom(32))
        self.assert_rejects_without_secret(
            self.good(keys={"g1": {"wrap_key": bad, "auth_key": b64(os.urandom(32))}}), [bad])
        good = self.good()
        self.assert_rejects_without_secret({**good, "active": "missing"},
                                           [good["keys"]["g1"]["wrap_key"], good["keys"]["g1"]["auth_key"]])
        self.assert_rejects_without_secret({**good, "schema": "other"})
        self.assert_rejects_without_secret({**good, "keys": {}})
        self.assert_rejects_without_secret({**good, "extra": 1})
        same = b64(os.urandom(32))
        self.assert_rejects_without_secret(
            self.good(keys={"g1": {"wrap_key": same, "auth_key": same}}), [same])

    def test_duplicate_generation_ids_rejected(self):
        k = b64(os.urandom(32))
        a = b64(os.urandom(32))
        self.path.write_text(
            '{"schema":"wasl-recovery-keyring-v1","active":"g1","keys":'
            f'{{"g1":{{"wrap_key":"{k}","auth_key":"{a}"}},"g1":{{"wrap_key":"{a}","auth_key":"{k}"}}}}}}')
        with self.assertRaises(ValueError):
            load_keyring(self.path)

    def test_rotation_preserves_old_generation_and_decrypts(self):
        keyring = self.path
        initial = make_keyring(keyring)
        old_id = initial["active"]
        old_dek = new_dek()
        old_entry = initial["keys"][old_id]
        nonce, wrapped = wrap_dek(old_entry, old_id, old_dek)
        before = os.stat(keyring)
        new_id = rotate_keyring(keyring)
        after = os.stat(keyring)
        rotated, active, _ = load_keyring(keyring)
        self.assertNotEqual(new_id, old_id)
        self.assertEqual(active, new_id)
        self.assertEqual(rotated["keys"][old_id], old_entry)
        self.assertEqual(stat.S_IMODE(after.st_mode), 0o444)
        self.assertEqual((after.st_uid, after.st_gid), (before.st_uid, before.st_gid))
        self.assertEqual(unwrap_dek(old_entry, old_id, nonce, wrapped), old_dek)
        new_entry = rotated["keys"][active]
        self.assertNotEqual(new_entry["wrap_key"], old_entry["wrap_key"])
        self.assertNotEqual(new_entry["auth_key"], old_entry["auth_key"])
        self.assertEqual(unwrap_dek(new_entry, active, *wrap_dek(new_entry, active, old_dek)), old_dek)
        self.assertEqual(sorted(os.listdir(self.root)), ["recovery_keys.json"])

    def test_rotate_rejects_invalid_keyring(self):
        self.write({"nope": 1})
        with self.assertRaises(ValueError):
            rotate_keyring(self.path)


class AeadTests(unittest.TestCase):
    def setUp(self):
        self.entry = {"wrap_key": b64(os.urandom(32)), "auth_key": b64(os.urandom(32))}

    def test_payload_roundtrip_and_fresh_nonce(self):
        dek = new_dek()
        self.assertEqual(len(dek), 32)
        self.assertNotEqual(dek, new_dek())
        n1, c1 = encrypt_payload(dek, b"aad", b"same plaintext")
        n2, c2 = encrypt_payload(dek, b"aad", b"same plaintext")
        self.assertEqual((len(n1), len(n2)), (12, 12))
        self.assertNotEqual(n1, n2)
        self.assertNotEqual(c1, c2)
        self.assertNotIn(b"same plaintext", c1)
        self.assertEqual(decrypt_payload(dek, b"aad", n1, c1), b"same plaintext")

    def test_payload_tamper_and_wrong_key_fail_without_leak(self):
        dek = new_dek()
        nonce, ct = encrypt_payload(dek, b"aad", b"secret-plaintext")
        flipped = bytes([ct[0] ^ 1]) + ct[1:]
        for args in ((dek, b"aad", nonce, flipped), (dek, b"other", nonce, ct),
                     (new_dek(), b"aad", nonce, ct), (dek, b"aad", os.urandom(12), ct),
                     (dek, b"aad", nonce, ct[:-1]), (dek, b"aad", nonce[:-1], ct),
                     (dek[:-1], b"aad", nonce, ct)):
            with self.assertRaises(ValueError) as ctx:
                decrypt_payload(*args)
            self.assertNotIn("secret-plaintext", str(ctx.exception))

    def test_wrap_roundtrip_fresh_nonce_and_key_id_binding(self):
        dek = new_dek()
        n1, w1 = wrap_dek(self.entry, "g1", dek)
        n2, w2 = wrap_dek(self.entry, "g1", dek)
        self.assertNotEqual(n1, n2)
        self.assertNotEqual(w1, w2)
        self.assertEqual(len(base64.b64decode(n1, validate=True)), 12)
        self.assertEqual(unwrap_dek(self.entry, "g1", n1, w1), dek)
        with self.assertRaises(ValueError):
            unwrap_dek(self.entry, "g2", n1, w1)
        other = {"wrap_key": b64(os.urandom(32)), "auth_key": self.entry["auth_key"]}
        with self.assertRaises(ValueError) as ctx:
            unwrap_dek(other, "g1", n1, w1)
        self.assertNotIn(other["wrap_key"], str(ctx.exception))

    def test_unwrap_strict_field_validation(self):
        dek = new_dek()
        nonce, wrapped = wrap_dek(self.entry, "g1", dek)
        bad = [
            ("", wrapped), (nonce, ""), ("!!!", wrapped), (nonce, "!!!"),
            (b64(os.urandom(11)), wrapped), (nonce, wrapped[:-4]),
            (nonce + "\n", wrapped), (nonce, wrapped + " "), (None, wrapped), (nonce, 5),
        ]
        for n, w in bad:
            with self.assertRaises(ValueError):
                unwrap_dek(self.entry, "g1", n, w)
        with self.assertRaises(ValueError):
            unwrap_dek({"wrap_key": "x", "auth_key": "y"}, "g1", nonce, wrapped)
        with self.assertRaises(ValueError):
            wrap_dek(self.entry, "g1", b"short")
        with self.assertRaises(ValueError):
            wrap_dek(self.entry, "", dek)


if __name__ == "__main__":
    unittest.main()
