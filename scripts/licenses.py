#!/usr/bin/env python3
"""Collect notices for the Go packages actually linked into the native executable."""
import argparse
import json
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]


def collect(destination):
    destination.mkdir(parents=True, exist_ok=True)
    output = subprocess.check_output(
        ['go', 'list', '-deps', '-json=Dir,Module', './cmd/steward'],
        cwd=ROOT, text=True, timeout=120)
    decoder, modules = json.JSONDecoder(), {}
    while output.strip():
        package, end = decoder.raw_decode(output.lstrip())
        output = output.lstrip()[end:]
        module = package.get('Module')
        if not module or module.get('Main'):
            continue
        actual = module.get('Replace', module)
        name = actual['Path'] + '@' + actual.get('Version', 'local')
        entry = modules.setdefault(name, dict(module=actual['Path'], version=actual.get('Version'), files=set()))
        base = Path(actual['Dir'])
        directory = Path(package['Dir'])
        # Licenses may apply at a module or a nested package's parent directory.
        while directory.is_relative_to(base):
            for file in directory.iterdir():
                if file.is_file() and file.name.upper().startswith(('LICENSE', 'COPYING', 'NOTICE', 'COPYRIGHT', 'AUTHORS')):
                    relative = Path('go') / name / file.relative_to(base)
                    target = destination / relative
                    target.parent.mkdir(parents=True, exist_ok=True)
                    shutil.copyfile(file, target)
                    entry['files'].add(relative.as_posix())
            directory = directory.parent
    for name, entry in modules.items():
        if not any(Path(f).name.upper().startswith(('LICENSE', 'COPYING')) for f in entry['files']):
            raise RuntimeError(f'No upstream license found for linked module {name}; review before distribution')
        entry['files'] = sorted(entry['files'])
    goroot = Path(subprocess.check_output(['go', 'env', 'GOROOT'], cwd=ROOT, text=True).strip())
    shutil.copyfile(goroot / 'LICENSE', destination / 'go-toolchain.txt')
    shutil.copyfile(ROOT / 'internal/interaction/web/assets/licenses.txt', destination / 'web.txt')
    catalog_notices = destination / 'model-catalog'
    catalog_notices.mkdir(exist_ok=True)
    shutil.copyfile(ROOT / 'internal/llm/provider/LICENSE', catalog_notices / 'LICENSE')
    (catalog_notices / 'catalog').mkdir(exist_ok=True)
    shutil.copyfile(ROOT / 'internal/llm/provider/catalog/NOTICE.md', catalog_notices / 'catalog/NOTICE.md')
    (destination / 'go-modules.json').write_text(json.dumps([modules[k] for k in sorted(modules)], indent=2) + '\n')
    print(f'Collected notices for {len(modules)} linked Go modules, the Go toolchain and Web assets')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('destination', type=Path)
    collect(parser.parse_args().destination)
