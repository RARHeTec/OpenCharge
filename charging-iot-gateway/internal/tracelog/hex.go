// Package tracelog writes the raw bytes received from a device together with
// the TraceId. This log is the only authoritative evidence when a vendor
// claims a frame was sent but the platform claims it was not received.
package tracelog

import (
	"context"
	"encoding/hex"
	"log/slog"

	"github.com/RARHeTec/OpenCharge/charging-common/trace"
)

func RawFrame(ctx context.Context, direction string, deviceID string, frame []byte) {
	id := trace.FromContext(ctx)
	if id == "" {
		id = trace.New()
	}
	slog.Info("raw.frame",
		"traceId", id,
		"direction", direction,
		"deviceId", deviceID,
		"length", len(frame),
		"hex", hex.EncodeToString(frame),
	)
}
