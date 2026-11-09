from pathlib import Path
import os
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
REQUIRED_FILES = (
    "outback_axs_flavor_contract_test.go",
    "outback_axs_sunspec_observation_test.go",
    "profile_codec_contract_test.go",
    "profile_coherence_contract_test.go",
    "registry_chronology_contract_test.go",
    "completion_record_m303_contract_test.go",
)


class TestTaxonomyPolicy(unittest.TestCase):
    def run_policy(self, missing=None):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "test_taxonomy_contract_test.go").write_bytes(
                (ROOT / "test_taxonomy_contract_test.go").read_bytes()
            )
            for name in REQUIRED_FILES:
                if name != missing:
                    (root / name).write_text("package modbusreg\n", encoding="utf-8")
            if missing != REQUIRED_FILES[-1]:
                (root / REQUIRED_FILES[-1]).write_text(
                    "package modbusreg\nimport \"testing\"\n"
                    "func TestCompletionRecordRetainsEvidence(t *testing.T) {}\n",
                    encoding="utf-8",
                )
            env = dict(os.environ, GO111MODULE="off", GOWORK="off", GOPROXY="off",
                       GOSUMDB="off", GOTOOLCHAIN="local", GOFLAGS="-buildvcs=false")
            return subprocess.run(
                ["go", "test", ".", "-run",
                 "^TestStableTestTaxonomyUsesVendorAndCapabilityNames$",
                 "-count=1", "-timeout=15s", "-v"],
                cwd=root, env=env, capture_output=True, text=True, timeout=25,
            )

    def test_behavior_named_completion_test_satisfies_policy(self):
        result = self.run_policy()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("--- PASS: TestStableTestTaxonomyUsesVendorAndCapabilityNames", result.stdout)

    def test_each_missing_required_file_fails_policy(self):
        for name in REQUIRED_FILES:
            with self.subTest(missing=name):
                result = self.run_policy(missing=name)
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertIn("taxonomy file", result.stdout)
                self.assertIn(name, result.stdout)


if __name__ == "__main__":
    unittest.main()
