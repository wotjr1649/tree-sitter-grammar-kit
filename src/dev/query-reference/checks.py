"""Fail-closed checks shared by reference preparation and owned controls."""
import hashlib
import json
import os
import re
import stat


def read_bounded(path, limit):
    info = path.stat()
    if path.is_symlink() or not stat.S_ISREG(info.st_mode) or info.st_size > limit:
        raise RuntimeError("REFERENCE_FILE_LIMIT_OR_TYPE")
    with path.open("rb") as source:
        info = os.fstat(source.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_size > limit:
            raise RuntimeError("REFERENCE_FILE_LIMIT_OR_TYPE")
        data = source.read(limit + 1)
    if len(data) > limit:
        raise RuntimeError("REFERENCE_FILE_LIMIT_OR_TYPE")
    return data


def file_sha(path, limit=128 * 1024**2):
    info = path.stat()
    if path.is_symlink() or not stat.S_ISREG(info.st_mode) or info.st_size > limit:
        raise RuntimeError("REFERENCE_FILE_LIMIT_OR_TYPE")
    digest, total = hashlib.sha256(), 0
    with path.open("rb") as source:
        info = os.fstat(source.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_size > limit:
            raise RuntimeError("REFERENCE_FILE_LIMIT_OR_TYPE")
        while chunk := source.read(min(1024**2, limit - total + 1)):
            total += len(chunk)
            if total > limit:
                raise RuntimeError("REFERENCE_FILE_LIMIT_OR_TYPE")
            digest.update(chunk)
    return digest.hexdigest()


def build_identity(data):
    value = json.loads(data)
    if value["schema"] != "tsgk-instrumented-reference-build/r1" or value["complete"] is not True:
        raise RuntimeError("REFERENCE_BUILD_INCOMPLETE")
    return value


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
    if read_bounded(path, len(expected)) != expected:
        raise RuntimeError("REFERENCE_INPUT_CHANGED")
