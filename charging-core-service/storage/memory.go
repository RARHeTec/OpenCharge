package storage

import (
	"context"
	"sort"
	"sync"

	"github.com/RARHeTec/OpenCharge/charging-common/money"
)

// MemoryOrderStore is an in-process OrderStore for tests and local closed loop.
type MemoryOrderStore struct {
	mu   sync.Mutex
	orders map[string]Order // by transaction id
	segs  map[string][]OrderSegment
}

func NewMemoryOrderStore() *MemoryOrderStore {
	return &MemoryOrderStore{orders: make(map[string]Order), segs: make(map[string][]OrderSegment)}
}

func (s *MemoryOrderStore) Create(_ context.Context, o Order) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.orders[o.TransactionID]; exists {
		return false, nil
	}
	s.orders[o.TransactionID] = o
	return true, nil
}

func (s *MemoryOrderStore) GetByTransaction(_ context.Context, txnID string) (Order, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[txnID]
	return o, ok, nil
}

func (s *MemoryOrderStore) Save(_ context.Context, o Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orders[o.TransactionID] = o
	return nil
}

func (s *MemoryOrderStore) SaveSegments(_ context.Context, orderID string, segs []OrderSegment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.segs[orderID] = append([]OrderSegment(nil), segs...)
	return nil
}

func (s *MemoryOrderStore) GetSegments(_ context.Context, orderID string) ([]OrderSegment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]OrderSegment(nil), s.segs[orderID]...), nil
}

func (s *MemoryOrderStore) List(_ context.Context, tenantID string, limit int) ([]Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Order, 0, len(s.orders))
	for _, o := range s.orders {
		if tenantID == "" || o.TenantID == tenantID {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type MemoryTariffStore struct {
	mu   sync.Mutex
	data map[string]TariffVersion
}

func NewMemoryTariffStore() *MemoryTariffStore {
	return &MemoryTariffStore{data: make(map[string]TariffVersion)}
}

func (s *MemoryTariffStore) Get(_ context.Context, id string) (TariffVersion, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[id]
	return v, ok, nil
}
func (s *MemoryTariffStore) Latest(_ context.Context, tenantID, stationID string, _ int64) (TariffVersion, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var best TariffVersion
	found := false
	for _, v := range s.data {
		if v.TenantID == tenantID && v.StationID == stationID {
			if !found || v.Effective.After(best.Effective) {
				best, found = v, true
			}
		}
	}
	return best, found, nil
}
func (s *MemoryTariffStore) List(_ context.Context, tenantID, stationID string) ([]TariffVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]TariffVersion, 0, len(s.data))
	for _, v := range s.data {
		if v.TenantID == tenantID && (stationID == "" || v.StationID == stationID) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Effective.After(out[j].Effective) })
	return out, nil
}

func (s *MemoryTariffStore) Save(_ context.Context, v TariffVersion) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[v.ID] = v
	return nil
}

type MemoryWalletStore struct {
	mu      sync.Mutex
	wallets map[string]Wallet
	applied map[string]bool // transaction id -> debited
}

func NewMemoryWalletStore() *MemoryWalletStore {
	return &MemoryWalletStore{wallets: make(map[string]Wallet), applied: make(map[string]bool)}
}

func (s *MemoryWalletStore) Get(_ context.Context, userID, tenantID string) (Wallet, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.wallets[userID+":"+tenantID]
	return w, ok, nil
}

func (s *MemoryWalletStore) Debit(_ context.Context, userID, tenantID, txnID string, amount money.Money) (Wallet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := userID + ":" + tenantID
	w, ok := s.wallets[key]
	if !ok {
		return w, ErrWalletNotFound
	}
	if !s.applied[txnID] {
		w.Principal = w.Principal.Sub(amount)
		s.applied[txnID] = true
		s.wallets[key] = w
	}
	return w, nil
}

func (s *MemoryWalletStore) Save(_ context.Context, w Wallet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wallets[w.UserID+":"+w.TenantID] = w
	return nil
}

type MemoryAuditStore struct {
	mu         sync.Mutex
	audits     []AuditRecord
	exceptions []ExceptionOrder
}

func NewMemoryAuditStore() *MemoryAuditStore { return &MemoryAuditStore{} }

func (s *MemoryAuditStore) Write(_ context.Context, a AuditRecord) error {
	s.mu.Lock(); defer s.mu.Unlock()
	s.audits = append(s.audits, a)
	return nil
}
func (s *MemoryAuditStore) WriteException(_ context.Context, e ExceptionOrder) error {
	s.mu.Lock(); defer s.mu.Unlock()
	s.exceptions = append(s.exceptions, e)
	return nil
}
func (s *MemoryAuditStore) ListExceptions(_ context.Context, tenantID string, limit int) ([]ExceptionOrder, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	out := append([]ExceptionOrder(nil), s.exceptions...)
	if limit > 0 && len(out) > limit { out = out[:limit] }
	return out, nil
}
func (s *MemoryAuditStore) ResolveException(_ context.Context, id, operator string) error {
	s.mu.Lock(); defer s.mu.Unlock()
	for i := range s.exceptions {
		if s.exceptions[i].ID == id {
			s.exceptions[i].Resolved = true
			return nil
		}
	}
	return ErrNotFound
}
func (s *MemoryAuditStore) ResolveExceptionByTransaction(_ context.Context, txnID, _ string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for i := range s.exceptions {
		if s.exceptions[i].TransactionID == txnID && !s.exceptions[i].Resolved {
			s.exceptions[i].Resolved = true
			n++
		}
	}
	return n, nil
}

func (s *MemoryAuditStore) Audits() []AuditRecord {
	s.mu.Lock(); defer s.mu.Unlock()
	return append([]AuditRecord(nil), s.audits...)
}

// Exceptions returns all recorded exception orders (test/inspection helper).
func (s *MemoryAuditStore) Exceptions() []ExceptionOrder {
	s.mu.Lock(); defer s.mu.Unlock()
	return append([]ExceptionOrder(nil), s.exceptions...)
}
