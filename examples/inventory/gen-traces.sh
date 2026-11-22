#!/bin/sh
# 在庫予約の評価用トレースを生成する。生成条件（seed・本数・ステップ数）は Phase 2 以降凍結する。
set -eu
cd "$(dirname "$0")"
mkdir -p testdata/traces
npx --no-install quint run spec/inventory.qnt --mbt --backend=rust --verbosity=0 \
  --seed=0x1 --max-samples=300 --n-traces=300 --max-steps=25 \
  --out-itf="testdata/traces/t{seq}.itf.json"
