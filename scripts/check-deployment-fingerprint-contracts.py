#!/usr/bin/env python3
"""Read-only verification of the local M0 bundle and an optional peer snapshot."""
import argparse
import hashlib
import json
from pathlib import Path


def verify(repository):
    bundle = repository / 'contracts/deployment-fingerprint/v1'
    manifest = json.loads((bundle / 'bundle.sha256.json').read_text())
    expected = manifest['files']
    actual = {p.name for p in bundle.iterdir() if p.is_file() and p.name != 'bundle.sha256.json'}
    if actual != set(expected):
        raise ValueError('bundle file list differs from the frozen manifest')
    for name, digest in expected.items():
        if Path(name).name != name or name in ('.', '..'):
            raise ValueError('invalid manifest path')
        if hashlib.sha256((bundle / name).read_bytes()).hexdigest() != digest:
            raise ValueError('bundle drift: ' + name)
    return expected


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--peer', type=Path, help='Other repository root; comparison never writes files')
    arguments = parser.parse_args()
    try:
        local = verify(Path(__file__).resolve().parents[1])
        if arguments.peer is not None and verify(arguments.peer.resolve()) != local:
            raise ValueError('peer protocol snapshot differs')
    except (OSError, ValueError, KeyError) as error:
        parser.exit(1, str(error) + '\n')
    print('Deployment fingerprint M0 bundle verified' + ('; peer snapshot identical' if arguments.peer else ''))
