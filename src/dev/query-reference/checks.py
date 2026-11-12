"""Fail-closed checks shared by reference preparation and owned controls."""
import base64
import binascii
import hashlib
import json
import math
import os
import re
import stat
from pathlib import Path, PurePosixPath

PIN_SELECTION_SCHEMA = "tsgk-evidence-pin-selection/r1"
PIN_SET_SCHEMA = "tsgk-evidence-pin-set/r1"
PIN_READER = "tsgk-evidence-materials/r1"
PIN_CONTEXT_SCHEMA = "tsgk-reference-production-context/r1"
PIN_TUPLE_SCHEMA = "tsgk-reference-pin-tuple/r1"
PIN_PRODUCTION_SCHEMA = "tsgk-reference-production/r1"
PIN_PATH_READER = "rust-path-1.97.1/r1"
PIN_OBJECT_BYTES = 16 * 1024**2
PIN_TOTAL_BYTES = 256 * 1024**2
PIN_JSON_DEPTH = 32
PIN_JSON_VALUES = 2_000_000
PIN_DIGEST = re.compile(r"[a-f0-9]{64}\Z")
PIN_RELEASE = re.compile(r"[0-9]+\.[0-9]+\.[0-9]+\Z")


def _unique_object(pairs):
    value = {}
    for key, item in pairs:
        if key in value:
            raise RuntimeError("REFERENCE_PIN_JSON_DUPLICATE")
        value[key] = item
    return value


def _json(data):
    def reject_constant(value):
        raise ValueError(value)

    def finite_float(value):
        result = float(value)
        if not math.isfinite(result):
            raise ValueError("non-finite number")
        return result

    try:
        value = json.loads(data.decode("utf-8"), object_pairs_hook=_unique_object,
                           parse_constant=reject_constant, parse_float=finite_float)
    except (UnicodeError, ValueError, RecursionError) as error:
        raise RuntimeError("REFERENCE_PIN_JSON_INVALID") from error
    pending, count = [(value, 1)], 0
    while pending:
        item, depth = pending.pop()
        count += 1
        if depth > PIN_JSON_DEPTH or count > PIN_JSON_VALUES:
            raise RuntimeError("REFERENCE_PIN_JSON_LIMIT")
        if isinstance(item, dict):
            pending.extend((child, depth + 1) for child in item.values())
        elif isinstance(item, list):
            pending.extend((child, depth + 1) for child in item)
    return value


def _pin_sha(data):
    return hashlib.sha256(data).hexdigest()


def pinned_release(output, expected):
    releases = []
    for line in output.splitlines():
        key, separator, value = line.partition(":")
        if separator and key.strip() == "release":
            releases.append(value.strip())
    if releases != [expected]:
        raise RuntimeError("REFERENCE_TOOL_VERSION")


def _pin_file(path, limit):
    path = Path(path)
    if path.is_symlink():
        raise RuntimeError("REFERENCE_PIN_FILE_TYPE")
    return read_bounded(path, limit)


def _pin_object_name(name):
    path = PurePosixPath(name) if isinstance(name, str) else None
    return bool(path and path.parts and not path.is_absolute() and str(path) == name
                and all(part not in (".", "..") and ":" not in part and "\\" not in part
                        and not any(ord(char) < 32 for char in part) for part in path.parts))


