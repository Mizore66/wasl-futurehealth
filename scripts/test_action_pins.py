#!/usr/bin/env python3
"""Tests for GitHub Action commit-object pin verification."""
from pathlib import Path
import unittest

import check_action_pins as P


ROOT = Path(__file__).resolve().parents[1]


PNPM_TAG = "9fd676a19091d4595eefd76e4bd31c97133911f1"
PNPM_COMMIT = "41ff72655975bd51cab0327fa583b6e92b6d3061"


class ActionPinTests(unittest.TestCase):
    def test_commit_object_is_accepted(self):
        calls = []

        def fetch(repo, sha):
            calls.append((repo, sha))
            return {"sha": sha}

        refs = [("pnpm/action-setup", PNPM_COMMIT)]
        self.assertEqual(P.validate_refs(refs, fetch), [])
        self.assertEqual(calls, refs)

    def test_annotated_tag_object_is_rejected(self):
        refs = [("pnpm/action-setup", PNPM_TAG)]
        errors = P.validate_refs(refs, lambda repo, sha: None)
        self.assertEqual(len(errors), 1)
        self.assertIn("not a commit object", errors[0])
        self.assertIn(PNPM_TAG, errors[0])

    def test_missing_or_mismatched_response_is_rejected(self):
        refs = [("pnpm/action-setup", PNPM_COMMIT)]
        errors = P.validate_refs(refs, lambda repo, sha: {"sha": "0" * 40})
        self.assertEqual(len(errors), 1)
        self.assertIn("returned a different commit", errors[0])

    def test_workflow_parser_requires_full_commit_pins(self):
        text = """
- uses: actions/checkout@08c6903cd8c0fde910a37f88322edcfb5dd907a8 # v5.0.0
- uses: pnpm/action-setup@v4
- uses: ./local-action
"""
        refs, errors = P.parse_workflow(text)
        self.assertEqual(refs, [("actions/checkout", "08c6903cd8c0fde910a37f88322edcfb5dd907a8")])
        self.assertEqual(len(errors), 1)
        self.assertIn("pnpm/action-setup@v4", errors[0])

    def test_supply_chain_uses_official_release_commit_pins(self):
        text = (ROOT / ".github" / "workflows" / "supply-chain.yml").read_text(encoding="utf-8")
        self.assertIn(
            "uses: actions/setup-go@d35c59abb061a4a6fb18e82ac0862c26744d6ab5 # v5.5.0",
            text,
        )
        self.assertIn(
            "uses: pnpm/action-setup@41ff72655975bd51cab0327fa583b6e92b6d3061 # v4.2.0",
            text,
        )
        refs, errors = P.parse_workflow(text)
        self.assertEqual(errors, [])
        self.assertIn(("actions/setup-go", "d35c59abb061a4a6fb18e82ac0862c26744d6ab5"), refs)
        self.assertIn(("pnpm/action-setup", PNPM_COMMIT), refs)


if __name__ == "__main__":
    unittest.main()
