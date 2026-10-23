#!/usr/bin/env python3
"""Check local Markdown destinations and documentation reachability offline."""
from pathlib import Path
import re
import sys
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]


def destinations(path, root):
    # Inline code and fenced examples are documentation, not hyperlinks.
    text = re.sub(r'```.*?```', '', path.read_text(), flags=re.S)
    text = re.sub(r'`[^`\n]*`', '', text)
    for match in re.finditer(r'!?\[[^\]\n]*\]\(([^\s)]+)(?:\s+"[^"]*")?\)', text):
        url = urlsplit(match[1].strip('<>'))
        if url.scheme or url.netloc or not url.path:
            continue
        target = (root if url.path.startswith('/') else path.parent) / unquote(url.path.lstrip('/'))
        yield match[1], target.resolve()


def documentation_files(root):
    return {path.resolve() for path in [*root.glob('*.md'), *root.glob('docs/**/*.md'),
                                        root / 'web/README.md', root / 'scripts/FEATURES.md']
            if path.is_file() and path.name != 'AGENTS.md'}


def broken_links(root=ROOT):
    root = root.resolve()
    for path in sorted(documentation_files(root)):
        for label, target in destinations(path, root):
            if not target.exists():
                yield f'{path.relative_to(root)}: missing {label}'


def unindexed_docs(root=ROOT):
    """Every maintained doc page must be reachable from the project README."""
    root = root.resolve()
    files = documentation_files(root)
    pending, reached = [(root / 'README.md').resolve()], set()
    while pending:
        path = pending.pop()
        if path in reached or path not in files:
            continue
        reached.add(path)
        pending.extend(target for _, target in destinations(path, root) if target in files)
    for path in sorted(files - reached):
        if path.is_relative_to(root / 'docs'):
            yield f'{path.relative_to(root)}: unreachable from README.md; update the documentation index'


def main():
    errors = [*broken_links(), *unindexed_docs()]
    if errors:
        print('\n'.join(errors), file=sys.stderr)
        return 1
    print('Local documentation links and index: OK')
    return 0


if __name__ == '__main__':
    sys.exit(main())