def _validate_pin_data(selection_raw, set_raw, material_raw, repo_root):
    selection = _json(selection_raw)
    if (not isinstance(selection, dict) or set(selection) != {"schema", "pin_set_sha256"}
            or selection["schema"] != PIN_SELECTION_SCHEMA
            or not isinstance(selection["pin_set_sha256"], str)
            or not PIN_DIGEST.fullmatch(selection["pin_set_sha256"])
            or _pin_sha(set_raw) != selection["pin_set_sha256"]):
        raise RuntimeError("REFERENCE_PIN_SELECTION_INVALID")
    pin_set = _json(set_raw)
    if (not isinstance(pin_set, dict) or set(pin_set) != {"schema", "reader", "materials"}
            or pin_set["schema"] != PIN_SET_SCHEMA or pin_set["reader"] != PIN_READER
            or not isinstance(pin_set["materials"], list) or not pin_set["materials"]
            or len(pin_set["materials"]) > 10000):
        raise RuntimeError("REFERENCE_PIN_SET_INVALID")
    descriptors, seen, total = {}, set(), 0
    roles = {"reference-catalogue", "reference-input", "reference-tuple", "runtime-manifest",
             "runtime-patch", "driver", "cp949", "shim-template", "roslyn-observation", "roslyn-cases"}
    for item in pin_set["materials"]:
        if (not isinstance(item, dict) or set(item) != {"name", "role", "bytes", "sha256"}
                or not _pin_object_name(item["name"]) or not isinstance(item["role"], str) or item["role"] not in roles
                or type(item["bytes"]) is not int or item["bytes"] < 0 or item["bytes"] > PIN_OBJECT_BYTES
                or not isinstance(item["sha256"], str) or not PIN_DIGEST.fullmatch(item["sha256"])):
            raise RuntimeError("REFERENCE_PIN_DESCRIPTOR_INVALID")
        key, data = item["name"].casefold(), material_raw.get(item["name"])
        if key in seen or data is None:
            raise RuntimeError("REFERENCE_PIN_DESCRIPTOR_DUPLICATE_OR_MISSING")
        seen.add(key)
        total += len(data)
        if total > PIN_TOTAL_BYTES or len(data) != item["bytes"] or _pin_sha(data) != item["sha256"]:
            raise RuntimeError("REFERENCE_PIN_MATERIAL_MISMATCH")
        descriptors[item["name"]] = item
    if set(material_raw) != set(descriptors):
        raise RuntimeError("REFERENCE_PIN_MATERIAL_UNUSED")

    required = {"reference/catalogue.json": "reference-catalogue",
                "reference/tuple.json": "reference-tuple",
                "reference/production.json": "reference-input"}
    if any(name not in descriptors or descriptors[name]["role"] != role for name, role in required.items()):
        raise RuntimeError("REFERENCE_PIN_REQUIRED_MATERIAL")
    catalogue_raw = material_raw["reference/catalogue.json"]
    tuple_raw = material_raw["reference/tuple.json"]
    production_raw = material_raw["reference/production.json"]
    catalogue, toolchain, production = _json(catalogue_raw), _json(tuple_raw), _json(production_raw)
    if (not isinstance(catalogue, dict) or catalogue.get("schema") != "tsgk-query-reference-eligibility-catalogue/r1"
            or not isinstance(toolchain, dict)
            or set(toolchain) != {"schema", "rustc_release", "cargo_release", "path_reader", "catalogue_sha256"}
            or toolchain.get("schema") != PIN_TUPLE_SCHEMA
            or not all(isinstance(toolchain.get(key), str) and PIN_RELEASE.fullmatch(toolchain[key])
                       for key in ("rustc_release", "cargo_release"))
            or toolchain.get("path_reader") != PIN_PATH_READER
            or not isinstance(production, dict)
            or set(production) != {"schema", "catalogue_sha256", "tuple_sha256", "acquire_sha256", "vendor_snapshot_sha256"}
            or production.get("schema") != PIN_PRODUCTION_SCHEMA):
        raise RuntimeError("REFERENCE_PIN_PRODUCTION_INVALID")
    catalogue_sha, tuple_sha = _pin_sha(catalogue_raw), _pin_sha(tuple_raw)
    if (toolchain["catalogue_sha256"] != catalogue_sha or production["catalogue_sha256"] != catalogue_sha
            or production["tuple_sha256"] != tuple_sha
            or not all(isinstance(production.get(key), str) and PIN_DIGEST.fullmatch(production[key])
                       for key in ("acquire_sha256", "vendor_snapshot_sha256"))):
        raise RuntimeError("REFERENCE_PIN_PRODUCTION_BINDING")
    if (not re.fullmatch(r"[a-f0-9]{40}", str(catalogue.get("upstream_commit", "")))
            or not all(isinstance(catalogue.get(key), str) and PIN_DIGEST.fullmatch(catalogue[key])
                       for key in ("archive_sha256", "binding_sha256"))
            or not isinstance(catalogue.get("reference_inputs"), dict)
            or set(catalogue["reference_inputs"]) != {"Cargo.toml", "main.rs", "instrumentation.patch", "build.py", "checks.py"}
            or not isinstance(catalogue.get("controller_sha256"), dict)
            or set(catalogue["controller_sha256"]) != {"build.py", "check.py", "checks.py"}):
        raise RuntimeError("REFERENCE_PIN_CATALOGUE_INVALID")
    source_hashes = dict(catalogue["reference_inputs"])
    for name, digest in catalogue["controller_sha256"].items():
        if name in source_hashes and source_hashes[name] != digest:
            raise RuntimeError("REFERENCE_PIN_CATALOGUE_CONTROLLER_MISMATCH")
        source_hashes[name] = digest
    source_hashes["acquire.py"] = production["acquire_sha256"]
    source_dir = Path(repo_root) / "src/dev/query-reference"
    for name, digest in source_hashes.items():
        descriptor = descriptors.get("reference/" + name)
        actual = _pin_sha(_pin_file(source_dir / name, PIN_OBJECT_BYTES))
        if (not isinstance(digest, str) or not PIN_DIGEST.fullmatch(digest) or not descriptor
                or descriptor["role"] != "reference-input" or descriptor["sha256"] != digest or actual != digest):
            raise RuntimeError("REFERENCE_PIN_SOURCE_MISMATCH")
    if production["acquire_sha256"] != descriptors["reference/acquire.py"]["sha256"]:
        raise RuntimeError("REFERENCE_PIN_ACQUIRE_MISMATCH")
    return selection, pin_set, catalogue, toolchain, production


