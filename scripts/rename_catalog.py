#!/usr/bin/env python3
"""Apply the bounded catalog spelling rewrite without changing wire literals."""
import argparse
import hashlib
from pathlib import Path
import re
import subprocess

TOKEN = re.compile(r'//[^\n]*|/\*[\s\S]*?\*/|"(?:\\.|[^"\\])*"|`[^`]*`|\'(?:\\.|[^\'\\])*\'|[A-Za-z_][A-Za-z_0-9]*')


def spelling(value):
    return value.replace('Catalogue', 'Catalog').replace('catalogue', 'catalog').replace('CATALOGUE', 'CATALOG')


def paths(value):
    for old, new in [('profiles/vendors/huawei/catalogue', 'profiles/vendors/huawei/catalog'),
                     ('tools/huaweicatalogue', 'tools/huaweicatalog'),
                     ('huawei_sun2000_catalogue_selection', 'huawei_sun2000_catalog_selection'),
                     ('sun2000_catalogue_decisions.json', 'sun2000_catalog_decisions.json'),
                     ('smartlogger_sdongle_emma_catalogue_decisions.json', 'smartlogger_sdongle_emma_catalog_decisions.json')]:
        value = value.replace(old, new)
    value = value.replace('huawei/catalogue', 'huawei/catalog')
    value = value.replace('tools/huaweicatalog/catalogue_test.go', 'tools/huaweicatalog/catalog_test.go').replace('tools/huaweicatalog/logger_catalogue_test.go', 'tools/huaweicatalog/logger_catalog_test.go')
    return value


def go_token(match):
    token = match.group()
    if token.startswith(('"', '`', "'")):
        return paths(token)
    if token.startswith( '//') or token.startswith('/*'):
        return paths(token) if token.startswith('//go:embed') else paths(re.sub(r'[A-Za-z_][A-Za-z_0-9]*', lambda m: spelling(m.group()) if 'Catalogue' in m.group() or m.group() == 'huaweicatalogue' else m.group(), token))
    return spelling(token)


def apply(root):
    files = list(root.glob('huawei*.go')) + list((root / 'tools/huaweicatalogue').glob('*.go')) + list((root / 'tools/huaweicatalog').glob('*.go'))
    for file in files:
        old = file.read_text()
        new = TOKEN.sub(go_token, old)
        if file.name == 'huawei_inverter_battery_disposition_test.go':
            new = new.replace('"catalogue", "emma-disposition.json"', '"catalog", "emma-disposition.json"')
        if old != new:
            file.write_text(new)
    text_files = [root / name for name in ['README.md', 'CHANGELOG.md', 'THIRD_PARTY_NOTICES.md', 'naming-inventory.json', 'naming-consumers.json', 'scripts/behavior-name-baseline.json', 'scripts/go-lint-baseline.json', 'scripts/size-baseline.json']]
    text_files += [root / 'docs' / name for name in ['huawei-catalogue.md', 'huawei-emma-observed-identity.md', 'huawei-readonly-providers.md', 'huawei-sdongle-observed-identity.md', 'huawei-smartlogger-observed-identity.md', 'huawei-sun2000-monitoring-profiles.md', 'huawei-vendor-documentation.md']]
    text_files += list((root / 'profiles/vendors/huawei').glob('*disposition.json')) + [root / 'profiles/vendors/huawei/evidence.json']
    text_files += list((root / 'testdata/huawei/provider').glob('*.json')) + list((root / 'testdata/huawei/qualification').glob('*observed_replay_expected.json'))
    for file in text_files:
        old = file.read_text()
        new = paths(old)
        if file.name in {'size-baseline.json', 'go-lint-baseline.json', 'behavior-name-baseline.json'}:
            new = re.sub(r'[A-Za-z_][A-Za-z_0-9]*', lambda m: spelling(m.group()) if 'Catalogue' in m.group() else m.group(), new)
        if old != new:
            file.write_text(new)
    moves = list((root / 'profiles/vendors/huawei/catalogue').rglob('*')) + list((root / 'tools/huaweicatalogue').glob('*')) + list(root.glob('huawei_sun2000_catalogue_selection*.go')) + list((root / 'testdata/huawei/selection').glob('*catalogue_decisions.json'))
    for file in moves:
        if not file.is_file():
            continue
        target = root / spelling(str(file.relative_to(root)))
        if target.exists():
            raise RuntimeError('destination already exists: ' + str(target))
        target.parent.mkdir(parents=True, exist_ok=True)
        file.rename(target)
    for source in ['profiles/vendors/huawei/catalogue', 'tools/huaweicatalogue']:
        directory = root / source
        children = sorted(directory.rglob('*'), key=lambda p: len(p.parts), reverse=True)
        for child in children + [directory]:
            if child.is_dir() and not any(child.iterdir()):
                child.rmdir()

    go_files = list(root.glob('huawei*.go')) + list((root / 'tools/huaweicatalog').glob('*.go'))
    subprocess.run(['gofmt', '-w', *map(str, go_files)], check=True)


def verify(before, after):
    count = 0
    for old in (before / 'profiles/vendors/huawei/catalogue').rglob('*'):
        if old.is_file():
            new = after / spelling(str(old.relative_to(before)))
            assert hashlib.sha256(old.read_bytes()).digest() == hashlib.sha256(new.read_bytes()).digest(), str(new)
            count += 1
    assert count, 'missing original embedded inputs'
    print(f'PASS: {count} embedded files have identical computed SHA-256 values')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, default=Path('.'))
    parser.add_argument('--verify-before', type=Path)
    args = parser.parse_args()
    if args.verify_before:
        verify(args.verify_before, args.root)
    else:
        apply(args.root)
