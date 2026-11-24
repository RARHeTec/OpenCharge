// Package trace carries a global TraceId from the first byte received by the
// IoT gateway all the way through Kafka headers and business service logs.
package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

const (
	HTTPHeader  = "X-Trace-Id"
	KafkaHeader = "trace_id"
)

type ctxKey struct{}

func New() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("trace: crypto/rand unavailable")
	}
	return hex.EncodeToString(b)
}

func WithTrace(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func FromContext(ctx context.Context) string {
	if id, ok := ctx.Value(ctxKey{}).(string); ok {
		return id
	}
	return ""
}

// FromHTTPHeader uses an upstream TraceId when present, otherwise creates one.
func FromHTTPHeader(values []string) string {
	if len(values) > 0 && values[0] != "" {
		return values[0]
	}
	return New()
}

func KafkaHeaders(ctx context.Context) map[string][]byte {
	id := FromContext(ctx)
	if id == "" {
		return nil
	}
	return map[string][]byte{KafkaHeader: []byte(id)}
}
