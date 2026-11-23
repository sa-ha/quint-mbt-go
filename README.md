# quint-mbt-go

[English](README.en.md)

[Quint](https://github.com/informalsystems/quint) の仕様を使ってモデルベーステスト（MBT）を行うための、Go のドライバです。`quint run --mbt` が出力する ITF トレースを Go の実装に再生し、実装とモデルの状態が最初に食い違ったステップをトレースごとに報告します。標準ライブラリだけで動き、外部の依存はありません。

設計は公式の Rust 製ドライバ [quint-connect](https://github.com/informalsystems/quint-connect) に沿っています。同じ 180 本のトレース（生成した 100 本と、わざと書き換えた 80 本）を両方で再生し、全件で判定（pass か、どのステップで失敗したか）が一致することを確かめました。

> 開発の初期段階です。Informal Systems とは関係のない個人のプロジェクトです。

## インストール

```sh
go get github.com/sa-ha/quint-mbt-go
```

## 使い方

ドライバはトレースを生成しません。トレースは Quint の CLI で作り、ドライバはそれを読んで再生します。

### 1. 仕様を書く

```quint
module counter {
  var total: int

  action init = total' = 0

  action add = {
    nondet k = 1.to(3).oneOf()
    total' = total + k
  }

  action reset = total' = 0

  action step = any { add, reset }
}
```

### 2. トレースを生成する

```sh
mkdir -p testdata/traces
quint run counter.qnt --mbt --seed=0x1 --max-samples=20 --n-traces=20 \
  --max-steps=10 --out-itf='testdata/traces/t{seq}.itf.json'
```

`--mbt` は必ず付けてください。各状態に実行したアクション名（`mbt::actionTaken`）と非決定的に選んだ値（`mbt::nondetPicks`）が記録され、ドライバはそれを使って再生します。`--seed` を指定すると `--max-samples` の既定値が 1 になるため、`--n-traces` 以上の値を明示する必要があります。また、Quint は出力先のディレクトリを作らないので、上の例のように `mkdir -p` で先に作っておきます。

### 3. アダプタを書いてテストする

アダプタが持つのは、アクション名から実装の呼び出しへの対応表と、実装の状態をモデルの変数名で返す関数の 2 つだけです。

```go
package counter

import (
	"testing"

	"github.com/sa-ha/quint-mbt-go/mbt"
)

// テスト対象の実装（例）
type Counter struct{ total int }

func (c *Counter) Add(k int)  { c.total += k }
func (c *Counter) Reset()     { c.total = 0 }
func (c *Counter) Total() int { return c.total }

type adapter struct{ c *Counter }

func (a *adapter) Actions() map[string]mbt.Action {
	return map[string]mbt.Action{
		"init": func(mbt.Picks) error { a.c = &Counter{}; return nil },
		"add": func(p mbt.Picks) error {
			k, err := p.Int("k") // このステップで選ばれた nondet k
			if err != nil {
				return err
			}
			a.c.Add(k)
			return nil
		},
		"reset": func(mbt.Picks) error { a.c.Reset(); return nil },
	}
}

func (a *adapter) State() (map[string]any, error) {
	return map[string]any{"total": a.c.Total()}, nil // 仕様の var total と比べる
}

func TestCounter(t *testing.T) {
	mbt.Run(t, "testdata/traces/*.itf.json", func() mbt.Adapter { return &adapter{} })
}
```

`mbt.Run` はパターンに合うトレースをパスの辞書順（`t10` は `t2` より先）にすべて再生し、ファイルが 1 つもなければテストを失敗にします。アダプタはトレースごとに新しく作るので、前のトレースの状態が次のトレースに持ち越されることはありません。

状態 0 は `init` のステップとして再生し、実装の状態は状態 0 を含むすべてのステップのあとでモデルと比べます。

## 状態の比べ方

アダプタが返した Go の値は、次の規則で ITF の値に変換してから比べます。

| Go の値 | Quint の値 |
|---|---|
| `bool`、`string`、整数型、`*big.Int` | `bool`、`str`、`int` |
| `map[K]struct{}` | 集合 |
| それ以外の `map[K]V` | マップ |
| slice、配列 | リスト |
| 構造体 | レコード（フィールド名は `itf:"name"` タグ、なければ Go のフィールド名） |
| ポインタ、インタフェース | 指している値を変換したもの |
| `itf.Value` | そのまま |
| `itf.Valuer` を実装した型 | `ITFValue()` が返した値 |

- nil の map と slice は空の集合・マップ・リストとして扱います。nil のポインタやインタフェースは変換できず、状態の変換の失敗になります。浮動小数点数、チャネル、関数も変換できません。
- 構造体の非公開フィールドと `itf:"-"` を付けたフィールドは変換されません。モデル側にその変数があると、報告には `missing in observed` と出ます。
- 直和型、タプル、Option は `itf.Variant`、`itf.Tuple`、`itf.Some` / `itf.None` で作ります。引数のない直和型の値は `itf.Variant("Pending", itf.Tuple())` のように空のタプルを渡して作ります。
- 集合とマップは要素の並び順を無視して比べます。Quint のバックエンドによって並び順が変わるためです。
- 比べるのはアダプタが返した変数だけです。トレースにない変数名が返ってきたら、綴りの誤りとみなして失敗にします。比べずに終わったモデルの変数は、pass したトレースについては `go test -v` のログに一覧で出ます。返し忘れをテストで確実に検出したいときは、`mbt.Run` が返す結果の `Unchecked` が空であることを確かめます（`examples/inventory/inventory_test.go` を参照）。

## 失敗の報告

報告には、トレースのファイル名、ステップ番号、アクション名、そのステップの nondet の値、食い違った値のパスが出ます。次の例は在庫予約のサンプルで実際に出た報告です。このときの実装には在庫ちょうどの数量の予約を拒否する誤りを入れていました。

```
FAIL testdata/traces/t10.itf.json: state mismatch at step 6, action process [client = "c2"]
  lastResult.tag: expected "Ok", observed "Rejected"
  lastResult.value: expected (), observed "insufficient"
  nextId: expected 2, observed 1
  reservations[1]: missing in observed (expected { expiresAt: 6, qty: 2, sku: "pear", status: Pending })
  stock["pear"]: expected 0, observed 2
```

失敗は次の 7 種類に分けて報告します。トレースの形式の誤り、無名のアクション、対応表にないアクション、アクションのエラー、panic、状態の取得・変換の失敗（トレースにない変数名を含む）、状態の不一致です。`Actions`、各アクション、`State` の中で起きた panic は捕まえて、そのステップの失敗として報告します。アダプタを作る関数（`mbt.Run` の第 3 引数）と `ITFValue()` の中の panic は捕まえないので、テストがそのまま止まります。

## 仕様を書くときの注意

- `any` の枝にはすべて名前付きのアクションを置いてください。`all { ... }` などを直接書いた枝では `mbt::actionTaken` が空文字で記録され、ドライバはそのステップを無名のアクションとして失敗にします。
- 記録されるアクション名は、最後に選ばれた `any` の枝の中で最初に入った名前付きアクションです。`any` が入れ子なら内側の枝の名前になり、`any` を使わない `step` は `step` と記録されます。
- `mbt::nondetPicks` には仕様中のすべての nondet 名が入りますが、値が入るのはそのステップで使われたものだけです。`mbt.Picks` からは、使われたものだけを取り出せます。
- トレースには有効だったアクションしか入りません。実装が操作を正しく拒否するかを確かめたいときは、その操作を常に有効なアクションにし、結果（成功か、拒否とその理由か）を状態の変数に記録してください。在庫予約のサンプルの `lastResult` がこの書き方の例です。

## Quint の既知の問題

Quint 0.33.1 で確認した問題です。

- Rust バックエンド（`quint run` の既定）は、10 進 16 桁以上の整数を bignumber.js の内部表現（`{"s", "e", "c"}`）のまま ITF に書き出します。ドライバはこれを整数に戻しません。この形の値が同じパスの整数と比べられて食い違ったときは、報告に注意書きを添えます（集合の要素やマップのキーの中にあるときは添えません）。そうした整数はモデルに入れないか、`--backend=typescript` で生成してください。
- Rust バックエンドの ITF では、`vars` 配列に mbt 変数が重複して入ります。ドライバは `vars` を使わないので、再生には影響しません。

## サンプル

- [`examples/tictactoe`](examples/tictactoe): quint-connect の三目並べの例を Go に移植したものです。公式版との突き合わせ（[`difftest/`](examples/tictactoe/difftest)）に使っています。
- [`examples/inventory`](examples/inventory): 在庫予約サービスです。予約の受付と処理、確定、キャンセル、論理時刻による期限切れを持ちます。テストには誤りを 1 つずつ入れた実装（ミュータント）が 6 種類あり、ドライバはすべてを検出します。

## テストと測定の再現

```sh
go test ./...                                                   # 単体テスト、サンプル、ミュータント
go test ./itf -run '^$' -fuzz FuzzDecode -fuzztime 60s          # デコーダの fuzz
go test ./examples/inventory -run Mutants -v                    # ミュータントごとの失敗トレース数
go test ./examples/inventory -run '^$' -bench Replay300Traces   # 再生の速さ
REPORT_OUT=$PWD/report.txt go test ./examples/inventory -run Deterministic -count=1
```

最後のコマンドは再生の報告の全文をファイルに書き出します。2 回実行して出力を比べると、決定性を確かめられます。

トレースを生成し直すときは、このリポジトリの `package.json` でバージョンを固定した Quint を使います。生成した ITF の `#meta` には生成時刻が入るため、コミット済みのファイルとは差分が出ます（`states` は同じ seed なら一致します）。

```sh
npm ci
./phase0/gen-golden.sh                  # デコーダの golden（itf/testdata/golden/）
./examples/tictactoe/gen-traces.sh
./examples/inventory/gen-traces.sh
```

公式 quint-connect との突き合わせには、Rust 1.100.0 のツールチェーン（rustup で導入）と python3 が要ります。`run-rust.sh` は初回に quint-connect を git で clone するので、ネットワークにつながっている必要があります。出力先は絶対パスで指定してください。

```sh
DIFFTEST_OUT=$PWD/go.tsv go test ./examples/tictactoe -run Differential
./examples/tictactoe/difftest/run-rust.sh $PWD/rust.tsv
python3 examples/tictactoe/difftest/compare.py go.tsv rust.tsv
```

設計の検討は [`docs/設計レビュー.md`](docs/設計レビュー.md) に、測定の結果は [`実測記録.md`](実測記録.md) にあります。

## 動作を確認したバージョン

Quint 0.33.1（Rust evaluator v0.7.1）、Go 1.27.3。`go.mod` の指定により Go 1.27 以上が必要です。公式版との突き合わせには quint-connect v0.1.2 と Rust 1.100.0 を使いました。

## ライセンス

Apache License 2.0 です（[LICENSE](LICENSE)）。`examples/tictactoe` には quint-connect から写した仕様と、その例のコードを Go に移植したものが含まれます。これらも Apache License 2.0（著作権者は Informal Systems Inc.）で、詳細は [NOTICE](examples/tictactoe/NOTICE) にあります。
