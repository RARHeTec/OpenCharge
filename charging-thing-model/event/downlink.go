package event

// RemoteStartTransaction is dispatched only after the connector lock is held.
type RemoteStartTransaction struct {
	TransactionID string `json:"transactionId"`
	UserID        string `json:"userId"`
	TenantID      string `json:"tenantId"`
	Token         string `json:"token,omitempty"` // RFID / VIN / QR credential
	CommandSeq    int64  `json:"commandSeq"`
	ReservedBy    string `json:"reservedBy,omitempty"`
}

// RemoteStopTransaction stops an active session.
type RemoteStopTransaction struct {
	TransactionID string `json:"transactionId"`
	Reason        string `json:"reason"` // user | prepay_low | bms_alarm | fault
	CommandSeq    int64  `json:"commandSeq"`
}

type ChargingProfile struct {
	MaxPowerW   int64 `json:"maxPowerW"`
	StartWindow int64 `json:"startWindow"`
	EndWindow   int64 `json:"endWindow"`
}

type SetChargingProfile struct {
	TransactionID string          `json:"transactionId,omitempty"`
	Profile       ChargingProfile `json:"profile"`
	CommandSeq    int64           `json:"commandSeq"`
}

type ReserveNow struct {
	ReservationID string `json:"reservationId"`
	UserID        string `json:"userId"`
	ExpiresAt     int64  `json:"expiresAt"`
	CommandSeq    int64  `json:"commandSeq"`
}

type CancelReservation struct {
	ReservationID string `json:"reservationId"`
	CommandSeq    int64  `json:"commandSeq"`
}

type TriggerMessage struct {
	Requested string `json:"requested"` // Diagnostics | FirmwareStatus | MeterValues
	CommandSeq int64 `json:"commandSeq"`
}
