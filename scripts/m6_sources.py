"""Validate fixture OCR/source provenance without treating hidden data as known."""
import hashlib
import math


def validate(document, descriptor, image_size):
    if document['sha256'] != descriptor['content_hash'] or document['image_size'] != image_size:
        raise ValueError('source hash or dimensions drift')
    for field in document['fields'].values():
        rect = field['region']
        if len(rect) != 4 or any(type(n) not in (int, float) or not math.isfinite(n) for n in rect):
            raise ValueError('invalid source region')
        x, y, w, h = rect
        if x < 0 or y < 0 or w <= 0 or h <= 0 or x+w > image_size[0] or y+h > image_size[1]:
            raise ValueError('source region out of bounds')
        if not field['visible'] and field['value'] is not None:
            raise ValueError('masked field must remain unknown')
    return {**document, 'artifactRef': descriptor['artifact_ref']}


def file_hash(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def vision(folder):
    """Optional macOS observation, kept separate from portable CI Fakes."""
    import json
    import platform
    from pathlib import Path
    import re
    import subprocess
    import tempfile
    root=Path(__file__).resolve().parents[1]
    manifest=json.loads((folder/'regions.json').read_text())
    report=dict(scope='actual macOS Vision on synthetic documents; not a model-quality/L2 personal-data evaluation',platform=platform.platform(),documents={})
    with tempfile.TemporaryDirectory(prefix='erii-vision-') as temporary:
        binary=Path(temporary)/'vision'
        subprocess.run(['swiftc',str(root/'scripts/m6_assets.swift'),'-o',str(binary)],check=True,capture_output=True,timeout=90)
        for name,document in manifest.items():
            result=subprocess.run([str(binary),'ocr',str(folder/name)],check=True,capture_output=True,text=True,timeout=120)
            observations=json.loads(result.stdout)
            width,height=document['image_size']
            checks={}
            for key,field in document['fields'].items():
                x,y,w,h=field['region']
                matching=[]
                for observation in observations:
                    a,b,c,d=observation['region']
                    if a*width<x+w and (a+c)*width>x and b*height<y+h and (b+d)*height>y:
                        matching.append(observation['text'])
                checks[key]=any(str(field['value']) in text for text in matching) if field['visible'] else not any(re.search(r'\b20\d{2}\b',text) for text in matching)
            report['documents'][name]=dict(sha256=file_hash(folder/name),observations=observations,assertions=checks)
    report['passed']=all(all(d['assertions'].values()) for d in report['documents'].values())
    report['cleanup']=True
    return report


if __name__=='__main__':
    import argparse
    import json
    from pathlib import Path
    parser=argparse.ArgumentParser(description='Optional actual macOS Vision source-region verification')
    parser.add_argument('--evidence',type=Path,required=True)
    args=parser.parse_args();args.evidence.unlink(missing_ok=True)
    report=vision(Path(__file__).resolve().parents[1]/'evals/fixtures/m6-v1')
    args.evidence.parent.mkdir(parents=True,exist_ok=True)
    args.evidence.write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps(dict(passed=report['passed'],cleanup=report['cleanup'])))
    raise SystemExit(0 if report['passed'] else 1)
