#!/usr/bin/env python3
"""Generate or check a SHA256SUMS manifest for a dependency bundle.

Usage: offline_dependency_manifest.py generate|check <root>

Only reads and hashes files; nothing under <root> is ever executed.
"""
import hashlib
import re
import sys
from pathlib import Path, PurePosixPath

MANIFEST = "SHA256SUMS"
LINE = re.compile(r"^([0-9a-f]{64})  (.+)$")


def fail(msg):
    print("error: " + msg, file=sys.stderr)
    sys.exit(1)


def scan(root):
    """Return sorted relative POSIX paths of regular files, excluding the manifest."""
    found = []
    for path in sorted(root.rglob("*")):
        if path.is_symlink():
            fail("symlink not allowed: %s" % path.relative_to(root))
        if path.is_dir():
            continue
        if not path.is_file():
            fail("not a regular file: %s" % path.relative_to(root))
        rel = path.relative_to(root).as_posix()
        if rel != MANIFEST:
            found.append(rel)
    return sorted(found)


def digest(root, rel):
    return hashlib.sha256((root / rel).read_bytes()).hexdigest()


def safe(rel):
    p = PurePosixPath(rel)
    return bool(rel) and not p.is_absolute() and ".." not in p.parts and "." not in p.parts and "\\" not in rel


def generate(root):
    files = scan(root)
    text = "".join("%s  %s\n" % (digest(root, rel), rel) for rel in files)
    (root / MANIFEST).write_text(text)
    print("wrote %d entries to %s" % (len(files), root / MANIFEST))


def check(root):
    manifest = root / MANIFEST
    if manifest.is_symlink() or not manifest.is_file():
        fail("missing %s" % MANIFEST)
    listed = {}
    for n, line in enumerate(manifest.read_text().splitlines(), 1):
        m = LINE.match(line)
        if not m:
            fail("line %d malformed" % n)
        want, rel = m.groups()
        if not safe(rel):
            fail("line %d unsafe path: %s" % (n, rel))
        if rel in listed:
            fail("line %d duplicate path: %s" % (n, rel))
        listed[rel] = want
    actual = scan(root)
    missing = sorted(set(listed) - set(actual))
    extra = sorted(set(actual) - set(listed))
    if missing:
        fail("listed but missing: %s" % ", ".join(missing))
    if extra:
        fail("present but unlisted: %s" % ", ".join(extra))
    if list(listed) != sorted(listed):
        fail("manifest is not sorted by path")
    for rel, want in listed.items():
        if digest(root, rel) != want:
            fail("hash mismatch: %s" % rel)
    print("ok: %d entries verified in %s" % (len(listed), root))


def main(argv):
    if len(argv) != 3 or argv[1] not in ("generate", "check"):
        fail("usage: offline_dependency_manifest.py generate|check <root>")
    root = Path(argv[2])
    if root.is_symlink() or not root.is_dir():
        fail("not a directory: %s" % root)
    (generate if argv[1] == "generate" else check)(root)


if __name__ == "__main__":
    main(sys.argv)
