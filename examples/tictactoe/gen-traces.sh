#!/bin/sh
# tictactoe の評価用トレースを生成する。生成条件（seed・本数・ステップ数）は Phase 2 以降凍結する。
set -eu
cd "$(dirname "$0")"
mkdir -p testdata/traces
npx --no-install quint run spec/tictactoe.qnt --mbt --backend=rust --verbosity=0 \
  --seed=0x2a --max-samples=100 --n-traces=100 --max-steps=20 \
  --out-itf="testdata/traces/t{seq}.itf.json"
