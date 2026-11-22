#!/usr/bin/env python3
"""tictactoe の凍結トレースから、差分テスト用の改ざんトレースを決定的に作る。

入力: ../testdata/traces/t{0..99}.itf.json
出力: testdata/tampered/*.itf.json と testdata/tampered/manifest.tsv

改ざんの種類（どれも乱数の seed を固定しているので、何度実行しても同じ出力になる）:
- cell-first / cell-last / cell-mid: board の 1 マスを書き換える（状態 0、最終状態、途中の状態）
- turn: nextTurn を反転する
- order: 全状態の #map の並び順だけを入れ替える。正しい判定は pass
- action: mbt::actionTaken を存在しない名前にする
- occupied: MoveO の座標を、直前の状態で埋まっているマスにする
"""
import copy
import json
import os
import random
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
SRC = os.path.join(HERE, "..", "testdata", "traces")
OUT = os.path.join(HERE, "testdata", "tampered")

EMPTY = {"tag": "Empty", "value": {"#tup": []}}


def occupied(player):
    return {"tag": "Occupied", "value": {"tag": player, "value": {"#tup": []}}}


def load(n):
    with open(os.path.join(SRC, f"t{n}.itf.json")) as f:
        return json.load(f)


def cell(state, row, col):
    for k, v in state["board"]["#map"]:
        if k["#bigint"] == str(row):
            for k2, v2 in v["#map"]:
                if k2["#bigint"] == str(col):
                    return v2
    raise KeyError((row, col))


def set_cell(state, row, col, value):
    for k, v in state["board"]["#map"]:
        if k["#bigint"] == str(row):
            for pair in v["#map"]:
                if pair[0]["#bigint"] == str(col):
                    pair[1] = value
                    return
    raise KeyError((row, col))


def flip_cell(state, rng):
    row, col = rng.randint(1, 3), rng.randint(1, 3)
    cur = cell(state, row, col)
    if cur["tag"] == "Empty":
        new = occupied(rng.choice(["X", "O"]))
    elif cur["value"]["tag"] == "X":
        new = occupied("O")
    else:
        new = EMPTY
    set_cell(state, row, col, new)
    return f"board[{row}][{col}]"


def main():
    rng = random.Random(20261123)
    os.makedirs(OUT, exist_ok=True)
    manifest = []

    def emit(name, trace, kind, step, expected):
        with open(os.path.join(OUT, f"{name}.itf.json"), "w") as f:
            json.dump(trace, f)
        manifest.append(f"{name}\t{kind}\t{step}\t{expected}")

    for n in range(0, 40):
        tr = load(n)
        states = tr["states"]
        if n < 10:
            kind, step = "cell-first", 0
        elif n < 20:
            kind, step = "cell-last", len(states) - 1
        elif n < 30:
            kind, step = "cell-mid", rng.randint(1, len(states) - 2)
        else:
            kind, step = "turn", rng.randint(0, len(states) - 1)
        if kind == "turn":
            cur = states[step]["nextTurn"]["tag"]
            states[step]["nextTurn"] = {"tag": "O" if cur == "X" else "X", "value": {"#tup": []}}
        else:
            flip_cell(states[step], rng)
        emit(f"d{n:03d}", tr, kind, step, f"fail@{step}")

    for n in range(40, 60):
        tr = load(n)
        for s in tr["states"]:
            rng.shuffle(s["board"]["#map"])
            for _, row in s["board"]["#map"]:
                rng.shuffle(row["#map"])
        emit(f"d{n:03d}", tr, "order", "-", "pass")

    for n in range(60, 70):
        tr = load(n)
        step = rng.randint(1, len(tr["states"]) - 1)
        tr["states"][step]["mbt::actionTaken"] = "Bogus"
        emit(f"d{n:03d}", tr, "action", step, f"fail@{step}")

    for n in range(70, 80):
        tr = load(n)
        states = tr["states"]
        steps = [i for i in range(1, len(states)) if states[i]["mbt::actionTaken"] == "MoveO"]
        step = rng.choice(steps)
        prev = states[step - 1]
        taken = [(r, c) for r in range(1, 4) for c in range(1, 4) if cell(prev, r, c)["tag"] != "Empty"]
        r, c = rng.choice(taken)
        states[step]["mbt::nondetPicks"]["coordinate"] = {
            "tag": "Some", "value": {"#tup": [{"#bigint": str(r)}, {"#bigint": str(c)}]}}
        emit(f"d{n:03d}", tr, "occupied", step, f"fail@{step}")

    with open(os.path.join(OUT, "manifest.tsv"), "w") as f:
        f.write("trace\tkind\tstep\texpected\n" + "\n".join(manifest) + "\n")
    print(f"wrote {len(manifest)} traces to {OUT}", file=sys.stderr)


if __name__ == "__main__":
    main()
