# quint-mbt-go

[日本語](README.md)

A small, dependency-free Go driver for model-based testing with
[Quint](https://github.com/informalsystems/quint). It replays the ITF traces
that `quint run --mbt` produces against your Go implementation and reports,
for each trace, the first step where the implementation and the model
disagree.

It follows the design of the official Rust driver,
[quint-connect](https://github.com/informalsystems/quint-connect), and was
checked against it: on 180 traces (100 generated, 80 deliberately tampered)
both drivers reach the same verdict and fail at the same step.

> Status: early. Not affiliated with Informal Systems.

```sh
go get github.com/sa-ha/quint-mbt-go
```

## How it works

1. You generate traces with the Quint CLI:

   ```sh
   mkdir -p testdata/traces
   quint run spec.qnt --mbt --seed=0x1 --max-samples=300 --n-traces=300 \
     --out-itf='testdata/traces/t{seq}.itf.json'
   ```

2. You write an adapter: a table from action names to calls into your code,
   and a function that reports the implementation state by model variable
   name.
3. The driver replays every trace with a fresh adapter. State 0 is the `init`
   step. After each step it compares the reported state with the trace, and
   ignores element order in sets and maps.

## Minimal example

The spec (`counter.qnt`):

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

Generate traces. Note that `--seed` makes `--max-samples` default to 1, so
set it to at least `--n-traces`. Quint does not create the output directory:

```sh
mkdir -p testdata/traces
quint run counter.qnt --mbt --seed=0x1 --max-samples=20 --n-traces=20 \
  --max-steps=10 --out-itf='testdata/traces/t{seq}.itf.json'
```

The adapter and the test:

```go
package counter

import (
	"testing"

	"github.com/sa-ha/quint-mbt-go/mbt"
)

// The implementation under test.
type Counter struct{ total int }

func (c *Counter) Add(k int)  { c.total += k }
func (c *Counter) Reset()     { c.total = 0 }
func (c *Counter) Total() int { return c.total }

type adapter struct{ c *Counter }

func (a *adapter) Actions() map[string]mbt.Action {
	return map[string]mbt.Action{
		"init": func(mbt.Picks) error { a.c = &Counter{}; return nil },
		"add": func(p mbt.Picks) error {
			k, err := p.Int("k") // the step's `nondet k`
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
	return map[string]any{"total": a.c.Total()}, nil // compared with `var total`
}

func TestCounter(t *testing.T) {
	mbt.Run(t, "testdata/traces/*.itf.json", func() mbt.Adapter { return &adapter{} })
}
```

Go maps become Quint maps, `map[K]struct{}` becomes a set, and structs become
records (use `itf:"name"` tags; unexported fields and `itf:"-"` are skipped).
Pointers and interfaces are followed; nil pointers, floats, channels, and
functions are errors. Use `itf.Variant`, `itf.Tuple`, and `itf.Some`/`itf.None`
for sum types, tuples, and options; a variant without arguments takes
`itf.Tuple()`, as in `itf.Variant("Pending", itf.Tuple())`.

Only the variables the adapter returns are compared. Model variables that a
passing trace never compared are logged (visible with `go test -v`); check
`Result.Unchecked` to enforce full coverage.

A failure report names the trace, the step, the action, its nondeterministic
picks, and each differing value path. This one comes from the inventory
example's `boundary` mutant, which rejects a request for exactly the available
stock:

```
FAIL testdata/traces/t10.itf.json: state mismatch at step 6, action process [client = "c2"]
  lastResult.tag: expected "Ok", observed "Rejected"
  lastResult.value: expected (), observed "insufficient"
  nextId: expected 2, observed 1
  reservations[1]: missing in observed (expected { expiresAt: 6, qty: 2, sku: "pear", status: Pending })
  stock["pear"]: expected 0, observed 2
```

## Examples

- [`examples/tictactoe`](examples/tictactoe): a port of quint-connect's
  tic-tac-toe example, used for the differential test against the Rust driver
  ([`difftest/`](examples/tictactoe/difftest))
- [`examples/inventory`](examples/inventory): a reservation service with
  submit, process, confirm, cancel, and expiry on a logical clock. Its tests
  include six seeded bugs, and the driver detects all of them

## Writing specs for MBT

- Name every branch of `any`. An unnamed branch records an empty
  `mbt::actionTaken`, and the driver rejects the step.
- The recorded action is the first named action entered after the most
  recently chosen `any` branch; with nested `any`, that is the inner branch.
  A `step` without `any` is recorded as `step`.
- Traces contain only enabled actions. To test that the implementation rejects
  something, model the attempt as an action that records the outcome in a
  state variable.

## Known Quint issues (0.33.1)

- The Rust backend serializes integers of 16 or more decimal digits as
  bignumber.js objects (`{"s", "e", "c"}`). The driver does not repair them.
  Instead, it adds a hint to the mismatch report when such a value is
  compared against an integer at the same path. Keep such integers out of
  the model, or use `--backend=typescript`.

## Tested with

Quint 0.33.1 (Rust evaluator v0.7.1), Go 1.27.3. Requires Go 1.27 or later.

## License

Apache License 2.0; see [LICENSE](LICENSE). `examples/tictactoe` contains a
specification copied from quint-connect and Go ports of its example code,
also under Apache License 2.0 (copyright Informal Systems Inc.); see
[NOTICE](examples/tictactoe/NOTICE).