def _pin_context(selection_raw, set_raw, material_raw):
    catalogue = _json(material_raw["reference/catalogue.json"])
    toolchain = _json(material_raw["reference/tuple.json"])
    production = _json(material_raw["reference/production.json"])
    return {"schema": PIN_CONTEXT_SCHEMA, "selection_sha256": _pin_sha(selection_raw),
            "selection": base64.b64encode(selection_raw).decode("ascii"),
            "pin_set_sha256": _pin_sha(set_raw), "pin_set": base64.b64encode(set_raw).decode("ascii"),
            "materials": {name: base64.b64encode(data).decode("ascii") for name, data in material_raw.items()},
            "catalogue": catalogue, "tuple": toolchain, "production": production}


def resolve_pin_context(selection_path, repo_root, output_path=None):
    contracts = Path(selection_path).parent
    selection_raw = _pin_file(selection_path, 1024**2)
    selection = _json(selection_raw)
    if (not isinstance(selection, dict) or set(selection) != {"schema", "pin_set_sha256"}
            or selection.get("schema") != PIN_SELECTION_SCHEMA
            or not isinstance(selection.get("pin_set_sha256"), str) or not PIN_DIGEST.fullmatch(selection["pin_set_sha256"])):
        raise RuntimeError("REFERENCE_PIN_SELECTION_INVALID")
    set_raw = _pin_file(contracts / "sets" / (selection["pin_set_sha256"] + ".json"), 1024**2)
    if _pin_sha(set_raw) != selection["pin_set_sha256"]:
        raise RuntimeError("REFERENCE_PIN_SET_DIGEST")
    pin_set = _json(set_raw)
    if (not isinstance(pin_set, dict) or not isinstance(pin_set.get("materials"), list)
            or pin_set.get("schema") != PIN_SET_SCHEMA or pin_set.get("reader") != PIN_READER
            or len(pin_set["materials"]) > 10000):
        raise RuntimeError("REFERENCE_PIN_SET_INVALID")
    names, total = set(), 0
    roles = {"reference-catalogue", "reference-input", "reference-tuple", "runtime-manifest",
             "runtime-patch", "driver", "cp949", "shim-template", "roslyn-observation", "roslyn-cases"}
    for item in pin_set["materials"]:
        if (not isinstance(item, dict) or set(item) != {"name", "role", "bytes", "sha256"}
                or not _pin_object_name(item["name"]) or not isinstance(item["role"], str) or item["role"] not in roles
                or type(item["bytes"]) is not int or item["bytes"] < 0 or item["bytes"] > PIN_OBJECT_BYTES
                or not isinstance(item["sha256"], str) or not PIN_DIGEST.fullmatch(item["sha256"])):
            raise RuntimeError("REFERENCE_PIN_DESCRIPTOR_INVALID")
        key = item["name"].casefold()
        total += item["bytes"]
        if key in names or total > PIN_TOTAL_BYTES:
            raise RuntimeError("REFERENCE_PIN_DESCRIPTOR_DUPLICATE_OR_LIMIT")
        names.add(key)
    material_raw = {}
    for item in pin_set["materials"]:
        material_raw[item["name"]] = _pin_file(contracts / "objects" / item["sha256"], item["bytes"])
    _validate_pin_data(selection_raw, set_raw, material_raw, repo_root)
    context = _pin_context(selection_raw, set_raw, material_raw)
    raw = json.dumps(context, sort_keys=True, separators=(",", ":")).encode("utf-8")
    if output_path is not None:
        with Path(output_path).open("xb") as output:
            output.write(raw)
    return context, raw


