"""Deterministic booking-simulation goldens are shared across verified numerical platforms."""
import os
import sys
import unittest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "sim"))
from pool_golden import output_name


class PoolGoldenTests(unittest.TestCase):
    def test_verified_platforms_share_the_deterministic_golden(self):
        for system, machine in (("Linux", "x86_64"), ("Linux", "aarch64"), ("Darwin", "arm64")):
            with self.subTest(system=system, machine=machine):
                self.assertEqual(output_name(system, machine), "pool_test.json")

    def test_unverified_platform_does_not_fall_back_to_the_golden(self):
        with self.assertRaisesRegex(RuntimeError, "no deterministic booking-simulation golden"):
            output_name("Windows", "AMD64")


if __name__ == "__main__":
    unittest.main()
