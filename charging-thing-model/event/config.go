package event

// TenantConfig is the white-label payload returned by /api/tenant/config.
// The mini-program must never hardcode brand assets.
type TenantConfig struct {
	TenantID     string `json:"tenantId"`
	Name         string `json:"name"`
	LogoURL      string `json:"logoUrl"`
	PrimaryColor string `json:"primaryColor"`
	ServicePhone string `json:"servicePhone"`
	PayMchID     string `json:"payMchId,omitempty"`
	PayAppID     string `json:"payAppId,omitempty"`
	RoamingEnabled bool `json:"roamingEnabled"`
}
