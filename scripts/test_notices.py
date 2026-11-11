import copy
import hashlib
import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import notices  # noqa: E402

TEXT = "Example MIT notice\n"
SHA = "b2fdef494b60e26685728b6807eeb78bd91c2e6f62da0c9b79f78450eb628077"
COMMIT = "0123456789012345678901234567890123456789"
LOCK = "packages:\n  example@1.0.0:\n    resolution: {integrity: sha512-AAAA}\n"


def direct():
    return {
        "name": "example",
        "version": "1.0.0",
        "declaration": "MIT",
        "evidence_class": "direct-gitHead",
        "source": {"url": "https://example.test/LICENSE", "commit": COMMIT},
        "qualification": "Upstream copy; does not prove shipment in the npm tarball.",
        "notices": [{"path": "notices/mit-example.txt", "sha256": SHA}],
    }


def manifest():
    return {"schema_version": 1, "packages": [direct()]}


def as_tag(m):
    p = m["packages"][0]
    p["evidence_class"] = "tag-candidate"
    p["source"] = {"url": "https://example.test/x", "tag": "v1", "resolved_commit": COMMIT}
    return p


def as_unresolved(m):
    p = m["packages"][0]
    p["evidence_class"] = "unresolved-native"
    p["source"] = {"url": "https://example.test/x"}
    return p


