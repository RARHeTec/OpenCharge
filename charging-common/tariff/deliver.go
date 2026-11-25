package tariff

// DeliverResult is returned by the admin downlink endpoint and produced by
// the gateway integration layer; living in common keeps both in sync.
type DeliverResult struct {
	PlanID   string   `json:"planId"`
	Targeted []string `json:"targeted"`
	Skipped  []string `json:"skipped"`
	Sent     int      `json:"sent"`
}
