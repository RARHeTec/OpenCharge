package tenant

import (
	"context"

	"gorm.io/gorm"
)

// Scope injects tenant_id filtering into every query against shared tables.
// It fails closed: a missing or empty tenant yields a query matching nothing.
// This is the Go equivalent of the MyBatis-Plus tenant interceptor.
func Scope(ctx context.Context) func(*gorm.DB) *gorm.DB {
	c, ok := FromContext(ctx)
	if !ok || c.TenantID == "" {
		return func(db *gorm.DB) *gorm.DB { return db.Where("1 = 0") }
	}
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("tenant_id = ?", c.TenantID)
	}
}

// StationScope narrows access to the stations bound to the current role.
func StationScope(ctx context.Context) func(*gorm.DB) *gorm.DB {
	c, ok := FromContext(ctx)
	if !ok || len(c.StationIDs) == 0 {
		return func(db *gorm.DB) *gorm.DB { return db.Where("1 = 0") }
	}
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("station_id IN ?", c.StationIDs)
	}
}

// Scoped applies both tenant and station isolation in one call.
func Scoped(ctx context.Context) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return Scope(ctx)(StationScope(ctx)(db))
	}
}
