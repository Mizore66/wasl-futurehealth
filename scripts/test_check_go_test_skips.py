#!/usr/bin/env python3
import contextlib
import io
import json
from pathlib import Path
import tempfile
import unittest

import check_go_test_skips as check


REQUIRED = (
    "TestAPIFixturesAndContracts",
    "TestAPIReviewRBACAndAppendOnly",
    "TestAPIBookingSupport",
    "TestHL7RoundTripCohort",
)
OPTIONAL = (
    "TestFresh2Reader",
    "TestAPIDRFixtureRun",
    "TestNATSBackupRestorePreservesRepeatedMessageIDs",
    "TestNATSBackupRestoreReadsRetainedMessagesOnEmptyCluster",
)


def event(action, test=None, output=None):
    row = {"Action": action}
    if test is not None:
        row["Test"] = test
    if output is not None:
        row["Output"] = output
    return row


def required_passes():
    return [event("run", name) for name in REQUIRED] + [event("pass", name) for name in REQUIRED] + [event("pass")]


class SkipPolicyTests(unittest.TestCase):
    def test_required_passes_and_named_optional_skips_are_accepted(self):
        rows = required_passes() + [event("skip", name) for name in OPTIONAL]
        report, errors = check.evaluate(rows, "integration")
        self.assertEqual(errors, [])
        self.assertEqual(report["summary"]["required_passed"], 4)
        self.assertEqual(report["summary"]["allowed_skips"], 4)
        self.assertNotIn("Output", json.dumps(report))

    def test_deleting_a_required_pass_fails_closed(self):
        rows = required_passes()
        deleted = next(row for row in rows if row["Action"] == "pass" and row.get("Test") == REQUIRED[1])
        rows.remove(deleted)
        report, errors = check.evaluate(rows, "integration")
        self.assertTrue(any("missing required" in error and deleted["Test"] in error for error in errors))
        self.assertEqual(report["required"][deleted["Test"]], "missing")

    def test_unexpected_top_level_and_package_skips_fail(self):
        for row in (event("skip", "TestUnexpected"), event("skip")):
            with self.subTest(row=row):
                _, errors = check.evaluate(required_passes() + [row], "integration")
                self.assertTrue(any("unexpected skip" in error for error in errors))

    def test_required_subtest_skip_cannot_hide_under_parent_pass(self):
        rows = required_passes() + [event("skip", REQUIRED[0] + "/one_fixture")]
        _, errors = check.evaluate(rows, "integration")
        self.assertTrue(any("required test skipped" in error and REQUIRED[0] in error for error in errors))

    def test_optional_subtest_skip_uses_exact_top_level_policy(self):
        rows = required_passes() + [event("skip", OPTIONAL[0] + "/missing_local_corpus")]
        report, errors = check.evaluate(rows, "integration")
        self.assertEqual(errors, [])
        self.assertEqual(report["summary"]["allowed_skips"], 1)
        rows[-1]["Test"] = OPTIONAL[0] + "_unexpected/missing_local_corpus"
        _, errors = check.evaluate(rows, "integration")
        self.assertTrue(any("unexpected skip" in error for error in errors))

    def test_test_and_package_failures_are_not_marked_passing(self):
        for row in (event("fail", REQUIRED[0]), event("fail")):
            with self.subTest(row=row):
                _, errors = check.evaluate(required_passes() + [row], "integration")
                self.assertTrue(any("test failure" in error for error in errors))

    def test_materialized_fixtures_may_skip_only_outside_integration(self):
        for lane in ("go-test", "go-race"):
            with self.subTest(lane=lane):
                rows = [event("run", REQUIRED[0])] + [event("skip", name) for name in REQUIRED] + [event("pass")]
                report, errors = check.evaluate(rows, lane)
                self.assertEqual(errors, [])
                self.assertEqual(report["summary"]["allowed_skips"], 4)
        _, errors = check.evaluate([event("skip", REQUIRED[0])], "integration")
        self.assertTrue(any("required test skipped" in error for error in errors))

    def test_non_required_lane_has_no_required_pass_gate(self):
        report, errors = check.evaluate([event("run", OPTIONAL[0]), event("skip", OPTIONAL[0]), event("pass")], "go-test")
        self.assertEqual(errors, [])
        self.assertEqual(report["required"], {})


    def test_empty_and_truncated_logs_fail_terminal_evidence_gate(self):
        for rows in ([], [event("run", "TestSomething")]):
            with self.subTest(rows=rows):
                _, errors = check.evaluate(rows, "go-test")
                self.assertTrue(any("terminal package pass" in error for error in errors))
                if not rows:
                    self.assertTrue(any("no test events" in error for error in errors))

    def test_subtest_pass_does_not_satisfy_required_parent(self):
        rows = [event("run", REQUIRED[0] + "/child"), event("pass", REQUIRED[0] + "/child"), event("pass")]
        _, errors = check.evaluate(rows, "integration")
        self.assertTrue(any("missing required" in error for error in errors))

    def test_duplicate_parent_passes_count_once_and_fail_is_sticky(self):
        rows = required_passes() + [event("pass", REQUIRED[0]), event("fail", REQUIRED[0]), event("pass", REQUIRED[0])]
        report, errors = check.evaluate(rows, "integration")
        self.assertTrue(any("test failure" in error for error in errors))
        self.assertEqual(report["summary"]["required_passed"], 3)
        self.assertEqual(report["required"][REQUIRED[0]], "failed")


class CommandTests(unittest.TestCase):
    def test_malformed_json_and_manifest_output(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "go.jsonl"
            source.write_text("{not-json}\n")
            with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(check.main(["--input", str(source), "--lane", "go-test"]), 1)
            source.write_text("".join(json.dumps(row) + "\n" for row in required_passes()))
            output = root / "manifest.json"
            with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(check.main(["--input", str(source), "--lane", "integration", "--output", str(output)]), 0)
            manifest = json.loads(output.read_text())
            self.assertEqual(manifest["schema"], "wasl-go-test-skip-manifest/1")
            self.assertEqual(manifest["lane"], "integration")
            self.assertEqual(manifest["summary"]["required_passed"], 4)

    def test_non_string_test_field_is_rejected_without_traceback(self):
        with tempfile.TemporaryDirectory() as tmp:
            source = Path(tmp) / "go.jsonl"
            source.write_text('{"Action":"skip","Test":42}\n')
            with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(check.main(["--input", str(source), "--lane", "go-test"]), 1)



if __name__ == "__main__":
    unittest.main()
