package payment

// ReconciliationItem is one row of the daily three-way match.
type ReconciliationItem struct {
	OrderID       string
	ChannelAmount int64 // cents
	PlatformAmount int64
	DeviceAmount  int64
	Result        string // matched | long | short | mismatch
}

// Classify implements the T+1 three-way matching rules.
func Classify(item ReconciliationItem) string {
	switch {
	case item.ChannelAmount > 0 && item.PlatformAmount == 0:
		return "long"
	case item.ChannelAmount == 0 && item.PlatformAmount > 0:
		return "short"
	case item.PlatformAmount != item.DeviceAmount:
		return "mismatch"
	default:
		return "matched"
	}
}
