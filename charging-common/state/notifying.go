package state

import "context"

// NotifyingStore wraps a Store and publishes every successful update to a Hub.
// Keeping the notification at the store boundary means every call site (login,
// heartbeat, realtime, transaction ack) is covered automatically.
type NotifyingStore struct {
	Store
	hub *Hub
}

func NewNotifyingStore(inner Store, hub *Hub) *NotifyingStore {
	return &NotifyingStore{Store: inner, hub: hub}
}

func (s *NotifyingStore) Update(ctx context.Context, c ConnectorState) error {
	if err := s.Store.Update(ctx, c); err != nil {
		return err
	}
	if s.hub != nil {
		s.hub.Publish(Event{Type: "status", Connector: c})
	}
	return nil
}
