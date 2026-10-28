#!/usr/bin/env python3
"""Check local Markdown destinations and documentation reachability offline."""
from pathlib import Path
import html
import re
import sys
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]


def prose(path):
    """Repository Markdown, excluding code examples and injected runtime policy."""
    text = path.read_text(encoding='utf-8')
    if path.name == 'AGENTS.md':
        text = text.split('<!-- BEGIN MULTICA-RUNTIME (auto-managed; do not edit) -->')[0]
    lines, fence, list_indents = [], None, []
    for line in text.splitlines():
        line = line.expandtabs(4)
        if not line.strip():
            lines.append('')
            continue
        # Code indentation is relative to the current list item, not the page.
        # Keep item content columns across blank lines for loose/nested lists.
        indent = len(line) - len(line.lstrip(' '))
        while list_indents and indent < list_indents[-1]:
            list_indents.pop()
        base = list_indents[-1] if list_indents else 0
        line = line[base:]
        if fence:
            if re.fullmatch(r' {0,3}' + re.escape(fence[0]) + '{' + str(len(fence)) + r',}\s*', line):
                fence = None
            lines.append('')
            continue
        item = re.match(r' {0,3}(?:[-+*]|[0-9]{1,9}[.)])(?: {1,4}(?=\S)| (?= |$))', line)
        if item:
            list_indents.append(base + item.end())
            line = line[item.end():]
        start = re.match(r' {0,3}(`{3,}|~{3,})', line)
        if start:
            fence = start[1]
            lines.append('')
        elif line.startswith(('    ', '\t')):
            lines.append('')
        else:
            lines.append(line)
    return re.sub(r'<!--.*?-->', '', '\n'.join(lines), flags=re.S)


def without_code_spans(text):
    """Mask inline examples, including multi-backtick spans."""
    return re.sub(r'(`+)(?!`)(.*?)(?<!`)\1(?!`)', '', text, flags=re.S)


def destinations(path, root):
    text = without_code_spans(prose(path))
    references = {}

    def reference_key(label):
        return ' '.join(label.split()).casefold()

    def definition(match):
        # Markdown uses the first definition; unused definitions are not links.
        references.setdefault(reference_key(match[1]), match[2])
        return ''

    text = re.sub(r'^ {0,3}\[([^\]\n]+)\]:[ \t]*(<[^>\n]+>|[^\s]+)'
                  r'(?:[ \t]+["\'][^\n]*?["\'])?[ \t]*$', definition, text, flags=re.M)
    for match in re.finditer(r'(?<!\\)!?\[([^\]\n]*)\](?:'
                             r'\((<[^>\n]+>|[^\s)]+)(?:\s+["\'][^\n]*?["\'])?\)'
                             r'|\[([^\]\n]*)\])?', text):
        label = match[2] or references.get(reference_key(match[3] or match[1]))
        if label is None:
            continue
        label = label.strip('<>')
        url = urlsplit(label)
        if url.scheme or url.netloc:
            continue
        if url.path:
            target = (root if url.path.startswith('/') else path.parent) / unquote(url.path.lstrip('/'))
        else:
            target = path
        yield label, target.resolve()


def anchors(path):
    """GitHub-style heading slugs plus explicit HTML id/name anchors."""
    text = prose(path)
    result = {html.unescape(match[2]) for match in re.finditer(
        r'<[A-Za-z][^>]*?\s(?:id|name)\s*=\s*(["\'])(.*?)\1', without_code_spans(text))}
    used = set()
    lines = text.splitlines()
    for index, line in enumerate(lines):
        heading = re.match(r' {0,3}#{1,6}\s+(.+?)\s*#*\s*$', line)
        if heading:
            title = heading[1]
        elif (index + 1 < len(lines) and line.strip()
              and re.fullmatch(r' {0,3}(?:=+|-+)\s*', lines[index + 1])):
            title = line.strip()
        else:
            continue
        # Formatting marks do not survive in slugs; code contents do.
        title = re.sub(r'!?\[([^\]]*)\]\([^)]*\)', r'\1', title)
        title = re.sub(r'<[^>]*>', '', title)
        title = html.unescape(title).lower()
        slug = re.sub(r'[^\w\-\s]', '', title)
        slug = re.sub(r'\s', '-', slug)
        candidate, suffix = slug, 0
        while candidate in used:
            suffix += 1
            candidate = f'{slug}-{suffix}'
        used.add(candidate)
        result.add(candidate)
    return result


def documentation_files(root):
    return {path.resolve() for path in [*root.glob('*.md'), *root.glob('docs/**/*.md'),
                                        root / 'web/README.md']
            if path.is_file()}


def broken_links(root=ROOT):
    root = root.resolve()
    known_anchors = {}
    for path in sorted(documentation_files(root)):
        for label, target in destinations(path, root):
            if not target.exists():
                yield f'{path.relative_to(root)}: missing {label}'
                continue
            fragment = unquote(urlsplit(label).fragment)
            if fragment and target.suffix.lower() == '.md' and target.is_file():
                if target not in known_anchors:
                    known_anchors[target] = anchors(target)
                if fragment not in known_anchors[target]:
                    yield f'{path.relative_to(root)}: missing anchor {label}'


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
