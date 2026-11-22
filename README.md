# quint-mbt-go

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
   quint run spec.qnt --mbt --seed=0x1 --max-samples=300 --n-traces=300 \
     --out-itf='testdata/traces/t{seq}.itf.json'
   ```

2. You write an adapter: a table from action names to calls into your code,
   and a function that reports the implementation state by model variable
   name.
3. The driver replays every trace with a fresh adapter. State 0 is the `init`
   step. After each step it compares the reported state with the trace, and
   ignores element order in sets and maps.

## Minimal adapter

```go
type adapter struct{ svc *Service }

func (a *adapter) Actions() map[string]mbt.Action {
	return map[string]mbt.Action{
		"init": func(mbt.Picks) error { a.svc = New(); return nil },
		"add": func(p mbt.Picks) error {
			k, err := p.Int("k") // the step's `nondet k`
			if err != nil {
				return err
			}
			return a.svc.Add(k)
		},
	}
}

func (a *adapter) State() (map[string]any, error) {
	return map[string]any{"total": a.svc.Total()}, nil // compared with `var total`
}

func TestModel(t *testing.T) {
	mbt.Run(t, "testdata/traces/*.itf.json", func() mbt.Adapter { return &adapter{} })
}
```

Go maps become Quint maps, `map[K]struct{}` becomes a set, and structs become
records (use `itf:"name"` tags). Use `itf.Variant`, `itf.Tuple`, and
`itf.Some`/`itf.None` for sum types, tuples, and options.

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
- The recorded action is the first named action entered after the innermost
  `any` choice. A `step` without `any` is recorded as `step`.
- Traces contain only enabled actions. To test that the implementation rejects
  something, model the attempt as an action that records the outcome in a
  state variable.

## Known Quint issues (0.33.1)

- The Rust backend serializes integers of 16 or more decimal digits as
  bignumber.js objects (`{"s", "e", "c"}`). The driver does not repair them.
  Instead, it adds a hint to the mismatch report. Keep such integers out of
  the model, or use `--backend=typescript`.

## Tested with

Quint 0.33.1 (Rust evaluator v0.7.1), Go 1.27.3.
