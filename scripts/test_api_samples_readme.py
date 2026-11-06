"""Guards the regeneration recipe in docs/api-samples/README.md against known clean-room defects."""
import pathlib
import re
import unittest

README = (pathlib.Path(__file__).resolve().parent.parent / "docs" / "api-samples" / "README.md").read_text()


class RegenRecipe(unittest.TestCase):
    def test_export_block_uses_working_ifhas_count(self):
        # WASL_IFHAS=800 panics the generator (background pool exhausted); the recipe must copy-paste cleanly.
        block = README.split("## Regenerating the samples", 1)[1].split("```", 2)[1]
        self.assertIn("WASL_IFHAS=700", block)
        self.assertNotIn("WASL_IFHAS=800", block)

    def test_states_regeneration_is_not_byte_identical(self):
        self.assertRegex(README, r"(?i)not byte-identical")
        for field in ("loaded_at", "in_seq", "throughput_ev_s"):
            self.assertIn(field, README)

    def test_names_nats_binary_requirement(self):
        self.assertRegex(README, re.compile(r"nats-server", re.I))


if __name__ == "__main__":
    unittest.main()
