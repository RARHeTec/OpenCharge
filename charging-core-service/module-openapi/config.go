// Package openapi exposes tenant configuration and third-party integration APIs.
package openapi

import model "github.com/RARHeTec/OpenCharge/charging-thing-model/event"

// TenantConfigStore is read by /api/tenant/config. The client caches the result
// with a default-value fallback so a config hiccup never whitescreens the app.
type TenantConfigStore interface {
	GetByAppID(ctxContext any, appID string) (model.TenantConfig, error)
	GetByDomain(ctxContext any, domain string) (model.TenantConfig, error)
}
