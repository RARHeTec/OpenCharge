package tenant

import "context"

type ctxKey struct{}

// Context carries the caller's isolation scope through the whole request.
type Context struct {
	TenantID   string
	StationIDs []string
	UserID     string
	Roles      []string
	GrantIDs   []string // B2B2C User-Tenant-Grant records
}

func WithContext(ctx context.Context, c Context) context.Context {
	return context.WithValue(ctx, ctxKey{}, c)
}

func FromContext(ctx context.Context) (Context, bool) {
	c, ok := ctx.Value(ctxKey{}).(Context)
	return c, ok
}

// TenantID returns the tenant id or empty string when the context is missing.
// Callers must treat an empty id as a deny, never as "all tenants".
func TenantID(ctx context.Context) string {
	c, ok := FromContext(ctx)
	if !ok {
		return ""
	}
	return c.TenantID
}