class NoticesTest(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.root = Path(self._tmp.name)
        (self.root / "web").mkdir()
        (self.root / "web" / "pnpm-lock.yaml").write_text(LOCK, encoding="utf-8")
        self.notice = self.root / "supply-chain" / "notices" / "mit-example.txt"
        self.notice.parent.mkdir(parents=True)
        self.notice.write_bytes(TEXT.encode())

    def fails(self, m, pattern, **kw):
        with self.assertRaisesRegex(ValueError, pattern):
            notices.validate_notice_manifest(m, self.root, **kw)

    def test_fixture_hash(self):
        self.assertEqual(hashlib.sha256(TEXT.encode()).hexdigest(), SHA)

    def test_valid(self):
        notices.validate_notice_manifest(manifest(), self.root)

    def test_missing_file(self):
        self.notice.unlink()
        self.fails(manifest(), "missing notice file")

    def test_wrong_hash(self):
        m = manifest()
        m["packages"][0]["notices"][0]["sha256"] = "0" * 64
        self.fails(m, "SHA-256 mismatch")

    def test_stale_lockfile_version(self):
        (self.root / "web" / "pnpm-lock.yaml").write_text(LOCK.replace("1.0.0", "1.0.1"), encoding="utf-8")
        self.fails(manifest(), "not in web/pnpm-lock.yaml")

    def test_duplicate(self):
        m = manifest()
        m["packages"].append(direct())
        self.fails(m, "duplicate package")

    def test_direct_empty_notices(self):
        m = manifest()
        m["packages"][0]["notices"] = []
        self.fails(m, "requires notices")

    def test_tag_without_qualification(self):
        m = manifest()
        p = as_tag(m)
        del p["qualification"]
        self.fails(m, "requires qualification")

    def test_tag_valid(self):
        m = manifest()
        as_tag(m)
        notices.validate_notice_manifest(m, self.root)

    def test_tag_missing_tag(self):
        m = manifest()
        as_tag(m)
        del m["packages"][0]["source"]["tag"]
        self.fails(m, "requires source tag")

    def test_tag_missing_resolved_commit(self):
        m = manifest()
        as_tag(m)
        del m["packages"][0]["source"]["resolved_commit"]
        self.fails(m, "resolved_commit")

    def test_unresolved_with_notices(self):
        m = manifest()
        p = as_unresolved(m)
        p["limitation"] = "No notice text located."
        self.fails(m, "must not have notices")

    def test_unresolved_without_limitation(self):
        m = manifest()
        p = as_unresolved(m)
        p["notices"] = []
        self.fails(m, "requires limitation")

    def test_unresolved_valid(self):
        m = manifest()
        p = as_unresolved(m)
        p["notices"] = []
        p["limitation"] = "No notice text located."
        notices.validate_notice_manifest(m, self.root)

    def test_malformed_schema(self):
        self.fails([], "manifest must be an object")
        self.fails({"schema_version": 2, "packages": []}, "schema_version")
        self.fails({"schema_version": 1, "packages": "x"}, "packages must be a list")
        m = manifest()
        m["packages"][0]["evidence_class"] = "bogus"
        self.fails(m, "unknown evidence_class")
        m = manifest()
        m["packages"][0]["declaration"] = ""
        self.fails(m, "declaration")
        m = manifest()
        m["packages"][0]["source"]["url"] = ""
        self.fails(m, "source url")
        m = manifest()
        m["packages"][0]["source"]["commit"] = "abc"
        self.fails(m, "40-character")
        m = manifest()
        m["packages"].append("x")
        self.fails(m, "must be an object")

    def test_path_escape(self):
        for bad in ("/etc/passwd", "../web/pnpm-lock.yaml", "notices/../../x"):
            m = manifest()
            m["packages"][0]["notices"][0]["path"] = bad
            self.fails(m, "invalid notice path")

    def test_non_utf8(self):
        raw = b"\xff\xfe\x00bad"
        self.notice.write_bytes(raw)
        m = manifest()
        m["packages"][0]["notices"][0]["sha256"] = hashlib.sha256(raw).hexdigest()
        self.fails(m, "not valid UTF-8")

    def test_duplicate_json_keys(self):
        p = self.root / "m.json"
        p.write_text('{"a": 1, "a": 2}', encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "duplicate JSON key"):
            notices.load_manifest(p)

    def test_production_counts(self):
        self.fails(
            manifest(),
            "evidence_class counts",
            expected_counts={"direct-gitHead": 47, "tag-candidate": 21, "unresolved-native": 13},
        )

    def test_counts_match(self):
        notices.validate_notice_manifest(
            manifest(),
            self.root,
            expected_counts={"direct-gitHead": 1, "tag-candidate": 0, "unresolved-native": 0},
        )

    def test_render(self):
        m = manifest()
        out = notices.render_notice_bundle(m, self.root)
        for needle in (
            "example@1.0.0",
            "MIT",
            "direct-gitHead",
            m["packages"][0]["qualification"],
            "Example MIT notice",
            "https://example.test/LICENSE",
            COMMIT,
            "do not prove",
            "tag-candidate",
            "unresolved-native",
        ):
            self.assertIn(needle, out)
        self.assertEqual(out, notices.render_notice_bundle(copy.deepcopy(m), self.root))
        self.assertNotIn(str(self.root), out)

    def test_render_ends_with_exactly_one_newline(self):
        for m in (manifest(), {"schema_version": 1, "packages": []}):
            out = notices.render_notice_bundle(m, self.root)
            self.assertTrue(out.endswith("\n"))
            self.assertFalse(out.endswith("\n\n"))

    def test_render_input_order(self):
        m = manifest()
        second = direct()
        second["name"] = "aaa"
        m["packages"].append(second)
        (self.root / "web" / "pnpm-lock.yaml").write_text(
            LOCK + "  aaa@1.0.0:\n    resolution: {integrity: sha512-BBBB}\n", encoding="utf-8"
        )
        out = notices.render_notice_bundle(m, self.root)
        self.assertLess(out.index("example@1.0.0"), out.index("aaa@1.0.0"))

    def _write_manifest(self):
        (self.root / "supply-chain" / "notice-evidence.json").write_text(json.dumps(manifest()), encoding="utf-8")

    def _bundle(self):
        return self.root / "supply-chain" / "THIRD-PARTY-NOTICES.txt"

    def test_cli(self):
        self._write_manifest()
        out = self._bundle()
        self.assertEqual(
            notices.main(["render", "--root", str(self.root), "--output", str(out), "--no-production-counts"]), 0
        )
        self.assertIn("Example MIT notice", out.read_text(encoding="utf-8"))
        self.assertEqual(notices.main(["check", "--root", str(self.root), "--no-production-counts"]), 0)
        self.assertNotEqual(notices.main(["check", "--root", str(self.root)]), 0)

    def test_check_fails_on_bundle_drift(self):
        self._write_manifest()
        out = self._bundle()
        notices.main(["render", "--root", str(self.root), "--output", str(out), "--no-production-counts"])
        out.write_text(out.read_text(encoding="utf-8") + "drift\n", encoding="utf-8")
        self.assertEqual(notices.main(["check", "--root", str(self.root), "--no-production-counts"]), 1)

    def test_check_fails_on_missing_bundle(self):
        self._write_manifest()
        self.assertEqual(notices.main(["check", "--root", str(self.root), "--no-production-counts"]), 1)

    def test_symlink_escape_rejected(self):
        outside = Path(self._tmp.name) / "outside.txt"
        outside.write_bytes(TEXT.encode())
        link = self.root / "supply-chain" / "notices" / "link.txt"
        link.symlink_to(outside)
        m = manifest()
        m["packages"][0]["notices"][0]["path"] = "notices/link.txt"
        self.fails(m, "escapes supply-chain")

    def test_tag_missing_tag_message(self):
        m = manifest()
        as_tag(m)
        del m["packages"][0]["source"]["tag"]
        self.fails(m, "tag row requires source tag$")


if __name__ == "__main__":
    unittest.main()
