// Package server implements a YunKuaiChong V1.6 TCP server. The pile connects
// to us as a client; one goroutine handles one long-lived connection.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/protocol"
)

type Server struct {
	addr    string
	handler Handler
	store   *memStore
	ln      net.Listener
	mu      sync.Mutex
	conns   map[net.Conn]struct{}
	// sessionsByDevice lets downlinks target one pile instead of broadcasting
	// a price to every connected device.
	sessionsByDevice map[string]*session
	// TariffModelFor supplies the current 0x58 model for a device id.
	// Nil means no model configured (legacy behaviour).
	TariffModelFor func(deviceID string) (protocol.TariffModel, bool)
	closed  bool
}

func New(addr string, handler Handler) *Server {
	if handler == nil {
		handler = NopHandler{}
	}
	return &Server{addr: addr, handler: handler, store: &memStore{},
		conns: make(map[net.Conn]struct{}), sessionsByDevice: make(map[string]*session)}
}

// Start binds the TCP port and begins accepting connections.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("ykc: listen %s: %w", s.addr, err)
	}
	s.mu.Lock()
	s.ln = ln
	s.mu.Unlock()
	slog.Info("ykc server listening", "addr", ln.Addr().String())

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				s.mu.Lock()
				closed := s.closed
				s.mu.Unlock()
				if closed || errors.Is(err, net.ErrClosed) {
					return
				}
				slog.Warn("accept failed", "err", err)
				continue
			}
			s.mu.Lock()
			s.conns[conn] = struct{}{}
			s.mu.Unlock()
			sess := &session{srv: s, conn: conn}
			go func() {
				defer func() {
					s.mu.Lock()
					delete(s.conns, conn)
					if sess.deviceID != "" && s.sessionsByDevice[sess.deviceID] == sess {
						delete(s.sessionsByDevice, sess.deviceID)
					}
					s.mu.Unlock()
				}()
				sess.serve()
			}()
		}
	}()
	return nil
}

// SetHandler installs the event handler after construction, allowing a
// downlink component to hold the server before the full handler is assembled.
func (s *Server) SetHandler(h Handler) {
	if h == nil {
		return
	}
	s.mu.Lock()
	s.handler = h
	s.mu.Unlock()
}

// Addr returns the actual bound address (useful when port 0 is requested).
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// Stats returns the current in-memory aggregate.
func (s *Server) Stats() Stats { return s.store.snapshot() }

// Shutdown stops accepting, sends a reconnect-friendly close to active piles,
// then closes remaining sessions within the deadline. It never waits for a
// multi-hour charging order to finish.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	ln := s.ln
	conns := make([]net.Conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	if ln != nil {
		_ = ln.Close()
	}
	// Step 1: stop accepting (listener closed above).
	// Step 2: protocol-layer reconnect command would be sent here (future).
	// Step 3: close remaining sessions with a deadline.
	for _, c := range conns {
		_ = c.SetDeadline(time.Now().Add(100 * time.Millisecond))
	}
	done := make(chan struct{})
	go func() {
		for _, c := range conns {
			_ = c.Close()
		}
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RemoteStart sends 0x34 to the pile with a generated transaction id.
// The caller must hold the connector-level lock before invoking this.
func (s *Server) RemoteStart(deviceID string, gun byte, logicalCard, physicalCard []byte, balanceCents uint32) error {
	txn := make([]byte, 16) // BCD transaction id; caller may supply a real one
	copy(txn, []byte("0000000000000001"))
	body := make([]byte, 0, 16+7+1+8+8+4)
	body = append(body, txn...)
	body = append(body, deviceBCD(deviceID)...)
	body = append(body, gun)
	card8 := make([]byte, 8)
	copy(card8, logicalCard)
	body = append(body, card8...)
	phys8 := make([]byte, 8)
	copy(phys8, physicalCard)
	body = append(body, phys8...)
	var bal [4]byte
	bal[0] = byte(balanceCents)
	bal[1] = byte(balanceCents >> 8)
	bal[2] = byte(balanceCents >> 16)
	bal[3] = byte(balanceCents >> 24)
	body = append(body, bal[:]...)
	return s.broadcast(deviceID, protocol.TypeRemoteStart, body)
}

// RemoteStop sends 0x36 to the pile.
func (s *Server) RemoteStop(deviceID string, gun byte) error {
	body := append(deviceBCD(deviceID), gun)
	return s.broadcast(deviceID, protocol.TypeRemoteStop, body)
}

func (s *Server) broadcast(deviceID string, typ byte, body []byte) error {
	s.mu.Lock()
	sess := s.sessionsByDevice[deviceID]
	s.mu.Unlock()
	if sess == nil {
		return fmt.Errorf("ykc: device %s is not connected", deviceID)
	}
	sess.send(typ, body)
	return nil
}

// registerSession indexes a logged-in pile so downlinks can address it.
func (s *Server) registerSession(deviceID string, sess *session) {
	s.mu.Lock()
	s.sessionsByDevice[deviceID] = sess
	s.mu.Unlock()
}

// SendTariffModel sends 0x58 to one specific pile. Only call this while the
// connector is idle: the spec requires tariff updates during idle periods.
func (s *Server) SendTariffModel(deviceID string, body []byte) error {
	return s.broadcast(deviceID, protocol.TypeTariffSet, body)
}

func deviceBCD(id string) []byte {
	out := make([]byte, 7)
	for i := 0; i < 7 && i < len(id); i++ {
		b := byte(0)
		if id[i] >= '0' && id[i] <= '9' {
			b = id[i] - '0'
		}
		if i+1 < len(id) && id[i+1] >= '0' && id[i+1] <= '9' {
			b |= (id[i+1] - '0') << 4
		}
		out[i] = b
	}
	return out
}
