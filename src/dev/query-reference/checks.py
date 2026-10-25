"""Fail-closed checks shared by reference preparation and owned controls."""
import base64
import binascii
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


def streams(receipt):
    try:
        if (set(receipt) != {'schema', 'execution', 'stdout_base64', 'stderr_base64'}
                or receipt['schema'] != 'tsgk-reference-process/r1'):
            raise ValueError('schema')
        result = []
        for name in ('stdout', 'stderr'):
            encoded = receipt[name + '_base64']
            if not isinstance(encoded, str):
                raise ValueError('base64 type')
            raw = base64.b64decode(encoded, validate=True)
            count = receipt['execution'][name + '_bytes']
            truncated = receipt['execution'][name + '_truncated']
            if (base64.b64encode(raw).decode('ascii') != encoded
                    or type(count) is not int or count < len(raw)
                    or type(truncated) is not bool or (not truncated and count != len(raw))):
                raise ValueError('stream identity')
            result.append(raw)
        return tuple(result)
    except (ValueError, binascii.Error, KeyError, TypeError) as error:
        raise RuntimeError('REFERENCE_STREAM_INVALID') from error


def stream_text(receipt, name='stdout'):
    try:
        return streams(receipt)[('stdout', 'stderr').index(name)].decode('utf-8')
    except UnicodeDecodeError as error:
        raise RuntimeError('REFERENCE_STREAM_UTF8') from error


def execution(receipt, expected_exit):
    streams(receipt)
    result = receipt["execution"]
    if (not result["cleanup"]["verified"] or result["cleanup"]["residual_after_exit"] != 0
            or result["stdout_truncated"] or result["stderr_truncated"]
            or result["reason"] != "EXITED" or result["exit_code"] != expected_exit
            or result["status"] != ("COMPLETED" if expected_exit == 0 else "FAILED")):
        raise RuntimeError("REFERENCE_EXECUTION_INELIGIBLE")


def unit(stdout):
    names = ("capture_occurrences_are_complete_and_unique", "batch_requests_preserve_order_and_bounds",
             "batch_budget_charges_common_work_and_caps_total_wall", "watchdog_exits_during_blocking_work_and_expired_rearm")
    if (not re.search(r"^running 4 tests$", stdout, re.M)
            or any(not re.search(r"^test tests::" + name + r" \.\.\. ok$", stdout, re.M) for name in names)
            or not re.search(r"^test result: ok\. 4 passed; 0 failed; 0 ignored;", stdout, re.M)):
        raise RuntimeError("REFERENCE_UNIT_NOT_EXECUTED")


def batch(stdout, expected_ids):
    def unique(pairs):
        value = {}
        for key, item in pairs:
            if key in value:
                raise RuntimeError("REFERENCE_BATCH_INVALID")
            value[key] = item
        return value

    if (not 1 <= len(expected_ids) <= 16 or len(set(expected_ids)) != len(expected_ids)
            or len(stdout) > 64 * 1024**2 or len(stdout.encode('utf-8')) > 64 * 1024**2
            or not stdout.endswith('\n')):
        raise RuntimeError("REFERENCE_BATCH_INVALID")
    lines = stdout[:-1].split('\n')
    if len(lines) != len(expected_ids) + 1:
        raise RuntimeError("REFERENCE_BATCH_INCOMPLETE")
    try:
        values = [json.loads(line, object_pairs_hook=unique) for line in lines]
        footer = values[-1]
        if (set(footer) != {'schema', 'status', 'complete', 'queries', 'responses_completed'}
                or footer['schema'] != 'tsgk-instrumented-query-reference-batch/r1'
                or footer['status'] != 'COMPLETED' or footer['complete'] is not True
                or footer['queries'] != expected_ids or type(footer['responses_completed']) is not int
                or footer['responses_completed'] != len(expected_ids)):
            raise RuntimeError("REFERENCE_BATCH_INCOMPLETE")
        for query_id, value in zip(expected_ids, values[:-1]):
            if (value['schema'] != 'tsgk-instrumented-query-reference/r1'
                    or value['status'] != 'COMPLETED' or value['complete'] is not True
                    or value['query']['id'] != query_id):
                raise RuntimeError("REFERENCE_BATCH_INCOMPLETE")
    except (ValueError, KeyError, TypeError, RecursionError) as error:
        raise RuntimeError("REFERENCE_BATCH_INVALID") from error
    return values[:-1]


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
