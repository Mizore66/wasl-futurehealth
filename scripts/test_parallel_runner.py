import json
import os
import subprocess
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
RUNNER = os.path.join(HERE, "pairfeat_experiment.sh")
WRAPPER = os.path.join(HERE, "pairfeat_parallel.sh")


class ParallelRunnerTests(unittest.TestCase):
    def test_single_seed_and_binary_isolation(self):
        with open(RUNNER) as h:
            text = h.read()
        for want in ("PAIRFEAT_SEED", 'BIN_DIR="${PAIRFEAT_BIN_DIR:-$ROOT/bin}"',
                     'bin_ = os.path.join(bin_dir, "wsi7")', 'rm -f "$BIN_DIR/wsi7"',
                     'env["WASL_PORT_OFFSET"] = os.environ["PAIRFEAT_PORT_OFFSET"]',
                     '"$BIN_DIR/wsi7"'):
            self.assertIn(want, text)
        with open(os.path.join(HERE, "pairfeat_compare.py")) as h:
            self.assertIn('os.environ.get("PAIRFEAT_REPO"', h.read())
            self.assertIn(want, text)

    def test_parallel_wrapper_unique_ports_and_composite(self):
        with tempfile.TemporaryDirectory() as td:
            exp = os.path.join(td, "exp")
            os.makedirs(exp)
            stub = os.path.join(td, "runner.sh")
            with open(stub, "w") as h:
                h.write("""#!/usr/bin/env bash
set -euo pipefail
mkdir -p "$1/v2-s$PAIRFEAT_SEED"
printf '%s\n' "$WASL_PORT_OFFSET $WASL_BIN" >> "$PAIRFEAT_TEST_LOG"
printf '{"binary_sha256":"same","git_head":"abc"}\n' > "$1/build.json"
""")
            os.chmod(stub, 0o755)
            log = os.path.join(td, "calls.log")
            env = dict(os.environ, PAIRFEAT_RUNNER=stub, PAIRFEAT_TEST_LOG=log,
                       PAIRFEAT_PORT_OFFSET="4000", PAIRFEAT_MAX_JOBS="3")
            subprocess.run(["bash", WRAPPER, "dev-clean", "r1agent12", exp, "1061", "1063", "1069"],
                           check=True, env=env, timeout=30)
            with open(log) as h:
                calls = [line.split() for line in h]
            self.assertEqual(sorted(int(x[0]) for x in calls), [4000, 4100, 4200])
            self.assertEqual(len({x[1] for x in calls}), 3)
            for _, bin_dir in calls:
                self.assertTrue(os.path.islink(os.path.join(bin_dir, "nats-server")))
            self.assertTrue(os.path.isfile(os.path.join(exp, "dev-clean-r1agent12", "build.json")))
            for seed in (1061, 1063, 1069):
                self.assertTrue(os.path.isdir(os.path.join(exp, "dev-clean-r1agent12", "v2-s%d" % seed)))


if __name__ == "__main__":
    unittest.main()
