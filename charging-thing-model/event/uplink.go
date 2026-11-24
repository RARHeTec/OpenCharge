package event

// BootNotification is sent when a device registers or reconnects.
type BootNotification struct {
	Vendor        string `json:"vendor"`
	Model         string `json:"model"`
	FirmwareVersion string `json:"firmwareVersion"`
	SerialNumber  string `json:"serialNumber"`
	ICCID         string `json:"iccid,omitempty"`
}

// Heartbeat reports link quality.
type Heartbeat struct {
	SignalStrength int `json:"signalStrength,omitempty"`
}

// StatusNotification reports a gun state change.
type StatusNotification struct {
	Status      ConnectorStatus `json:"status"`
	VendorCode  string          `json:"vendorCode,omitempty"`
	Info        string          `json:"info,omitempty"`
	BMSFault    bool            `json:"bmsFault,omitempty"`
	Temperature int             `json:"temperature,omitempty"` // cable temp, Celsius
}

// MeterSample is one telemetry point. All numeric fields are integers;
// energy is Wh, voltage mV, current mA, power W, SOC in percent*100.
type MeterSample struct {
	Timestamp int64 `json:"timestamp"`
	Voltage   int64 `json:"voltageMv,omitempty"`
	Current   int64 `json:"currentMa,omitempty"`
	Power     int64 `json:"powerW,omitempty"`
	EnergyWh  int64 `json:"energyWh"`
	SOC       int32 `json:"socX100,omitempty"`
	CableTemp int32 `json:"cableTemp,omitempty"`
}

// MeterValues is the high-throughput telemetry event. It carries a batch and
// is routed to the dedicated telemetry topic, never to the state topic.
type MeterValues struct {
	TransactionID string        `json:"transactionId"`
	Samples       []MeterSample `json:"samples"`
}

// StopTransaction is the final bill reported by the device.
// Offline=true means the transaction happened while disconnected and is being
// backfilled; TariffVersionID lets the billing engine reprice with history.
type StopTransaction struct {
	TransactionID   string `json:"transactionId"`
	Reason          string `json:"reason"`
	StartEnergyWh   int64  `json:"startEnergyWh"`
	EndEnergyWh     int64  `json:"endEnergyWh"`
	TotalEnergyWh   int64  `json:"totalEnergyWh"`
	Offline         bool   `json:"offline"`
	StartedAt       int64  `json:"startedAt"`
	StoppedAt       int64  `json:"stoppedAt"`
	TariffVersionID string `json:"tariffVersionId,omitempty"`
	Segments        []EnergySegment `json:"segments,omitempty"`
}

// EnergySegment is a time-sliced energy reading for peak/valley billing.
type EnergySegment struct {
	From     int64 `json:"from"`
	To       int64 `json:"to"`
	EnergyWh int64 `json:"energyWh"`
	Period   string `json:"period"` // sharp | peak | flat | valley
}

type DiagnosticsResponse struct {
	TraceID   string `json:"traceId"`
	Success   bool   `json:"success"`
	LogURL    string `json:"logUrl,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}
