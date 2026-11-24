package server

import (
	"bufio"
	"context"
	"encoding/hex"
	"log/slog"
	"net"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-common/trace"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/protocol"
)

type session struct {
	srv      *Server
	conn     net.Conn
	deviceID string
	seq      uint16
}

func (s *session) serve() {
	defer s.conn.Close()
	splitter := protocol.NewSplitter(s.conn)
	for {
		frame, err := splitter.NextFrame()
		if err != nil {
			slog.Info("session closed", "remote", s.conn.RemoteAddr().String(), "err", err)
			return
		}
		s.handleFrame(frame)
	}
}

func (s *session) handleFrame(wire []byte) {
	// TraceId is generated at the very first line that sees raw device bytes.
	ctx := trace.WithTrace(context.Background(), trace.New())
	traceID := trace.FromContext(ctx)

	slog.Info("raw.frame",
		"traceId", traceID,
		"direction", "up",
		"remote", s.conn.RemoteAddr().String(),
		"length", len(wire),
		"hex", hex.EncodeToString(wire),
	)

	f, err := protocol.Decode(wire)
	if err != nil {
		slog.Warn("frame decode failed", "traceId", traceID, "err", err)
		return
	}
	s.seq = f.Seq // response keeps the same sequence number
	s.dispatch(traceID, f)
}

func (s *session) send(typ byte, body []byte) {
	wire := protocol.Encode(s.seq, false, typ, body)
	slog.Info("raw.frame",
		"traceId", "downlink",
		"direction", "down",
		"remote", s.conn.RemoteAddr().String(),
		"length", len(wire),
		"hex", hex.EncodeToString(wire),
	)
	if _, err := s.conn.Write(wire); err != nil {
		slog.Warn("write failed", "err", err)
	}
}

func (s *session) dispatch(traceID string, f protocol.Frame) {
	e := Event{TraceID: traceID, DeviceID: s.deviceID}

	switch f.Type {
	case protocol.TypeLogin:
		login, err := protocol.ParseLogin(f.Body)
		if err != nil {
			slog.Warn("bad login frame", "traceId", traceID, "err", err)
			return
		}
		s.deviceID = login.DeviceID
		e.Type = "BootNotification"
		e.DeviceID = login.DeviceID
		e.Payload = login
		s.srv.store.addLogin()
		slog.Info("device login", "traceId", traceID, "device", login.DeviceID, "guns", login.GunCount)
		// Whitelist mode: accept any device for integration testing.
		s.send(protocol.TypeLoginAck, protocol.LoginAckBody(f.Body[:7], true))

	case protocol.TypeHeartbeat:
		hb, err := protocol.ParseHeartbeat(f.Body)
		if err != nil {
			return
		}
		e.Type = "Heartbeat"
		e.DeviceID = hb.DeviceID
		e.GunNo = hb.GunNo
		e.Payload = hb
		s.srv.store.addHeartbeat()
		s.send(protocol.TypeHeartbeatAck, protocol.HeartbeatAckBody(hbDeviceBCD(f.Body), hb.GunNo))

	case protocol.TypeRealtime:
		rt, err := protocol.ParseRealtime(f.Body)
		if err != nil {
			slog.Warn("bad realtime frame", "traceId", traceID, "err", err, "len", len(f.Body))
			return
		}
		e.Type = "MeterValues"
		e.DeviceID = rt.DeviceID
		e.GunNo = rt.GunNo
		e.TransactionID = rt.TransactionID
		e.Payload = rt
		s.srv.store.addRealtime(rt)
		// Fast channel: telemetry is logged and aggregated, never blocks state.

	case protocol.TypeStartRequest:
		s.srv.store.addStartRequest()
		e.Type = "StartRequest"
		if len(f.Body) >= 9 {
			e.GunNo = f.Body[7]
		}
		slog.Info("start request", "traceId", traceID)

	case protocol.TypeRemoteStartAck:
		e.Type = "StartAck"
		if len(f.Body) >= 25 {
			e.GunNo = f.Body[24]
		}

	case protocol.TypeRemoteStopAck:
		e.Type = "StopAck"

	case protocol.TypeTransaction:
		txn, err := protocol.ParseTransaction(f.Body)
		if err != nil {
			slog.Warn("bad transaction frame", "traceId", traceID, "err", err, "len", len(f.Body))
			return
		}
		e.Type = "StopTransaction"
		e.DeviceID = txn.DeviceID
		e.GunNo = txn.GunNo
		e.TransactionID = txn.TransactionID
		e.Payload = txn
		s.srv.store.addTransaction(txn)
		slog.Info("transaction received", "traceId", traceID, "txn", txn.TransactionID,
			"energyUnits", txn.TotalEnergy, "amountUnits", txn.TotalAmount)
		// High priority: confirm receipt immediately so the pile deletes its local copy.
		s.send(protocol.TypeTransactionAck, protocol.TransactionAckBody(f.Body[:16], true))

	case protocol.TypeTariffVerify:
		// Respond "model matches" so the pile does not request a tariff download.
		if len(f.Body) >= 9 {
			body := append([]byte(nil), f.Body[:7]...)
			body = append(body, f.Body[7:9]...)
			body = append(body, 0x00) // verification result: consistent
			s.send(protocol.TypeTariffVerifyAck, body)
		}

	case protocol.TypeTariffRequest:
		// Empty tariff model response; real tariff delivery is a later phase.
		if len(f.Body) >= 7 {
			body := append([]byte(nil), f.Body[:7]...)
			body = append(body, 0x00, 0x00)
			s.send(protocol.TypeTariffRequestAck, body)
		}

	default:
		slog.Debug("unhandled frame", "traceId", traceID, "type", hex.EncodeToString([]byte{f.Type}))
		return
	}

	s.srv.handler.OnEvent(e)
}

func hbDeviceBCD(body []byte) []byte {
	if len(body) < 7 {
		return make([]byte, 7)
	}
	return body[:7]
}

// keep time import used for future idle timeout handling
var _ = time.Second
var _ = bufio.NewReader
