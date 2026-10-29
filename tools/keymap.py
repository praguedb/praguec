#!/usr/bin/env python3
"""Extract MessagePack [Key(N)] layouts from the pinned corpus and audit
alignment between related messages.

This is the executable form of the key-alignment check specified in
"The cross-message key-alignment check". It is deliberately regex-based and
corpus-shaped: it is evidence, not the eventual praguec implementation.

    python3 tools/keymap.py            # table + audit
    python3 tools/keymap.py --json     # machine-readable key maps
"""
import re, glob, os, sys, json

CORPUS = os.path.join(os.path.dirname(__file__), '..', 'corpus', 'prague', 'payloads')

MEMBER = re.compile(
    r'\[Key\((\d+)\)\]'                 # the key
    r'((?:\s*\[[^\]]*\])*)'             # any attributes between key and member
    r'\s*\n\s*public\s+(?:required\s+)?'
    r'([\w\.<>\?\[\], ()]+?)\s+(\w+)\s*(?:\{|=>)'
)
DECL = re.compile(r'public\s+(?:sealed\s+)?(?:partial\s+)?(class|struct|record)\s+(\w+)')

# (parent, view, kind) — kind names which of the three alignment relationships
# this pair is, because the rules differ per kind.
PAIRS = [
    ('BOSportGameCacheItem', 'BPSportGameCacheItem', 'twin'),
    ('BOBetTypeCacheItem',   'BPBetTypeCacheItem',   'twin'),
    ('BOSearchCacheItem',    'BPSearchCacheItem',    'twin'),
    ('BOLiveObjectCacheItem','BPLiveObjectCacheItem','twin'),
    ('BOLiveObjectCacheItem','LiveObjectCacheItem',  'twin'),
    ('BOVenue',              'Venue',                'twin'),
    ('BOSportGameCacheItem', 'SlimSportGame',        'projection'),
    ('BOBetTypeCacheItem',   'SlimBetType',          'projection'),
]


def load():
    types = {}
    for path in sorted(glob.glob(os.path.join(CORPUS, '*.cs'))):
        src = open(path, encoding='utf-8-sig').read()
        decl = DECL.search(src)
        name = decl.group(2) if decl else os.path.basename(path)[:-3]
        types[name] = sorted(
            (int(m.group(1)), m.group(3).strip(), m.group(4))
            for m in MEMBER.finditer(src)
        )
    return types


def normalise(t, kind):
    """A twin pair differs only by the BO/BP prefix on its own types; a
    projection may legitimately narrow the element type of a collection."""
    return t.replace('BO', '').replace('BP', '') if kind == 'twin' else t


def audit(types):
    findings = 0
    for a, b, kind in PAIRS:
        if a not in types or b not in types:
            continue
        da = {k: (t, n) for k, t, n in types[a]}
        db = {k: (t, n) for k, t, n in types[b]}
        shared = sorted(set(da) & set(db))
        added = sorted(set(db) - set(da))
        print(f'{a} vs {b}  [{kind}]  {len(shared)} shared')
        for k in shared:
            ta, na = da[k]
            tb, nb = db[k]
            if normalise(ta, kind) != normalise(tb, kind):
                print(f'  TYPE  key {k}: {ta} {na}  !=  {tb} {nb}')
                findings += 1
            elif na != nb:
                print(f'  NAME  key {k}: {na} != {nb}')
                findings += 1
        if added:
            print(f'  ADDED key(s) absent from the parent: {added}')
            findings += 1
    return findings


if __name__ == '__main__':
    types = load()
    if '--json' in sys.argv:
        json.dump(types, sys.stdout, indent=1)
        sys.exit(0)
    for n, ms in types.items():
        ks = [k for k, _, _ in ms]
        gaps = sorted(set(range(min(ks), max(ks) + 1)) - set(ks)) if ks else []
        print(f'{n:26} {len(ms):3} members  keys {min(ks)}..{max(ks)}  '
              f'gaps {",".join(map(str, gaps)) or "-"}')
    print()
    n = audit(types)
    print(f'\n{n} finding(s).')
