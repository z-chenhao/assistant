#!/usr/bin/env python3
"""Check local Markdown destinations without network access or dependencies."""
from pathlib import Path
import re
import sys
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]


def broken_links(root=ROOT):
    files = [*root.glob('*.md'), *root.glob('docs/**/*.md'),
             root / 'web/README.md', root / 'scripts/FEATURES.md']
    for path in sorted(set(files)):
        if not path.is_file() or path.name == 'AGENTS.md':
            continue
        # Inline code and fenced examples are documentation, not hyperlinks.
        text = re.sub(r'```.*?```', '', path.read_text(), flags=re.S)
        text = re.sub(r'`[^`\n]*`', '', text)
        for match in re.finditer(r'!?\[[^\]\n]*\]\(([^\s)]+)(?:\s+"[^"]*")?\)', text):
            url = urlsplit(match[1].strip('<>'))
            if url.scheme or url.netloc or not url.path:
                continue
            target = (root if url.path.startswith('/') else path.parent) / unquote(url.path.lstrip('/'))
            if not target.exists():
                yield f'{path.relative_to(root)}: missing {match[1]}'


def main():
    errors = list(broken_links())
    if errors:
        print('\n'.join(errors), file=sys.stderr)
        return 1
    print('Local documentation links: OK')
    return 0


if __name__ == '__main__':
    sys.exit(main())
