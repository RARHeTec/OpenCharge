// Command device-simulator creates virtual chargers for load testing and
// protocol integration without physical hardware.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"time"
)

func main() {
	count := flag.Int("n", 10, "number of virtual chargers")
	deviceID := flag.String("device", "SIM-001", "device id prefix")
	interval := flag.Duration("interval", 15*time.Second, "telemetry interval")
	flag.Parse()

	ctx := context.Background()
	for i := 0; i < *count; i++ {
		go simulate(ctx, fmt.Sprintf("%s-%03d", *deviceID, i), *interval)
	}
	fmt.Printf("simulator running: %d virtual chargers\n", *count)
	select {}
}

func simulate(ctx context.Context, deviceID string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var energy int64
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			energy += int64(rng.Intn(100))
			fmt.Printf("%s MeterValues energyWh=%d\n", deviceID, energy)
		}
	}
}
