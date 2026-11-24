package telemetry

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Buffer accumulates samples and flushes on size or interval, whichever
// comes first. Writes are non-blocking for producers (channel send), and the
// single flush goroutine serializes writes to the store.
type Buffer struct {
	store   Store
	in      chan Sample
	size    int
	interval time.Duration
	done    chan struct{}
	once    sync.Once
}

func NewBuffer(store Store, size int, interval time.Duration) *Buffer {
	if size <= 0 {
		size = 100
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	b := &Buffer{
		store: store, in: make(chan Sample, size*4),
		size: size, interval: interval, done: make(chan struct{}),
	}
	go b.run()
	return b
}

// Push returns immediately; telemetry ingestion must never block the device
// read loop or the settlement path.
func (b *Buffer) Push(s Sample) {
	select {
	case b.in <- s:
	default:
		slog.Warn("telemetry buffer full, dropping sample", "device", s.DeviceID)
	}
}

func (b *Buffer) run() {
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()
	batch := make([]Sample, 0, b.size)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := b.store.WriteBatch(context.Background(), batch); err != nil {
			slog.Warn("telemetry flush failed", "err", err, "count", len(batch))
		} else {
			slog.Debug("telemetry flushed", "count", len(batch))
		}
		batch = batch[:0]
	}
	for {
		select {
		case s := <-b.in:
			batch = append(batch, s)
			if len(batch) >= b.size {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-b.done:
			flush()
			return
		}
	}
}

func (b *Buffer) Close() {
	b.once.Do(func() { close(b.done) })
	_ = b.store.Close()
}
