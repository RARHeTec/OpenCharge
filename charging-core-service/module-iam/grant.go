package iam

import "context"

// TenantGrant is the User-Tenant-Grant record created after a car owner
// explicitly accepts an operator's service and data authorization agreement.
type TenantGrant struct {
	ID         string
	UserID     string
	TenantID   string
	Scopes     []string
	CreatedAt  int64
	RevokedAt  int64
}

// GrantStore backs the B2B2C authorization check. Only granted tenants' data
// may appear in an operator admin list.
type GrantStore interface {
	ActiveGrants(ctx context.Context, userID string) ([]TenantGrant, error)
	HasGrant(ctx context.Context, userID, tenantID string) (bool, error)
}
