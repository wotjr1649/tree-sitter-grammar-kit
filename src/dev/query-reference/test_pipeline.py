"""Focused integrity regressions; fault processes use the real reference supervisor."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from checks import execution, snapshot, unchanged, unchanged_bytes, unit, read_bounded, file_sha, build_identity


class IntegrityTests(unittest.TestCase):
    def test_input_files_are_bounded_before_reading(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'oversized'
            with path.open('wb') as stream:
                stream.truncate(1024**3)
            for operation in (read_bounded, file_sha):
                with self.assertRaisesRegex(RuntimeError, 'FILE_LIMIT_OR_TYPE'):
                    operation(path, 1024)
                with self.assertRaisesRegex(RuntimeError, 'FILE_LIMIT_OR_TYPE'):
                    operation(Path(directory), 1024)
            path.write_bytes(b'abc')
            self.assertEqual(read_bounded(path, 3), b'abc')
            import hashlib
            self.assertEqual(file_sha(path, 3), hashlib.sha256(b'abc').hexdigest())

    def test_build_complete_is_exact_json_boolean(self):
        for complete in ('false', 'true', 1, 0, False, None):
            data = json.dumps({'schema': 'tsgk-instrumented-reference-build/r1', 'complete': complete})
            with self.assertRaisesRegex(RuntimeError, 'BUILD_INCOMPLETE'):
                build_identity(data)
        self.assertIs(build_identity('{"schema":"tsgk-instrumented-reference-build/r1","complete":true}')['complete'], True)

    def test_selected_unit_must_run(self):
        unit("running 1 test\ntest tests::capture_occurrences_are_complete_and_unique ... ok\ntest result: ok. 1 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out\n")
        for output in ("running 0 tests\ntest result: ok. 0 passed; 0 failed; 0 ignored;", "running 1 test\ntest tests::capture_occurrences_are_complete_and_unique ... ignored\ntest result: ok. 0 passed; 0 failed; 1 ignored;"):
            with self.assertRaisesRegex(RuntimeError, "UNIT_NOT_EXECUTED"):
                unit(output)

    def test_optimized_python_rejected_before_arguments_or_work(self):
        for script in ("build.py", "check.py"):
            for flags, optimize in ((["-O"], ""), ([], "1")):
                with tempfile.TemporaryDirectory() as directory:
                    work = Path(directory) / "must-not-exist"
                    env = dict(os.environ, PYTHONOPTIMIZE=optimize)
                    result = subprocess.run([sys.executable, *flags, str(Path(__file__).with_name(script)), "--work", str(work)], env=env, capture_output=True, timeout=5)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn(b"assertions must be enabled", result.stderr)
                    self.assertNotIn(b"required", result.stderr)
                    self.assertFalse(work.exists())

    def test_snapshot_detects_source_vendor_and_binary_drift(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in ("lib/binding_rust/lib.rs", "Cargo.lock", ".cargo/config.toml", "Cargo.toml", "crate/src/lib.rs", "reference.exe"):
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(b"original")
                before = snapshot(root)
                unchanged(root, before)
                path.write_bytes(b"replaced")
                with self.assertRaisesRegex(RuntimeError, "INPUT_CHANGED"):
                    unchanged(root, before)

    def test_recorded_identity_and_snapshots_keep_validated_bytes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in ('identity.json', 'source-snapshot.json', 'vendor-snapshot.json', 'selected-tree.json'):
                path = root / name
                original = b'{"validated":true}'
                path.write_bytes(original)
                unchanged_bytes(path, original)
                path.write_bytes(b'{}')
                with self.assertRaisesRegex(RuntimeError, 'INPUT_CHANGED'):
                    unchanged_bytes(path, original)

    @unittest.skipUnless(os.environ.get("TSGK_REFERENCE_SUPERVISOR"), "real supervisor not supplied")
    def test_real_negative_exit_timeout_and_output_limit(self):
        supervisor = Path(os.environ["TSGK_REFERENCE_SUPERVISOR"])
        self.assertTrue(supervisor.is_absolute() and supervisor.is_file())
        with tempfile.TemporaryDirectory() as directory:
            env = {key: os.environ[key] for key in ("SystemRoot", "WINDIR", "COMSPEC", "PATHEXT") if key in os.environ}
            env.update(TEMP=directory, TMP=directory, TMPDIR=directory)
            for label, tail, wall in (("exit", "raise SystemExit(1)", 3), ("hang", "import time; time.sleep(30)", 1), ("truncate", "import sys; sys.stderr.write('x'*8192); sys.stderr.flush(); raise SystemExit(1)", 3)):
                spec = {"Path": sys.executable, "Args": ["-I", "-c", 'print(\'{"status":"FAILED","complete":false}\', flush=True); ' + tail], "Dir": directory, "Env": [k + "=" + v for k, v in env.items()], "StdoutBytes": 1024, "StderrBytes": 1024, "Wall": wall * 10**9, "Grace": 10**9, "Memory": {"Bytes": 1024**3, "Hard": sys.platform != "darwin"}, "CgroupParent": os.environ.get("TSGK_REFERENCE_CGROUP_PARENT", "")}
                result = subprocess.run([str(supervisor)], input=json.dumps(spec).encode(), env=env, capture_output=True, timeout=10)
                receipt = json.loads(result.stdout)
                self.assertEqual(result.returncode, 1)
                self.assertTrue(receipt["execution"]["cleanup"]["verified"])
                if label == "exit":
                    execution(receipt, 1)
                else:
                    with self.assertRaisesRegex(RuntimeError, "EXECUTION_INELIGIBLE"):
                        execution(receipt, 1)
                    self.assertTrue(receipt["execution"]["stderr_truncated"] if label == "truncate" else receipt["execution"]["reason"] != "EXITED")


if __name__ == "__main__":
    unittest.main()
