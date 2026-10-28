package middleware

import (
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

type userClaims struct {
	Subject     string `json:"sub"`
	RealmAccess struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
}

// JWTAuth verifies signature, issuer, audience, expiry and the requested realm role.
// The same UserID context contract used by our handlers is preserved.
func JWTAuth(verifier *oidc.IDTokenVerifier, requiredRole string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		verified, err := verifier.Verify(r.Context(), parts[1])
		if err != nil {
			http.Error(w, "invalid bearer token", http.StatusUnauthorized)
			return
		}
		var claims userClaims
		if err := verified.Claims(&claims); err != nil || strings.TrimSpace(claims.Subject) == "" {
			http.Error(w, "invalid token claims", http.StatusUnauthorized)
			return
		}
		if !hasRole(claims.RealmAccess.Roles, requiredRole) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		// The subject is read only from the validated JWT, never from the request body.
		ctx := WithUserID(r.Context(), claims.Subject)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func hasRole(roles []string, wanted string) bool {
	for _, role := range roles {
		if role == wanted {
			return true
		}
	}
	return false
}
