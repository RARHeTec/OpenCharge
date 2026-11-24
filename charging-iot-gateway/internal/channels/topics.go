// Package channels centralizes Kafka topic names and partition key rules.
// Fast and slow channels are physically separated so that a telemetry backlog
// can never delay a settlement event.
package channels

import "fmt"

const (
	// TopicTelemetry is high-throughput, many partitions, short retention.
	TopicTelemetry = "charging.telemetry.metervalues.v1"
	// TopicState is low-volume but high-priority: plug / start / stop / settle.
	TopicState = "charging.state.events.v1"
	// TopicCommands carries downlink commands to gateway nodes.
	TopicCommands = "charging.commands.v1"
)

// StateKey partitions by connector so that all events for one gun are ordered.
func StateKey(deviceID string, connectorID int) string {
	return fmt.Sprintf("%s:%d", deviceID, connectorID)
}

// IsHighPriority reports whether an event must go to the state topic.
func IsHighPriority(t string) bool {
	switch t {
	case "BootNotification", "Heartbeat", "StatusNotification", "StopTransaction":
		return true
	case "MeterValues", "DiagnosticsResponse":
		return false
	default:
		return true
	}
}
