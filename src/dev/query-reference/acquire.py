"""Acquire the pinned upstream archive and locked crates only; never builds or runs crate code."""
import argparse
import concurrent.futures
import hashlib
import io
import json
import pathlib
import re
import subprocess
import sys
import tarfile
import threading
import time
import tomllib
import urllib.request

from build import ARCHIVE, UPSTREAM

DEADLINE = time.monotonic() + 900
LOCK = threading.Lock()
TOTAL = 0


class RefuseRedirect(urllib.request.HTTPRedirectHandler):
    # refuse before a second request exists; the crate URL is fixed and must answer directly
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        fp.close()
        raise RuntimeError('unexpected dependency redirect')


OPENER = urllib.request.build_opener(RefuseRedirect)


def spend(n):
    # every byte read counts, including a response that later fails its checksum
    global TOTAL
    with LOCK:
        TOTAL += n
        if TOTAL > 256 * 1024**2:
            raise RuntimeError('total download bound')


def digest(data):
    return hashlib.sha256(data).hexdigest()


def unpack(data, destination, prefix=None):
    size = 0
    count = 0
    hashes = {}
    with tarfile.open(fileobj=io.BytesIO(data), mode='r:*') as archive:
        for member in archive:
            count += 1
            if time.monotonic() > DEADLINE or count > 30000:
                raise RuntimeError('archive time/member bound')
            p = pathlib.PurePosixPath(member.name)
            if (p.is_absolute() or any(x in ('..', '') for x in p.parts)
                    or any(':' in x or '\\' in x or any(ord(c) < 32 for c in x) for x in p.parts)):
                raise RuntimeError('unsafe archive path')
            if prefix:
                if not p.parts or p.parts[0] != prefix:
                    raise RuntimeError('unexpected crate root')
                p = pathlib.PurePosixPath(*p.parts[1:])
            if not p.parts:
                if not member.isdir():
                    raise RuntimeError('non-directory archive root')
                continue
            if '.git' in p.parts or p.name == '.cargo-checksum.json':
                raise RuntimeError('unexpected archive metadata')
            target = destination.joinpath(*p.parts)
            if not target.resolve().is_relative_to(destination.resolve()):
                raise RuntimeError('archive escaped destination')
            if member.isdir():
                target.mkdir(parents=True, exist_ok=True)
                continue
            size += member.size
            if not member.isfile() or member.size > 64 * 1024**2 or size > 256 * 1024**2:
                raise RuntimeError('archive type/size bound')
            target.parent.mkdir(parents=True, exist_ok=True)
            source = archive.extractfile(member)
            if source is None:
                raise RuntimeError('missing archive body')
            body = source.read(member.size + 1)
            if len(body) != member.size:
                raise RuntimeError('archive length mismatch')
            with target.open('xb') as output:
                output.write(body)
            hashes[str(p)] = digest(body)
    return hashes, size


def acquire(package, vendor):
    name, version, checksum = package['name'], package['version'], package['checksum']
    if not re.fullmatch(r'[A-Za-z0-9_-]{1,100}', name) or not re.fullmatch(r'[0-9A-Za-z.+_-]{1,100}', version) or not re.fullmatch(r'[a-f0-9]{64}', checksum):
        raise RuntimeError('invalid locked package identity')
    if package['source'] != 'registry+https://github.com/rust-lang/crates.io-index':
        raise RuntimeError('unexpected dependency destination')
    url = f'https://static.crates.io/crates/{name}/{name}-{version}.crate'
    if time.monotonic() > DEADLINE:
        raise RuntimeError('download size/time bound')
    chunks, length = [], 0
    with OPENER.open(url, timeout=30) as response:
        if response.geturl() != url:
            raise RuntimeError('unexpected dependency redirect')
        while chunk := response.read(1024**2):
            length += len(chunk)
            spend(len(chunk))
            if length > 32 * 1024**2 or time.monotonic() > DEADLINE:
                raise RuntimeError('download size/time bound')
            chunks.append(chunk)
    body = b''.join(chunks)
    if digest(body) != checksum:
        raise RuntimeError('crate checksum mismatch')
    destination = vendor / f'{name}-{version}'
    destination.mkdir()
    hashes, unpacked = unpack(body, destination, f'{name}-{version}')
    with (destination / '.cargo-checksum.json').open('x', encoding='utf-8') as output:
        json.dump({'files': hashes, 'package': checksum}, output, sort_keys=True)
    return {'name': name, 'version': version, 'sha256': checksum,
            'archive_bytes': len(body), 'unpacked_bytes': unpacked, 'files': len(hashes)}


def main():
    if sys.flags.optimize:
        raise RuntimeError('assertions must be enabled')
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--upstream', type=pathlib.Path, required=True, help='pristine checkout of the pinned upstream commit')
    parser.add_argument('--git', type=pathlib.Path, required=True)
    parser.add_argument('--out', type=pathlib.Path, required=True, help='new directory for upstream.tar, vendor/ and acquisition.json')
    args = parser.parse_args()
    for path in (args.upstream, args.git, args.out):
        assert path.is_absolute(), path
    git = [str(args.git), '-C', str(args.upstream)]
    head = subprocess.check_output(git + ['rev-parse', 'HEAD'], text=True, timeout=60).strip()
    status = subprocess.check_output(git + ['status', '--porcelain'], text=True, timeout=60)
    if head != UPSTREAM or status:
        raise RuntimeError('pinned pristine upstream required')
    args.out.mkdir()
    (args.out / 'vendor').mkdir()
    (args.out / 'source').mkdir()
    with (args.out / 'upstream.tar').open('xb') as output:
        subprocess.run(git + ['archive', '--format=tar', UPSTREAM], stdout=output, check=True, timeout=60)
    archive = (args.out / 'upstream.tar').read_bytes()
    if len(archive) > 128 * 1024**2:
        raise RuntimeError('upstream archive bound')
    # build.py accepts only these exact bytes; a different git archive output fails here first
    if digest(archive) != ARCHIVE:
        raise RuntimeError('upstream archive identity mismatch')
    hashes, _ = unpack(archive, args.out / 'source')
    lock_bytes = (args.out / 'source/Cargo.lock').read_bytes()
    packages = tomllib.loads(lock_bytes.decode())['package']
    registry = [p for p in packages if 'source' in p]
    if len(registry) > 400:
        raise RuntimeError('registry package count bound')
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as executor:
        receipts = list(executor.map(lambda p: acquire(p, args.out / 'vendor'), registry))
    receipt = {'schema': 'tsgk-reference-acquisition/r1', 'upstream_commit': UPSTREAM,
               'archive_sha256': digest(archive), 'upstream_files': hashes,
               'upstream_lock_sha256': digest(lock_bytes), 'packages': receipts,
               'execution': 'NOT_RUN', 'complete': True}
    with (args.out / 'acquisition.json').open('x', encoding='utf-8') as output:
        json.dump(receipt, output, sort_keys=True, indent=2)
    print(json.dumps({'complete': True, 'registry_packages': len(receipts), 'download_bytes': TOTAL, 'execution': 'NOT_RUN'}))


if __name__ == '__main__':
    main()
