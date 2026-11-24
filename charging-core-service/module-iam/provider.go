// Package iam implements the platform's own OAuth2.1 authorization server and
// multi-tenant organization tree, while leaving an extension point for an
// external Identity Provider (Keycloak / Ory Hydra) to be swapped in later.
package iam

import "context"

// Principal is the authenticated identity resolved from a token.
type Principal struct {
	UserID     string
	TenantID   string
	StationIDs []string
	Roles      []string
	GrantIDs   []string
}

// Provider verifies credentials and issues/validates tokens.
type Provider interface {
	Authenticate(ctx context.Context, token string) (Principal, error)
	IssueToken(ctx context.Context, userID, tenantID string) (string, error)
}

// ExternalIdP is the extension point for a future external identity provider.
// Implement it and register with SetExternalIdP to replace the built-in one.
type ExternalIdP interface {
	Introspect(ctx context.Context, token string) (Principal, bool, error)
}

var external ExternalIdP

func SetExternalIdP(idp ExternalIdP) { external = idp }

// LocalProvider is the built-in Go implementation used at launch.
type LocalProvider struct{}

func (p *LocalProvider) Authenticate(ctx context.Context, token string) (Principal, error) {
	if external != nil {
		if principal, ok, err := external.Introspect(ctx, token); err == nil && ok {
			return principal, nil
		}
	}
	// Built-in token validation to be wired to the token store.
	return Principal{}, ErrInvalidToken
}

func (p *LocalProvider) IssueToken(ctx context.Context, userID, tenantID string) (string, error) {
	return "", ErrNotImplemented
}
