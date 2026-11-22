// Package inventory is a small stock-reservation service used as the
// function-call sample: the adapter drives it in-process. It implements
// spec/inventory.qnt directly.
package inventory

import (
	"errors"
	"maps"
)

// Errors returned by Service. ErrInsufficient and ErrNotPending are expected
// outcomes of valid requests; the others indicate misuse.
var (
	ErrBusy         = errors.New("inventory: client already has a request in flight")
	ErrNoRequest    = errors.New("inventory: client has no request in flight")
	ErrInsufficient = errors.New("inventory: insufficient stock")
	ErrNotFound     = errors.New("inventory: no such reservation")
	ErrNotPending   = errors.New("inventory: reservation is not pending")
)

// Status is the lifecycle state of a reservation.
type Status int

// Reservation statuses. Only Pending reservations hold stock that can return.
const (
	Pending Status = iota
	Confirmed
	Cancelled
	Expired
)

// Reservation is a hold of Qty units of SKU until ExpiresAt.
type Reservation struct {
	SKU       string
	Qty       int
	Status    Status
	ExpiresAt int64
}

// Request is a reservation request accepted but not yet processed.
type Request struct {
	SKU string
	Qty int
}

// Clock is injected so that expiry depends on logical time only.
type Clock interface{ Now() int64 }

// Service holds stock and reservations. It is not safe for concurrent use.
type Service struct {
	clock        Clock
	ttl          int64
	stock        map[string]int
	reservations map[int]Reservation
	nextID       int
	inflight     map[string]Request
}

// New returns a service with the given initial stock per SKU. Reservations
// expire ttl time units after they are made.
func New(stock map[string]int, ttl int64, clock Clock) *Service {
	return &Service{
		clock:        clock,
		ttl:          ttl,
		stock:        maps.Clone(stock),
		reservations: map[int]Reservation{},
		nextID:       1,
		inflight:     map[string]Request{},
	}
}

// Submit accepts a request without checking stock; Process checks it.
func (s *Service) Submit(client, sku string, qty int) error {
	if _, busy := s.inflight[client]; busy {
		return ErrBusy
	}
	s.inflight[client] = Request{SKU: sku, Qty: qty}
	return nil
}

// Process completes the client's request, reserving stock if enough is
// available at this moment, and returns the new reservation's ID.
func (s *Service) Process(client string) (int, error) {
	req, ok := s.inflight[client]
	if !ok {
		return 0, ErrNoRequest
	}
	delete(s.inflight, client)
	if s.stock[req.SKU] < req.Qty {
		return 0, ErrInsufficient
	}
	s.stock[req.SKU] -= req.Qty
	id := s.nextID
	s.nextID++
	s.reservations[id] = Reservation{SKU: req.SKU, Qty: req.Qty, Status: Pending, ExpiresAt: s.clock.Now() + s.ttl}
	return id, nil
}

// Confirm turns a pending reservation into a sale.
func (s *Service) Confirm(id int) error {
	r, ok := s.reservations[id]
	if !ok {
		return ErrNotFound
	}
	if r.Status != Pending {
		return ErrNotPending
	}
	r.Status = Confirmed
	s.reservations[id] = r
	return nil
}

// Cancel releases a pending reservation's stock.
func (s *Service) Cancel(id int) error {
	r, ok := s.reservations[id]
	if !ok {
		return ErrNotFound
	}
	if r.Status != Pending {
		return ErrNotPending
	}
	r.Status = Cancelled
	s.reservations[id] = r
	s.stock[r.SKU] += r.Qty
	return nil
}

// Sweep expires pending reservations whose ExpiresAt is not after the clock
// and returns their stock. Callers run it whenever the clock advances.
func (s *Service) Sweep() {
	now := s.clock.Now()
	for id, r := range s.reservations {
		if r.Status == Pending && r.ExpiresAt <= now {
			r.Status = Expired
			s.reservations[id] = r
			s.stock[r.SKU] += r.Qty
		}
	}
}

// Snapshot is a copy of the service's observable state.
type Snapshot struct {
	Stock        map[string]int
	Reservations map[int]Reservation
	NextID       int
	Inflight     map[string]Request
}

// Snapshot returns copies that the caller may keep.
func (s *Service) Snapshot() Snapshot {
	return Snapshot{
		Stock:        maps.Clone(s.stock),
		Reservations: maps.Clone(s.reservations),
		NextID:       s.nextID,
		Inflight:     maps.Clone(s.inflight),
	}
}
