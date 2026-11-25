package admin

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/RARHeTec/OpenCharge/charging-common/trace"
)

// Principal is the authenticated caller: one API key maps to exactly one
// tenant. The tenant is taken from the credential, never from the query string,
// so a caller cannot read another tenant's data by changing a parameter.
type Principal struct {
	KeyID    string
	TenantID string
}

type principalKey struct{}

// KeyRegistry resolves API keys to principals.
type KeyRegistry interface {
	Lookup(key string) (Principal, bool)
}

// StaticKeys is a map-backed registry for single-node deployments and tests.
type StaticKeys struct {
	keys map[string]Principal
}

func NewStaticKeys(pairs map[string]Principal) *StaticKeys {
	return &StaticKeys{keys: pairs}
}

func (s *StaticKeys) Lookup(key string) (Principal, bool) {
	if s == nil || key == "" {
		return Principal{}, false
	}
	// Constant-time comparison over every entry avoids leaking key length or
	// prefix through timing.
	var found Principal
	ok := false
	for k, v := range s.keys {
		if subtle.ConstantTimeCompare([]byte(k), []byte(key)) == 1 {
			found, ok = v, true
		}
	}
	return found, ok
}

// PrincipalFrom returns the authenticated caller stored on the context.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// AuthMiddleware rejects unauthenticated requests with 401 and attaches the
// principal to the request context.
func (a *API) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No registry configured: single-tenant local mode, auth disabled.
		// Production must always supply keys via -admin-key.
		if a.keys == nil {
			next.ServeHTTP(w, r)
			return
		}
		// Health checks stay open so load balancers and probes work.
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		key := apiKeyFrom(r)
		if key == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "missing API key (send X-API-Key or Authorization: Bearer <key>)",
			})
			return
		}
		principal, ok := a.keys.Lookup(key)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid API key"})
			return
		}
		ctx := context.WithValue(r.Context(), principalKey{}, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func apiKeyFrom(r *http.Request) string {
	if k := strings.TrimSpace(r.Header.Get("X-API-Key")); k != "" {
		return k
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(auth) > 7 && strings.EqualFold(auth[:7], "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	// EventSource cannot set custom headers, so SSE clients pass the key as
	// a query parameter. Header forms remain preferred for normal API calls.
	if k := strings.TrimSpace(r.URL.Query().Get("api_key")); k != "" {
		return k
	}
	return ""
}

// authorize returns the tenant the caller may act on. A request for another
// tenant is refused with 403 rather than silently returning different data.
func (a *API) authorize(w http.ResponseWriter, r *http.Request) (string, bool) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		// No auth middleware mounted (single-tenant dev mode): honour the query
		// parameter so local exploration still works, but never in production.
		return r.URL.Query().Get("tenantId"), true
	}
	requested := r.URL.Query().Get("tenantId")
	if requested != "" && requested != principal.TenantID {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "tenant mismatch: credential is scoped to " + principal.TenantID,
			"traceId": trace.FromContext(r.Context()),
		})
		return "", false
	}
	return principal.TenantID, true
}

// resolveTenant is used by write paths (POST) where there is no query param.
func (a *API) resolveTenant(r *http.Request, fromBody string) (string, bool) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		return fromBody, true
	}
	if fromBody != "" && fromBody != principal.TenantID {
		return "", false
	}
	return principal.TenantID, true
}
