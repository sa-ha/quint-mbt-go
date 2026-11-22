#!/usr/bin/env python3
"""Go と Rust の判定 TSV を突き合わせる。使い方: compare.py <go.tsv> <rust.tsv>

判定（pass / fail@N）の一致を数え、不一致を列挙する。失敗の種類（Go の分類と Rust のログから
読んだ分類）は判定とは別に集計する。終了コードは判定の不一致があれば 1。
"""
import collections
import sys


def load(path):
    rows = {}
    with open(path) as f:
        for line in f:
            if line.strip():
                name, verdict, kind = (line.rstrip("\n").split("\t") + [""])[:3]
                rows[name] = (verdict, kind)
    return rows


go, rust = load(sys.argv[1]), load(sys.argv[2])
names = sorted(set(go) | set(rust))
diff = [n for n in names if go.get(n, ("missing",))[0] != rust.get(n, ("missing",))[0]]
print(f"traces: go {len(go)}, rust {len(rust)}, union {len(names)}")
print(f"verdict agreement: {len(names) - len(diff)}/{len(names)}")
print("verdict pairs:", dict(collections.Counter((go[n][0].split('@')[0], rust[n][0].split('@')[0]) for n in names if n in go and n in rust)))
print("kind pairs (failures):", dict(collections.Counter((go[n][1], rust[n][1]) for n in names if n in go and n in rust and go[n][1])))
for n in diff:
    print(f"DISAGREE {n}: go={go.get(n)} rust={rust.get(n)}")
sys.exit(1 if diff else 0)
