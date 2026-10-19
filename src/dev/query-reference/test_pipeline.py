"""Focused integrity regressions; fault processes use the real reference supervisor."""
import json
import hashlib
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
import shutil

from checks import execution, snapshot, unchanged, unchanged_bytes, unit, read_bounded, file_sha, build_identity


class IntegrityTests(unittest.TestCase):
    def fault_tool(self, directory):
        root = Path(directory)
        source = root / 'tool.c'
        source.write_text(r'''
#include <stdio.h>
#include <string.h>
int main(int argc, char **argv) {
  char root[4096], path[8192], mode[128] = "control";
  if (argc < 2 || strlen(argv[0]) >= sizeof root) return 20;
  strcpy(root, argv[0]);
  char *last = strrchr(root, '/'), *back = strrchr(root, '\\');
  if (back && (!last || back > last)) last = back;
  if (!last) return 21;
  *last = 0;
  snprintf(path, sizeof path, "%s/mode.txt", root);
  FILE *f = fopen(path, "r");
  if (f) { if (!fgets(mode, sizeof mode, f)) return 22; fclose(f); }
  if (!strcmp(argv[1], "-vV")) {
    int rust = strstr(argv[0], "rustc") != NULL;
    const char *release = !strcmp(mode, rust ? "rust-suffix" : "cargo-suffix") ? "1.97.1-custom" :
                          !strcmp(mode, rust ? "rust-ten" : "cargo-ten") ? "1.97.10" : "1.97.1";
    printf("%s 1.97.1 (fixture)\nrelease: %s\n", rust ? "rustc" : "cargo", release);
    return 0;
  }
  if (!strcmp(argv[1], "metadata")) {
    f = fopen("Cargo.lock", "ab"); if (!f) return 23;
    fputs("\n[[package]]\nname = \"tsgk-query-reference\"\nversion = \"0.1.0\"\n", f);
    fclose(f); puts("{}"); return 0;
  }
  if (!strcmp(argv[1], "tree")) {
    if (!strcmp(mode, "source")) strcpy(path, "README.md");
    else if (!strcmp(mode, "vendor")) snprintf(path, sizeof path, "%s/vendor/probe.txt", root);
    else { puts("fixture selected tree"); return 0; }
    f = fopen(path, "ab"); if (!f) return 24;
    fputs("changed", f); fclose(f); puts("fixture selected tree"); return 0;
  }
  if (!strcmp(mode, "oversize")) {
    for (int i = 1; i + 1 < argc; i++) if (!strcmp(argv[i], "-o")) {
      f = fopen(argv[i+1], "wb"); if (!f) return 25;
      if (fseek(f, 64L * 1024 * 1024, SEEK_SET) || fputc(0, f) == EOF) return 26;
      fclose(f); return 0;
    }
  }
  snprintf(path, sizeof path, "%s/later-stage-reached", root);
  f = fopen(path, "wb"); if (!f) return 27; fclose(f); return 9;
}
''', encoding='utf-8')
        binary = root / ('tool.exe' if os.name == 'nt' else 'tool')
        env = {key: os.environ[key] for key in ('SystemRoot', 'WINDIR', 'COMSPEC', 'PATHEXT') if key in os.environ}
        env.update(PATH=str(Path(os.environ['TSGK_REFERENCE_CC']).parent) + os.pathsep + ('C:/Windows/System32;C:/Windows' if os.name == 'nt' else '/usr/bin:/bin'), TEMP=directory, TMP=directory, TMPDIR=directory)
        spec = {'Path': os.environ['TSGK_REFERENCE_CC'], 'Args': [str(source), '-o', str(binary)], 'Dir': directory,
                'Env': [key+'='+value for key,value in env.items()], 'StdoutBytes': 1024**2, 'StderrBytes': 1024**2,
                'Wall': 30*10**9, 'Grace': 2*10**9, 'Memory': {'Bytes': 1024**3, 'Hard': sys.platform != 'darwin'},
                'CgroupParent': os.environ.get('TSGK_REFERENCE_CGROUP_PARENT', '')}
        process = subprocess.run([os.environ['TSGK_REFERENCE_SUPERVISOR']], input=json.dumps(spec).encode(), env=env, capture_output=True, timeout=40)
        self.assertEqual(process.returncode, 0, process.stderr)
        execution(json.loads(process.stdout), 0)
        return binary, env

    @unittest.skipUnless(all(os.environ.get('TSGK_REFERENCE_'+name) for name in ('SUPERVISOR','CC','AR','GIT','ARCHIVE')), 'orchestration tools not supplied')
    def test_actual_build_rejects_tool_versions_and_tree_input_mutation(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            tool, env = self.fault_tool(directory)
            for mode in ('control', 'rust-ten', 'rust-suffix', 'cargo-ten', 'cargo-suffix', 'source', 'vendor', 'archive-swap'):
                with self.subTest(mode=mode):
                    case = root / mode
                    case.mkdir()
                    (case/'vendor').mkdir()
                    (case/'vendor/probe.txt').write_bytes(b'original')
                    (case/'mode.txt').write_text(mode, encoding='ascii')
                    suffix = '.exe' if os.name == 'nt' else ''
                    for name in ('cargo','rustc'):
                        shutil.copyfile(tool, case/(name+suffix))
                        (case/(name+suffix)).chmod(0o700)
                    work = case/'build'
                    entry = Path(__file__).with_name('build.py')
                    archive = Path(os.environ['TSGK_REFERENCE_ARCHIVE'])
                    prefix = [sys.executable, '-B', str(entry)]
                    if mode == 'archive-swap':
                        archive = case/'archive.tar'
                        shutil.copyfile(os.environ['TSGK_REFERENCE_ARCHIVE'], archive)
                        wrapper = case/'swap.py'
                        wrapper.write_text('''import sys, tarfile, runpy
from pathlib import Path
entry = sys.argv.pop(1)
sys.path.insert(0, str(Path(entry).parent))
original = tarfile.open
def swap(*args, **kwargs):
    Path(sys.argv[sys.argv.index('--archive')+1]).write_bytes(b'changed archive')
    return original(*args, **kwargs)
tarfile.open = swap
runpy.run_path(entry, run_name='__main__')
''', encoding='utf-8')
                        prefix = [sys.executable, '-B', str(wrapper), str(entry)]
                    argv = prefix + ['--archive', str(archive),
                            '--vendor', str(case/'vendor'), '--cargo', str(case/('cargo'+suffix)), '--rustc', str(case/('rustc'+suffix)),
                            '--cc', os.environ['TSGK_REFERENCE_CC'], '--ar', os.environ['TSGK_REFERENCE_AR'], '--git', os.environ['TSGK_REFERENCE_GIT'],
                            '--supervisor', os.environ['TSGK_REFERENCE_SUPERVISOR'], '--work', str(work), '--target', {'win32':'x86_64-pc-windows-gnu','darwin':'aarch64-apple-darwin'}.get(sys.platform,'x86_64-unknown-linux-gnu')]
                    if os.environ.get('TSGK_REFERENCE_CGROUP_PARENT'):
                        argv += ['--cgroup-parent', os.environ['TSGK_REFERENCE_CGROUP_PARENT']]
                    result = subprocess.run(argv, env=env, capture_output=True, timeout=60)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertFalse((work/'identity.json').exists())
                    if mode in ('control', 'archive-swap'):
                        self.assertTrue((case/'later-stage-reached').exists(), result.stderr)
                        self.assertTrue((work/'build-request.json').exists())
                    else:
                        expected = b'REFERENCE_INPUT_CHANGED' if mode in ('source','vendor') else b'REFERENCE_TOOL_VERSION'
                        self.assertIn(expected, result.stderr)
                        self.assertFalse((case/'later-stage-reached').exists())
                        self.assertFalse((work/('build-request.json' if mode in ('source','vendor') else 'metadata-request.json')).exists())

    @unittest.skipUnless(all(os.environ.get('TSGK_REFERENCE_'+name) for name in ('SUPERVISOR','CC')), 'orchestration tools not supplied')
    def test_actual_controls_bound_compiler_library_before_reference(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            tool, env = self.fault_tool(directory)
            (root/'mode.txt').write_text('oversize', encoding='ascii')
            selected = root/'selected-tree.json'
            selected.write_bytes(b'{}')
            here = Path(__file__).resolve().parent
            sha = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()
            identity = {'schema':'tsgk-instrumented-reference-build/r1','complete':True,'binary_sha256':sha(tool),
                        'upstream_commit':'10eb5ac119eec9cb4267a8e035066473983f5f3a', 'selected_tree_sha256':sha(selected),
                        'inputs':{name:sha(here/name) for name in ('Cargo.toml','main.rs','instrumentation.patch','build.py','checks.py')}}
            identity_path = root/'identity.json'
            identity_path.write_text(json.dumps(identity), encoding='utf-8')
            work = root/'controls'
            argv = [sys.executable, '-B', str(here/'check.py'), '--reference', str(tool), '--identity', str(identity_path),
                    '--supervisor', os.environ['TSGK_REFERENCE_SUPERVISOR'], '--cc', str(tool), '--work', str(work)]
            if os.environ.get('TSGK_REFERENCE_CGROUP_PARENT'):
                argv += ['--cgroup-parent', os.environ['TSGK_REFERENCE_CGROUP_PARENT']]
            result = subprocess.run(argv, env=env, capture_output=True, timeout=60)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(b'REFERENCE_FILE_LIMIT_OR_TYPE', result.stderr)
            self.assertFalse((work/'structural-0.json').exists())
            self.assertFalse((root/'later-stage-reached').exists())

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
