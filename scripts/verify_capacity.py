#!/usr/bin/env python3
"""Default-limit capacity gate through the maintained operations batch driver."""
import argparse
import hashlib
import json
from pathlib import Path

from verify_operations import capacity


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--binary', type=Path, required=True)
    p.add_argument('--evidence', type=Path, required=True)
    a = p.parse_args()
    result = capacity(a.binary.resolve())
    result['binary_sha256'] = hashlib.sha256(a.binary.read_bytes()).hexdigest()
    a.evidence.parent.mkdir(parents=True, exist_ok=True)
    a.evidence.write_text(json.dumps(result, indent=2)+'\n')
    print(json.dumps(dict(passed=result['passed'], items=result['items'], seconds=result['seconds'])))
