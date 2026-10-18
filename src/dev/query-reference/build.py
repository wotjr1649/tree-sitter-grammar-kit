"""Build the pinned reference from reviewed local inputs, without downloading or installing."""
import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import subprocess
import sys
import tarfile
import tomllib
from checks import execution, snapshot, unchanged, unchanged_bytes, unit, read_bounded, file_sha as sha

UPSTREAM = "10eb5ac119eec9cb4267a8e035066473983f5f3a"
ARCHIVE = "6025ffd838c63bb011eeec9b259891ae0e051a3db9aa24c8812679f7462a50a9"
BINDING = "3a350f2c4735d3a07a0892547afff9f34cabe1e63f962e2d4f26e1b7db8836c2"
TARGETS = ("x86_64-pc-windows-gnu", "x86_64-unknown-linux-gnu", "aarch64-apple-darwin")



def pinned_release(output):
    releases = []
    for line in output.splitlines():
        key, separator, value = line.partition(":")
        if separator and key.strip() == "release":
            releases.append(value.strip())
    if releases != ["1.97.1"]:
        raise RuntimeError("REFERENCE_TOOL_VERSION")


def main():
    if sys.flags.optimize:
        raise RuntimeError("assertions must be enabled")
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("archive", "vendor", "cargo", "rustc", "cc", "ar", "git", "supervisor", "work"):
        parser.add_argument("--" + name, type=Path, required=True)
    parser.add_argument("--target", choices=TARGETS, required=True)
    parser.add_argument("--cgroup-parent", default="")
    args = parser.parse_args()
    for name in ("archive", "vendor", "cargo", "rustc", "cc", "ar", "git", "supervisor", "work"):
        path = getattr(args, name)
        assert path.is_absolute(), name
        if name != "work":
            assert path.exists() and not path.is_symlink(), name
    assert args.archive.stat().st_size <= 128 * 1024**2 and sha(args.archive) == ARCHIVE
    args.work.mkdir(exist_ok=False)
    source = args.work / "source"
    source.mkdir()
    (args.work / "tmp").mkdir()
    environment = {key: os.environ[key] for key in ("SystemRoot", "WINDIR", "COMSPEC", "PATHEXT", "SystemDrive") if key in os.environ}
    system_path = "C:/Windows/System32;C:/Windows" if os.name == "nt" else "/usr/bin:/bin"
    environment.update({"PATH": os.pathsep.join((str(args.cc.parent), str(args.rustc.parent), system_path)),
        "TEMP": str(args.work / "tmp"), "TMP": str(args.work / "tmp"), "TMPDIR": str(args.work / "tmp"),
        "CARGO_HOME": str(args.work / "cargo-home"), "CARGO_TARGET_DIR": str(args.work / "target"),
        "CARGO_NET_OFFLINE": "true", "CARGO_TERM_COLOR": "never", "CARGO_INCREMENTAL": "0", "RUSTC": str(args.rustc),
        "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull, "GIT_CEILING_DIRECTORIES": str(args.work),
        "CC_" + args.target.replace("-", "_"): str(args.cc), "AR_" + args.target.replace("-", "_"): str(args.ar),
        "CARGO_TARGET_" + args.target.upper().replace("-", "_") + "_LINKER": str(args.cc)})

    def execute(name, program, argv, wall=120, cwd=source):
        spec = {"Path": str(program), "Args": list(map(str, argv)), "Dir": str(cwd), "Env": [key + "=" + value for key, value in sorted(environment.items())],
            "StdoutBytes": 16 * 1024**2, "StderrBytes": 4 * 1024**2, "Wall": wall * 10**9, "Grace": 5 * 10**9,
            "Memory": {"Bytes": 4 * 1024**3, "Hard": sys.platform != "darwin"}, "CgroupParent": args.cgroup_parent}
        (args.work / (name + "-request.json")).write_text(json.dumps(spec, indent=2), encoding="utf-8")
        with (args.work / (name + ".json")).open("xb") as out, (args.work / (name + ".stderr")).open("xb") as err:
            result = subprocess.run([str(args.supervisor)], input=json.dumps(spec).encode(), env=environment, stdout=out, stderr=err, timeout=wall + 20)
        saved = (args.work / (name + ".json")).read_bytes()
        assert saved, (name, (args.work / (name + ".stderr")).read_text(encoding="utf-8")[-4000:])
        receipt = json.loads(saved)
        execution(receipt, 0)
        assert result.returncode == 0 and receipt["execution"]["cleanup"]["verified"], (name, receipt["stderr"][-4000:])
        assert not receipt["execution"]["stdout_truncated"] and not receipt["execution"]["stderr_truncated"]
        return receipt["stdout"]

    total = count = 0
    with tarfile.open(args.archive, "r:") as archive:
        for member in archive:
            path = PurePosixPath(member.name)
            assert not path.is_absolute() and ".." not in path.parts and all(":" not in part and "\\" not in part for part in path.parts)
            if member.isdir():
                continue
            assert member.isfile() and member.size <= 64 * 1024**2
            count += 1
            total += member.size
            assert count <= 30000 and total <= 256 * 1024**2
            target = source / Path(*path.parts)
            assert target.resolve().is_relative_to(source.resolve())
            target.parent.mkdir(parents=True, exist_ok=True)
            with target.open("xb") as out:
                out.write(archive.extractfile(member).read(member.size))
    for parent in args.work.parents:
        assert not (parent / ".cargo/config").exists() and not (parent / ".cargo/config.toml").exists(), "ancestor Cargo configuration"
    here = Path(__file__).resolve().parent
    input_hashes = {name: sha(here / name) for name in ("Cargo.toml", "main.rs", "instrumentation.patch", "build.py", "checks.py")}
    tools = {name: getattr(args, name) for name in ("cargo", "rustc", "cc", "ar", "git", "supervisor")}
    tools["python"] = Path(sys.executable).resolve(strict=True)
    tool_hashes = {name: sha(path) for name, path in tools.items()}
    assert sha(source / "lib/binding_rust/lib.rs") == "1837935ea37e0d8bde7c4e6daab9e6020047b7d7af40fbc55eceb6c982403b7a"
    execute("patch-check", args.git, ["apply", "--unidiff-zero", "--check", here / "instrumentation.patch"])
    execute("patch", args.git, ["apply", "--unidiff-zero", here / "instrumentation.patch"])
    assert sha(source / "lib/binding_rust/lib.rs") == BINDING
    manifest = source / "Cargo.toml"
    text = manifest.read_text(encoding="utf-8")
    assert text.count("members = [\n") == 1
    manifest.write_text(text.replace("members = [\n", 'members = [\n  "crates/tsgk-query-reference",\n'), encoding="utf-8", newline="")
    package = source / "crates/tsgk-query-reference"
    package.mkdir()
    for name in ("Cargo.toml", "main.rs"):
        (package / name).write_bytes((here / name).read_bytes())
    config = source / ".cargo/config.toml"
    config.write_text(config.read_text(encoding="utf-8") + '\n[source.crates-io]\nreplace-with = "tsgk-vendored"\n[source.tsgk-vendored]\ndirectory = ' + json.dumps(str(args.vendor)) + "\n", encoding="utf-8", newline="")
    original_lock = tomllib.loads((source / "Cargo.lock").read_text(encoding="utf-8"))
    versions = {"rustc": execute("rustc-version", args.rustc, ["-vV"]), "cargo": execute("cargo-version", args.cargo, ["-vV"]), "cc": execute("cc-version", args.cc, ["--version"]), "python": sys.version}
    for name in ("rustc", "cargo"):
        pinned_release(versions[name])
    metadata = execute("metadata", args.cargo, ["metadata", "--offline", "--format-version", "1", "--filter-platform", args.target])
    effective_lock = tomllib.loads((source / "Cargo.lock").read_text(encoding="utf-8"))
    before = {(p["name"], p["version"]): p for p in original_lock["package"]}
    after = {(p["name"], p["version"]): p for p in effective_lock["package"]}
    assert set(after) - set(before) == {("tsgk-query-reference", "0.1.0")} and all(after.get(key) == value for key, value in before.items())
    source_snapshot, vendor_snapshot = snapshot(source), snapshot(args.vendor)
    snapshot_bytes = {"source-snapshot.json": json.dumps(source_snapshot, sort_keys=True).encode(),
        "vendor-snapshot.json": json.dumps(vendor_snapshot, sort_keys=True).encode()}
    for name, data in snapshot_bytes.items():
        (args.work / name).write_bytes(data)
    execute("selected-tree", args.cargo, ["tree", "--offline", "--frozen", "--package", "tsgk-query-reference", "--target", args.target, "--edges", "normal,build", "--prefix", "none", "--format", "{p}|{f}"])
    unchanged(source, source_snapshot)
    unchanged(args.vendor, vendor_snapshot)
    selected_tree = read_bounded(args.work / "selected-tree.json", 4 * 1024**2)
    execute("build", args.cargo, ["build", "--offline", "--frozen", "--release", "--package", "tsgk-query-reference", "--target", args.target, "--jobs", "2"], wall=900)
    unit(execute("unit", args.cargo, ["test", "--offline", "--frozen", "--release", "--package", "tsgk-query-reference", "--target", args.target, "--jobs", "2", "--bin", "tsgk-query-reference", "--", "--exact", "tests::capture_occurrences_are_complete_and_unique"], wall=120))
    binary = args.work / "target" / args.target / "release" / ("tsgk-query-reference.exe" if os.name == "nt" else "tsgk-query-reference")
    assert all(sha(here / name) == digest for name, digest in input_hashes.items()), "reference inputs changed during build"
    assert all(sha(tools[name]) == digest for name, digest in tool_hashes.items()), "tool changed during build"
    unchanged_bytes(args.work / "selected-tree.json", selected_tree)
    assert all(sha(package / name) == input_hashes[name] for name in ("Cargo.toml", "main.rs"))
    unchanged(source, source_snapshot)
    unchanged(args.vendor, vendor_snapshot)
    for name, data in snapshot_bytes.items():
        unchanged_bytes(args.work / name, data)
    assert sha(source / "lib/binding_rust/lib.rs") == BINDING
    identity = {"schema": "tsgk-instrumented-reference-build/r1", "upstream_commit": UPSTREAM, "archive_sha256": ARCHIVE, "binding_sha256": BINDING, "target": args.target,
        "binary_sha256": sha(binary), "lock_sha256": sha(source / "Cargo.lock"), "metadata_sha256": hashlib.sha256(metadata.encode()).hexdigest(), "versions": versions,
        "inputs": input_hashes, "tools": tool_hashes, "tool_paths": {name: str(path) for name, path in tools.items()},
        "selected_tree_sha256": hashlib.sha256(selected_tree).hexdigest(), "source_snapshot_sha256": hashlib.sha256(snapshot_bytes["source-snapshot.json"]).hexdigest(),
        "vendor_snapshot_sha256": hashlib.sha256(snapshot_bytes["vendor-snapshot.json"]).hexdigest(), "complete": True}
    (args.work / "identity.json").write_text(json.dumps(identity, indent=2), encoding="utf-8")
    print(json.dumps({"schema": identity["schema"], "binary_sha256": identity["binary_sha256"], "complete": True}))


if __name__ == "__main__":
    main()
