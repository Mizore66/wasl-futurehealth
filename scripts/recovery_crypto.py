"""Versioned recovery keyring and AES-256-GCM primitives.

No key, DEK or plaintext value is ever included in an exception message.
"""
from __future__ import annotations

import base64
import binascii
import fcntl
import json
import os
import re
from pathlib import Path

from cryptography.exceptions import InvalidTag
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

SCHEMA = "wasl-recovery-keyring-v1"
KEY_BYTES = 32
NONCE_BYTES = 12
WRAP_AAD_PREFIX = b"wasl-file-backup-v2-aesgcm|"
_KEY_ID = re.compile(r"[A-Za-z0-9._-]{1,64}")


def _b64(raw: bytes) -> str:
    return base64.b64encode(raw).decode("ascii")


def _unb64(value, size: int | None, label: str) -> bytes:
    """Strict canonical standard base64; `size` is the exact decoded length (None = any non-empty)."""
    if not isinstance(value, str) or not value or not value.isascii():
        raise ValueError(f"{label} is malformed")
    try:
        raw = base64.b64decode(value, validate=True)
    except (binascii.Error, ValueError):
        raise ValueError(f"{label} is malformed") from None
    if _b64(raw) != value or (size is not None and len(raw) != size) or not raw:
        raise ValueError(f"{label} is malformed")
    return raw


def _check_key_id(key_id) -> str:
    if not isinstance(key_id, str) or not _KEY_ID.fullmatch(key_id):
        raise ValueError("recovery key id is invalid")
    return key_id


def _entry_keys(entry, label: str = "recovery key entry") -> tuple[bytes, bytes]:
    if not isinstance(entry, dict) or set(entry) != {"wrap_key", "auth_key"}:
        raise ValueError(f"{label} is malformed")
    wrap = _unb64(entry["wrap_key"], KEY_BYTES, f"{label} wrap key")
    auth = _unb64(entry["auth_key"], KEY_BYTES, f"{label} auth key")
    if wrap == auth:
        raise ValueError(f"{label} keys must be independent")
    return wrap, auth


def _new_entry() -> dict:
    return {"wrap_key": _b64(os.urandom(KEY_BYTES)), "auth_key": _b64(os.urandom(KEY_BYTES))}


def _no_duplicates(pairs):
    out = {}
    for key, value in pairs:
        if key in out:
            raise ValueError("recovery keyring contains duplicate entries")
        out[key] = value
    return out


def _validate(doc) -> tuple[dict, str, dict]:
    if not isinstance(doc, dict) or set(doc) != {"schema", "active", "keys"} or doc["schema"] != SCHEMA:
        raise ValueError("recovery keyring is not a valid versioned keyring")
    keys = doc["keys"]
    if not isinstance(keys, dict) or not keys:
        raise ValueError("recovery keyring has no key generations")
    for key_id, entry in keys.items():
        _check_key_id(key_id)
        _entry_keys(entry, "recovery key generation")
    active = doc["active"]
    if not isinstance(active, str) or active not in keys:
        raise ValueError("recovery keyring active generation is missing")
    return doc, active, keys[active]


def _atomic_write(path: Path, doc: dict, mode: int, uid: int | None = None, gid: int | None = None,
                  replace: bool = False) -> None:
    data = (json.dumps(doc, sort_keys=True, indent=2) + "\n").encode("utf-8")
    tmp = path.with_name(f".{path.name}.{os.urandom(6).hex()}.tmp")
    fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(fd, "wb") as handle:
            handle.write(data)
            handle.flush()
            if uid is not None and (uid, gid) != (os.fstat(handle.fileno()).st_uid, os.fstat(handle.fileno()).st_gid):
                os.fchown(handle.fileno(), uid, gid)
            os.fchmod(handle.fileno(), mode)
            os.fsync(handle.fileno())
        if replace:
            os.replace(tmp, path)
        else:
            os.link(tmp, path)  # fails if the destination exists; never overwrites
            os.unlink(tmp)
        dir_fd = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(dir_fd)
        finally:
            os.close(dir_fd)
    except BaseException:
        try:
            os.unlink(tmp)
        except FileNotFoundError:
            pass
        raise