def _decode_pin_context(raw, expected_sha256, repo_root):
    if not expected_sha256 or not PIN_DIGEST.fullmatch(expected_sha256):
        raise RuntimeError("REFERENCE_PIN_CONTEXT_REQUIRED")
    if _pin_sha(raw) != expected_sha256:
        raise RuntimeError("REFERENCE_PIN_CONTEXT_MISMATCH")
    value = _json(raw)
    keys = {"schema", "selection_sha256", "selection", "pin_set_sha256", "pin_set", "materials",
            "catalogue", "tuple", "production"}
    if not isinstance(value, dict) or set(value) != keys or value["schema"] != PIN_CONTEXT_SCHEMA or not isinstance(value["materials"], dict):
        raise RuntimeError("REFERENCE_PIN_CONTEXT_INVALID")
    try:
        selection_raw = base64.b64decode(value["selection"], validate=True)
        set_raw = base64.b64decode(value["pin_set"], validate=True)
        material_raw = {name: base64.b64decode(data, validate=True) for name, data in value["materials"].items()}
    except (ValueError, TypeError, binascii.Error) as error:
        raise RuntimeError("REFERENCE_PIN_CONTEXT_INVALID") from error
    if (base64.b64encode(selection_raw).decode("ascii") != value["selection"]
            or base64.b64encode(set_raw).decode("ascii") != value["pin_set"]
            or _pin_sha(selection_raw) != value["selection_sha256"] or _pin_sha(set_raw) != value["pin_set_sha256"]):
        raise RuntimeError("REFERENCE_PIN_CONTEXT_MISMATCH")
    selection, _, catalogue, toolchain, production = _validate_pin_data(selection_raw, set_raw, material_raw, repo_root)
    if (selection["pin_set_sha256"] != value["pin_set_sha256"] or value["catalogue"] != catalogue
            or value["tuple"] != toolchain or value["production"] != production):
        raise RuntimeError("REFERENCE_PIN_CONTEXT_MISMATCH")
    return {"selection_sha256": value["selection_sha256"], "pin_set_sha256": value["pin_set_sha256"],
            "catalogue": catalogue, "tuple": toolchain, "production": production}


def load_pin_context(context_path, expected_sha256, repo_root):
    if not context_path:
        raise RuntimeError("REFERENCE_PIN_CONTEXT_REQUIRED")
    return _decode_pin_context(_pin_file(context_path, 16 * 1024**2), expected_sha256, repo_root)


def production_pins(repo_root, context_path=None, expected_sha256=None, fallback_path=None):
    explicit = context_path is not None or expected_sha256 is not None
    if explicit:
        if context_path is None or expected_sha256 is None:
            raise RuntimeError("REFERENCE_PIN_CONTEXT_REQUIRED")
        raw = _pin_file(context_path, 16 * 1024**2)
        return _decode_pin_context(raw, expected_sha256, repo_root), raw, True
    if fallback_path is not None and Path(fallback_path).is_file():
        raw = _pin_file(fallback_path, 16 * 1024**2)
        return _decode_pin_context(raw, _pin_sha(raw), repo_root), raw, False
    context, raw = resolve_pin_context(Path(repo_root) / "src/contracts/evidence-pins/selection.json", repo_root)
    return {"selection_sha256": context["selection_sha256"], "pin_set_sha256": context["pin_set_sha256"],
            "catalogue": context["catalogue"], "tuple": context["tuple"], "production": context["production"]}, raw, False


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


def process_receipt(data):
    def unique(pairs):
        value = {}
        for key, item in pairs:
            if key in value:
                raise RuntimeError('REFERENCE_RECEIPT_INVALID')
            value[key] = item
        return value

    try:
        return json.loads(data.decode('utf-8'), object_pairs_hook=unique)
    except (ValueError, RecursionError) as error:
        raise RuntimeError('REFERENCE_RECEIPT_INVALID') from error


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


if __name__ == "__main__":
    import argparse
    import sys

    parser = argparse.ArgumentParser(description="Freeze the selected reference production pin set once.")
    parser.add_argument("--resolve-pins", action="store_true", required=True)
    parser.add_argument("--selection", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    options = parser.parse_args()
    if sys.flags.optimize:
        raise RuntimeError("assertions must be enabled")
    root = Path(__file__).resolve().parents[3]
    _, raw = resolve_pin_context(options.selection, root, options.out)
    print(json.dumps({"schema": PIN_CONTEXT_SCHEMA, "sha256": _pin_sha(raw)}))
