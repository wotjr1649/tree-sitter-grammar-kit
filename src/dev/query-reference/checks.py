"""Fail-closed checks shared by reference preparation and owned controls."""
import hashlib
import re


def execution(receipt, expected_exit):
    result = receipt["execution"]
    if (not result["cleanup"]["verified"] or result["cleanup"]["residual_after_exit"] != 0
            or result["stdout_truncated"] or result["stderr_truncated"]
            or result["reason"] != "EXITED" or result["exit_code"] != expected_exit
            or result["status"] != ("COMPLETED" if expected_exit == 0 else "FAILED")):
        raise RuntimeError("REFERENCE_EXECUTION_INELIGIBLE")


def unit(stdout):
    if (not re.search(r"^running 1 test$", stdout, re.M)
            or not re.search(r"^test tests::capture_occurrences_are_complete_and_unique \.\.\. ok$", stdout, re.M)
            or not re.search(r"^test result: ok\. 1 passed; 0 failed; 0 ignored;", stdout, re.M)):
        raise RuntimeError("REFERENCE_UNIT_NOT_EXECUTED")


def snapshot(directory):
    files = {}
    total = 0
    for path in sorted(directory.rglob("*")):
        if path.is_symlink():
            raise RuntimeError("REFERENCE_INPUT_SYMLINK")
        if not path.is_file():
            continue
        size = path.stat().st_size
        total += size
        if size > 64 * 1024**2 or total > 1024**3 or len(files) >= 30000:
            raise RuntimeError("REFERENCE_INPUT_LIMIT")
        digest = hashlib.sha256()
        with path.open("rb") as source:
            while chunk := source.read(1024**2):
                digest.update(chunk)
        files[path.relative_to(directory).as_posix()] = {"bytes": size, "sha256": digest.hexdigest()}
    return files


def unchanged(directory, expected):
    if snapshot(directory) != expected:
        raise RuntimeError("REFERENCE_INPUT_CHANGED")


def unchanged_bytes(path, expected):
    if path.read_bytes() != expected:
        raise RuntimeError("REFERENCE_INPUT_CHANGED")
