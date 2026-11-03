"""Encode the browser-baked complete-frame PNG atlases without pixel loss."""
from pathlib import Path
import subprocess

assets = Path(__file__).resolve().parents[2] / 'internal/interaction/web/assets/brand'
for tone in ('charcoal', 'ivory'):
    for size in (96, 256):
        source = assets / f'ghost-{tone}-{size}.png'
        target = source.with_suffix('.webp')
        subprocess.run(['cwebp', '-quiet', '-lossless', '-exact', '-m', '6', str(source), '-o', str(target)], check=True)
        print(target.name, target.stat().st_size)
        source.unlink()
