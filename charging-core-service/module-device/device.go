// Package device manages the station / device / connector ledger.
package device

// Connector is one charging gun. gunNo is part of the distributed lock key.
type Connector struct {
	DeviceID   string
	GunNo      int
	StationID  string
	TenantID   string
	Status     string
	LastSeenAt int64
}

func LockKey(c Connector) string {
	// Delegates to charging-common to keep exactly one definition of the key.
	return lockKey(c.DeviceID, c.GunNo)
}
