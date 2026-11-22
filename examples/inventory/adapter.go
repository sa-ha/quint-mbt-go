package inventory

import (
	"errors"

	"quintmbt/itf"
	"quintmbt/mbt"
)

// API is the surface the adapter drives. *Service implements it; the mutants
// in the tests wrap a Service and override one method each.
type API interface {
	Submit(client, sku string, qty int) error
	Process(client string) (int, error)
	Confirm(id int) error
	Cancel(id int) error
	Sweep()
	Snapshot() Snapshot
}

// Constants of spec/inventory.qnt.
const (
	initialStock = 2
	ttl          = 2
)

// ManualClock is a logical clock advanced only by the model's tick action.
type ManualClock struct{ T int64 }

// Now implements Clock.
func (c *ManualClock) Now() int64 { return c.T }

// Adapter replays spec/inventory.qnt against an API built by newAPI.
type Adapter struct {
	newAPI func(Clock) API
	clock  *ManualClock
	api    API
	last   itf.Value
}

// NewAdapter returns a factory for mbt.Run. newAPI builds the implementation
// under test from the initial stock and the adapter's clock; nil means *Service.
func NewAdapter(newAPI func(stock map[string]int, ttl int64, clock Clock) API) func() mbt.Adapter {
	if newAPI == nil {
		newAPI = func(stock map[string]int, ttl int64, clock Clock) API { return New(stock, ttl, clock) }
	}
	return func() mbt.Adapter {
		return &Adapter{newAPI: func(c Clock) API {
			return newAPI(map[string]int{"apple": initialStock, "pear": initialStock}, ttl, c)
		}}
	}
}

// outcome maps an API result to the spec's lastResult. Errors other than the
// two rejections the spec models are failures of the implementation.
func (a *Adapter) outcome(err error) error {
	switch {
	case err == nil:
		a.last = itf.Variant("Ok", itf.Tuple())
	case errors.Is(err, ErrInsufficient):
		a.last = itf.Variant("Rejected", itf.Str("insufficient"))
	case errors.Is(err, ErrNotPending):
		a.last = itf.Variant("Rejected", itf.Str("notPending"))
	default:
		return err
	}
	return nil
}

// Actions maps each spec action to one API call.
func (a *Adapter) Actions() map[string]mbt.Action {
	byID := func(call func(int) error) mbt.Action {
		return func(p mbt.Picks) error {
			id, err := p.Int("id")
			if err != nil {
				return err
			}
			return a.outcome(call(id))
		}
	}
	return map[string]mbt.Action{
		"init": func(mbt.Picks) error {
			a.clock = &ManualClock{}
			a.api = a.newAPI(a.clock)
			return a.outcome(nil)
		},
		"submit": func(p mbt.Picks) error {
			client, err1 := p.String("client")
			sku, err2 := p.String("sku")
			qty, err3 := p.Int("qty")
			if err := errors.Join(err1, err2, err3); err != nil {
				return err
			}
			return a.outcome(a.api.Submit(client, sku, qty))
		},
		"process": func(p mbt.Picks) error {
			client, err := p.String("client")
			if err != nil {
				return err
			}
			_, err = a.api.Process(client)
			return a.outcome(err)
		},
		"confirm": byID(func(id int) error { return a.api.Confirm(id) }),
		"cancel":  byID(func(id int) error { return a.api.Cancel(id) }),
		"tick": func(mbt.Picks) error {
			a.clock.T++
			a.api.Sweep()
			return a.outcome(nil)
		},
	}
}

type reservationView struct {
	SKU       string    `itf:"sku"`
	Qty       int       `itf:"qty"`
	Status    itf.Value `itf:"status"`
	ExpiresAt int64     `itf:"expiresAt"`
}

type requestView struct {
	SKU string `itf:"sku"`
	Qty int    `itf:"qty"`
}

var statusTags = [...]string{Pending: "Pending", Confirmed: "Confirmed", Cancelled: "Cancelled", Expired: "Expired"}

// State reports every spec variable.
func (a *Adapter) State() (map[string]any, error) {
	snap := a.api.Snapshot()
	reservations := map[int]reservationView{}
	for id, r := range snap.Reservations {
		reservations[id] = reservationView{r.SKU, r.Qty, itf.Variant(statusTags[r.Status], itf.Tuple()), r.ExpiresAt}
	}
	inflight := map[string]requestView{}
	for c, r := range snap.Inflight {
		inflight[c] = requestView{r.SKU, r.Qty}
	}
	return map[string]any{
		"stock":        snap.Stock,
		"reservations": reservations,
		"nextId":       snap.NextID,
		"now":          a.clock.T,
		"inflight":     inflight,
		"lastResult":   a.last,
	}, nil
}