def new_keyring(path: Path) -> dict:
    """Create a keyring with one active generation. Refuses to overwrite an existing file."""
    path = Path(path)
    key_id = "g-" + os.urandom(8).hex()
    doc = {"schema": SCHEMA, "active": key_id, "keys": {key_id: _new_entry()}}
    _atomic_write(path, doc, 0o600)
    return doc


def load_keyring(path: Path) -> tuple[dict, str, dict]:
    try:
        text = Path(path).read_bytes().decode("utf-8")
        doc = json.loads(text, object_pairs_hook=_no_duplicates)
    except (OSError, UnicodeDecodeError, json.JSONDecodeError):
        raise ValueError("recovery keyring is unreadable or malformed") from None
    return _validate(doc)


def rotate_keyring(path: Path) -> str:
    """Add a fresh active generation, retaining all old ones; preserves owner and mode."""
    path = Path(path)
    lock_path = path.with_name(f".{path.name}.lock")
    lock_fd = os.open(lock_path, os.O_RDWR | os.O_CREAT, 0o600)
    try:
        fcntl.flock(lock_fd, fcntl.LOCK_EX)
        doc, _, _ = load_keyring(path)
        st = os.stat(path)
        key_id = "g-" + os.urandom(8).hex()
        while key_id in doc["keys"]:
            key_id = "g-" + os.urandom(8).hex()
        doc["keys"][key_id] = _new_entry()
        doc["active"] = key_id
        _atomic_write(path, doc, st.st_mode & 0o7777, st.st_uid, st.st_gid, replace=True)
        return key_id
    finally:
        os.close(lock_fd)  # releases the flock


def new_dek() -> bytes:
    return os.urandom(KEY_BYTES)


def _aead(key, label: str) -> AESGCM:
    if not isinstance(key, bytes) or len(key) != KEY_BYTES:
        raise ValueError(f"{label} must be 32 bytes")
    return AESGCM(key)


def encrypt_payload(dek: bytes, aad: bytes, plaintext: bytes) -> tuple[bytes, bytes]:
    aead = _aead(dek, "data key")
    nonce = os.urandom(NONCE_BYTES)
    return nonce, aead.encrypt(nonce, plaintext, aad)


def decrypt_payload(dek: bytes, aad: bytes, nonce: bytes, ciphertext: bytes) -> bytes:
    aead = _aead(dek, "data key")
    if not isinstance(nonce, bytes) or len(nonce) != NONCE_BYTES or not isinstance(ciphertext, bytes):
        raise ValueError("encrypted payload is malformed")
    try:
        return aead.decrypt(nonce, ciphertext, aad)
    except InvalidTag:
        raise ValueError("payload authentication failed: wrong key or modified data") from None


def wrap_dek(entry: dict, key_id: str, dek: bytes) -> tuple[str, str]:
    wrap_key, _ = _entry_keys(entry)
    _check_key_id(key_id)
    _aead(dek, "data key")
    nonce = os.urandom(NONCE_BYTES)
    wrapped = AESGCM(wrap_key).encrypt(nonce, dek, WRAP_AAD_PREFIX + key_id.encode())
    return _b64(nonce), _b64(wrapped)


def unwrap_dek(entry: dict, key_id: str, wrap_nonce: str, wrapped: str) -> bytes:
    wrap_key, _ = _entry_keys(entry)
    _check_key_id(key_id)
    nonce = _unb64(wrap_nonce, NONCE_BYTES, "wrap nonce")
    blob = _unb64(wrapped, KEY_BYTES + 16, "wrapped data key")
    try:
        return AESGCM(wrap_key).decrypt(nonce, blob, WRAP_AAD_PREFIX + key_id.encode())
    except InvalidTag:
        raise ValueError("data key unwrap failed: wrong key or modified bundle") from None
