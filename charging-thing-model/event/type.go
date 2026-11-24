package event

// Type identifies a unified thing-model event.
type Type string

const (
	// Uplink events (device -> platform)
	TypeBootNotification    Type = "BootNotification"
	TypeHeartbeat           Type = "Heartbeat"
	TypeStatusNotification  Type = "StatusNotification"
	TypeMeterValues         Type = "MeterValues"
	TypeStopTransaction     Type = "StopTransaction"
	TypeDiagnosticsResponse Type = "DiagnosticsResponse"

	// Downlink commands (platform -> device)
	TypeRemoteStartTransaction Type = "RemoteStartTransaction"
	TypeRemoteStopTransaction  Type = "RemoteStopTransaction"
	TypeSetChargingProfile     Type = "SetChargingProfile"
	TypeReserveNow             Type = "ReserveNow"
	TypeCancelReservation      Type = "CancelReservation"
	TypeTriggerMessage         Type = "TriggerMessage"
)

// ConnectorStatus is the canonical gun state. Transitions are guarded by a
// state machine in module-order-billing; adapters must map their native codes here.
type ConnectorStatus string

const (
	StatusAvailable   ConnectorStatus = "Available"
	StatusPlugged     ConnectorStatus = "Plugged"
	StatusCharging    ConnectorStatus = "Charging"
	StatusFaulted     ConnectorStatus = "Faulted"
	StatusReserved    ConnectorStatus = "Reserved"
	StatusUnavailable ConnectorStatus = "Unavailable"
)
