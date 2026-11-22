#!/bin/sh
# Phase 0 の golden ITF を再生成する。Quint はリポジトリの package.json で exact pin した版を使う。
# 出力先は itf/testdata/golden/。#meta の description / timestamp は実行ごとに変わるため、
# 再生成の一致確認は states だけで行う。
set -eu
cd "$(dirname "$0")/.."
OUT=itf/testdata/golden
SPEC=phase0/spec
mkdir -p "$OUT"
q() { npx --no-install quint run --verbosity=0 "$@"; }

q "$SPEC/counter.qnt"     --mbt --backend=rust       --seed=0x1 --max-samples=10 --n-traces=2 --max-steps=6 --out-itf="$OUT/counter_rust_{seq}.itf.json"
q "$SPEC/counter.qnt"     --mbt --backend=typescript --seed=0x1 --max-samples=10 --n-traces=2 --max-steps=6 --out-itf="$OUT/counter_ts_{seq}.itf.json"
q "$SPEC/counter_big.qnt" --mbt --backend=typescript --seed=0x1 --max-samples=1  --n-traces=1 --max-steps=3 --out-itf="$OUT/counterbig_ts_{seq}.itf.json"
q "$SPEC/probe.qnt"       --mbt --backend=rust       --seed=0x2 --max-samples=10 --n-traces=1 --max-steps=8 --out-itf="$OUT/probe_rust_{seq}.itf.json"
q "$SPEC/probe.qnt"       --mbt --backend=typescript --seed=0x2 --max-samples=10 --n-traces=1 --max-steps=8 --out-itf="$OUT/probe_ts_{seq}.itf.json"
q "$SPEC/probe2.qnt" --main=probe2 --mbt --backend=rust --seed=0x3 --max-samples=10 --n-traces=1 --max-steps=6 --out-itf="$OUT/probe2_rust_{seq}.itf.json"
q "$SPEC/probe2.qnt" --main=probe3 --mbt --backend=rust --seed=0x3 --max-samples=10 --n-traces=1 --max-steps=3 --out-itf="$OUT/probe3_rust_{seq}.itf.json"
q "$SPEC/probe2.qnt" --main=probe3       --backend=rust --seed=0x3 --max-samples=10 --n-traces=1 --max-steps=3 --out-itf="$OUT/probe3_nombt_rust_{seq}.itf.json"
q "$SPEC/probe4.qnt"      --mbt --backend=rust       --seed=0x4 --max-samples=10 --n-traces=1 --max-steps=6 --out-itf="$OUT/probe4_rust_{seq}.itf.json"
q "$SPEC/probe4.qnt"      --mbt --backend=typescript --seed=0x4 --max-samples=10 --n-traces=1 --max-steps=6 --out-itf="$OUT/probe4_ts_{seq}.itf.json"
q "examples/tictactoe/spec/tictactoe.qnt"   --mbt --backend=rust       --seed=0x2a --max-samples=3 --n-traces=3 --out-itf="$OUT/tictactoe_rust_{seq}.itf.json"
