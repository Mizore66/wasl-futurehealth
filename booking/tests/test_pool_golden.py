"""Exact booking-simulation goldens are explicit about the numerical platform they reproduce."""
import os
import sys
import unittest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "sim"))
from pool_golden import output_name


class PoolGoldenTests(unittest.TestCase):
    def test_linux_amd64_has_its_own_exact_golden(self):
        self.assertEqual(output_name("Linux", "x86_64"), "pool_test.linux-amd64.json")

    def test_original_darwin_arm64_golden_is_unchanged(self):
        self.assertEqual(output_name("Darwin", "arm64"), "pool_test.json")

    def test_unverified_platform_does_not_fall_back_to_another_golden(self):
        with self.assertRaisesRegex(RuntimeError, "no exact booking-simulation golden"):
            output_name("Linux", "aarch64")


if __name__ == "__main__":
    unittest.main()
