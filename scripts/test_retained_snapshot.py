"""Expose retained HTTP/projector regressions to normal test_*.py CI discovery."""
import unittest

import redacted_snapshot_test as projector


class RetainedSnapshotContractTest(unittest.TestCase):
    setUp = projector.RedactedSnapshotTest.setUp
    test_actual_retained_http_contract = (
        projector.RedactedSnapshotTest.test_actual_retained_http_contract_keeps_only_lifecycle_and_scoped_history
    )
    test_legacy_person_allowlist = (
        projector.RedactedSnapshotTest.test_person_snapshot_uses_strict_allowlist_and_aliases
    )
    test_raw_registration_and_canary_rejection = (
        projector.RedactedSnapshotTest.test_person_snapshot_rejects_raw_registrations_and_canaries
    )
    test_legacy_redacted_detail = (
        projector.RedactedSnapshotTest.test_detail_uses_only_redacted_export_and_adds_empty_defaults
    )
    test_reject_non_redacted_export = (
        projector.RedactedSnapshotTest.test_non_redacted_or_wrong_schema_fails_closed
    )


if __name__ == "__main__":
    unittest.main()
