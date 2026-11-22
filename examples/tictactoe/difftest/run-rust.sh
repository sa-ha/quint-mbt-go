#!/bin/sh
# 公式 quint-connect（Rust）の tictactoe 例で、凍結トレースと改ざんトレースを 1 本ずつ判定する。
# 出力: 引数のファイル（既定は verdicts-rust.tsv）に「トレース名<TAB>判定<TAB>失敗の種類」を 1 行ずつ書く。
# 判定は pass か fail@<ステップ番号>。ステップ番号は QUINT_VERBOSE=1 のログの最後の [Step N] から読む。
set -eu
cd "$(dirname "$0")"
HERE=$(pwd)
QC_COMMIT=4f018f54fc7dd4cef341d10111427bab59d3b307
TOOLCHAIN=1.100.0
command -v rustup >/dev/null || PATH="/opt/homebrew/opt/rustup/bin:$PATH"
# Homebrew 版の rustup では `rustup run` が rustc を PATH に加えないため、版を固定した実体を直接使う
RUSTC=$(rustup which --toolchain "$TOOLCHAIN" rustc)
CARGO=$(rustup which --toolchain "$TOOLCHAIN" cargo)
export RUSTC
OUT=${1:-verdicts-rust.tsv}

[ -d .quint-connect ] || git clone -q https://github.com/informalsystems/quint-connect.git .quint-connect
git -C .quint-connect checkout -q "$QC_COMMIT"

BIN=$(cd .quint-connect && QUINT_VERBOSE=1 "$CARGO" test --locked -q -p quint-connect \
  --example tictactoe --no-run --message-format=json 2>/dev/null |
  python3 -I -c 'import json,sys
for l in sys.stdin:
    m=json.loads(l)
    if m.get("reason")=="compiler-artifact" and m.get("executable") and m["target"]["name"]=="tictactoe":
        print(m["executable"])')
[ -x "$BIN" ] || { echo "test binary not found" >&2; exit 1; }

: > "$OUT"
for f in ../testdata/traces/*.itf.json testdata/tampered/*.itf.json; do
  name=$(basename "$f" .itf.json)
  if log=$(cd .quint-connect/connect && NO_COLOR=1 PATH="$HERE/bin:$PATH" DIFFTEST_TRACE="$HERE/$f" \
      "$BIN" --nocapture --test-threads=1 2>&1); then
    printf '%s\tpass\t\n' "$name" >> "$OUT"
  else
    printf '%s' "$log" | python3 -I -c 'import re,sys
log=sys.stdin.read()
steps=re.findall(r"\[Step (\d+)\]",log)
kind=("mismatch" if "states diverge" in log else
      "panic" if "panicked at connect/examples/tictactoe/game.rs" in log else "error")
print("%s\tfail@%s\t%s" % (sys.argv[1], steps[-1] if steps else "-1", kind))' "$name" >> "$OUT"
  fi
done
wc -l < "$OUT"
