package inventory

import (
	"fmt"
	"path/filepath"
	"testing"

	"quintmbt/mbt"
)

// Each mutant wraps a faithful Service and replaces one method with a copy
// that contains a single seeded bug. Service itself stays untouched.

// boundary rejects a request for exactly the available stock (>= became >).
type boundary struct{ *Service }

func (m boundary) Process(client string) (int, error) {
	s := m.Service
	req, ok := s.inflight[client]
	if !ok {
		return 0, ErrNoRequest
	}
	delete(s.inflight, client)
	if s.stock[req.SKU] <= req.Qty {
		return 0, ErrInsufficient
	}
	s.stock[req.SKU] -= req.Qty
	id := s.nextID
	s.nextID++
	s.reservations[id] = Reservation{SKU: req.SKU, Qty: req.Qty, Status: Pending, ExpiresAt: s.clock.Now() + s.ttl}
	return id, nil
}

// confirmAny confirms reservations in any status (missing Pending guard).
type confirmAny struct{ *Service }

func (m confirmAny) Confirm(id int) error {
	r, ok := m.reservations[id]
	if !ok {
		return ErrNotFound
	}
	r.Status = Confirmed
	m.reservations[id] = r
	return nil
}

// cancelAny cancels reservations in any status and returns their stock.
type cancelAny struct{ *Service }

func (m cancelAny) Cancel(id int) error {
	r, ok := m.reservations[id]
	if !ok {
		return ErrNotFound
	}
	r.Status = Cancelled
	m.reservations[id] = r
	m.stock[r.SKU] += r.Qty
	return nil
}

// expiryLeak expires reservations but does not return their stock.
type expiryLeak struct{ *Service }

func (m expiryLeak) Sweep() {
	now := m.clock.Now()
	for id, r := range m.reservations {
		if r.Status == Pending && r.ExpiresAt <= now {
			r.Status = Expired
			m.reservations[id] = r
		}
	}
}

// expiryLate expires a reservation one tick late (<= became <).
type expiryLate struct{ *Service }

func (m expiryLate) Sweep() {
	now := m.clock.Now()
	for id, r := range m.reservations {
		if r.Status == Pending && r.ExpiresAt < now {
			r.Status = Expired
			m.reservations[id] = r
			m.stock[r.SKU] += r.Qty
		}
	}
}

// checkAtSubmit checks stock when the request is accepted and reserves at
// Process without checking again, so interleaved requests oversell.
type checkAtSubmit struct {
	*Service
	approved map[string]bool
}

func (m *checkAtSubmit) Submit(client, sku string, qty int) error {
	if err := m.Service.Submit(client, sku, qty); err != nil {
		return err
	}
	m.approved[client] = m.stock[sku] >= qty
	return nil
}

func (m *checkAtSubmit) Process(client string) (int, error) {
	s := m.Service
	req, ok := s.inflight[client]
	if !ok {
		return 0, ErrNoRequest
	}
	delete(s.inflight, client)
	if !m.approved[client] {
		return 0, ErrInsufficient
	}
	s.stock[req.SKU] -= req.Qty
	id := s.nextID
	s.nextID++
	s.reservations[id] = Reservation{SKU: req.SKU, Qty: req.Qty, Status: Pending, ExpiresAt: s.clock.Now() + s.ttl}
	return id, nil
}

var mutants = []struct {
	name string
	make func(stock map[string]int, ttl int64, clock Clock) API
}{
	{"boundary", func(st map[string]int, ttl int64, c Clock) API { return boundary{New(st, ttl, c)} }},
	{"confirmAny", func(st map[string]int, ttl int64, c Clock) API { return confirmAny{New(st, ttl, c)} }},
	{"cancelAny", func(st map[string]int, ttl int64, c Clock) API { return cancelAny{New(st, ttl, c)} }},
	{"expiryLeak", func(st map[string]int, ttl int64, c Clock) API { return expiryLeak{New(st, ttl, c)} }},
	{"expiryLate", func(st map[string]int, ttl int64, c Clock) API { return expiryLate{New(st, ttl, c)} }},
	{"checkAtSubmit", func(st map[string]int, ttl int64, c Clock) API {
		return &checkAtSubmit{New(st, ttl, c), map[string]bool{}}
	}},
}

// TestMutantsAreDetected is part of the frozen evaluator. Run with -v to see,
// per mutant, how many traces fail and where the first failure is.
func TestMutantsAreDetected(t *testing.T) {
	paths, _ := filepath.Glob(traces)
	for _, m := range mutants {
		results := mbt.ReplayFiles(paths, NewAdapter(m.make))
		failed, first := 0, ""
		for _, r := range results {
			if !r.Passed() {
				if failed == 0 {
					first = fmt.Sprintf("%s: %s at step %d", filepath.Base(r.Trace), r.Failure.Kind, r.Failure.Step)
				}
				failed++
			}
		}
		t.Logf("mutant %-13s failing traces %3d/%d; first %s", m.name, failed, len(results), first)
		if failed == 0 {
			t.Errorf("mutant %s was not detected", m.name)
		}
	}
}
