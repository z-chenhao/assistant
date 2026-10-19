#!/usr/bin/env python3
"""Refresh isolated diagnostic replicas without editing Required workflows."""
from pathlib import Path
import re
import subprocess

BRANCH = 'ERII-141-ordinary-profile'
ROOT = Path(__file__).resolve().parents[1]


def replica(source, label, path, cancel):
    text = re.sub(r'\Aname:.*?\n\non:.*?\npermissions:',
        f'name: ERII-141 {label} profile\n\non:\n  push:\n    branches: [{BRANCH}]\n    paths: [{path}]\n\npermissions:', source, count=1, flags=re.S)
    text = text.replace("github.head_ref == 'agent/erii-builder/erii-141'", f"github.ref_name == '{BRANCH}'")
    text = text.replace('BASE_SHA: ${{ github.event.pull_request.base.sha || github.event.before }}',
                        'BASE_SHA: ${{ github.event.before }}')
    if cancel:
        def job(match):
            value = match.group(0)
            guard = "!contains(github.event.head_commit.message, '[stop-cancel-profile]')"
            if re.search(r'^    if:', value, re.M):
                value = re.sub(r'(^    if: )(.*)$', lambda m: m[1]+'${{ ('+
                    m[2].strip().removeprefix('${{').removesuffix('}}').strip()+') && '+guard+' }}',
                    value,count=1,flags=re.M)
            else:
                value = value.replace('\n','\n    if: ${{ '+guard+' }}\n',1)
            return value
        return re.sub(r'^  (fast|race|verify|checks):\n(?:(?!^  [a-z]+:).)*', job, text, flags=re.M|re.S)
    return text.replace('name: Shadow feedback','name: Ordinary feedback').replace(
        'name: Shadow race detector','name: Ordinary race detector').replace(
        'name: Shadow verification','name: Ordinary verification').replace(
        'name: CI shadow comparison','name: Ordinary complete profile')


def generate(root):
    folder = root/'.github/workflows'
    for filename,label in [('ci.yml','control'),('ci-shadow.yml','shadow')]:
        text = replica((folder/filename).read_text(),label+' cancellation','docs/ci-cancel-probe.md',True)
        (folder/f'ci-{label}-cancel-profile.yml').write_text(text)
    text = replica((folder/'ci-shadow.yml').read_text(),'ordinary','docs/ci-ordinary-probe.md',False)
    (folder/'ci-ordinary-profile.yml').write_text(text)


def main():
    branch = subprocess.check_output(['git','branch','--show-current'],cwd=ROOT,text=True).strip()
    if branch != BRANCH:
        raise ValueError('diagnostic branch required; never write replicas on a PR branch')
    generate(ROOT)


if __name__ == '__main__':
    main()
