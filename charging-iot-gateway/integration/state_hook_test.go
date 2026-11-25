package integration

import "testing"

func TestConnectorStatusOccupied(t *testing.T) {
	tests := []struct {
		reported, plugged byte
		want              string
	}{
		{2, 1, "occupied"}, // idle + plugged in (the reported real-pile case)
		{2, 0, "idle"},     // idle + unplugged
		{3, 1, "charging"}, // charging wins over plugged
		{3, 0, "charging"},
		{1, 1, "faulted"},  // fault wins over plugged
		{0, 1, "offline"},  // offline wins over plugged
		{9, 1, "offline"},  // unknown reported state degrades safely
	}
	for _, tt := range tests {
		if got := connectorStatus(tt.reported, tt.plugged); got != tt.want {
			t.Errorf("connectorStatus(%d,%d) = %s, want %s",
				tt.reported, tt.plugged, got, tt.want)
		}
	}
}
