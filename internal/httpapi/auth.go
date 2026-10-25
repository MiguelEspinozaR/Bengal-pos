package httpapi

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"bengalpos/internal/pos"
)

type contextKey string

const userContextKey contextKey = "auth_user"

func getUser(r *http.Request) *pos.UsuarioSesion {
	v := r.Context().Value(userContextKey)
	if v == nil {
		return nil
	}
	u, _ := v.(*pos.UsuarioSesion)
	return u
}

func withUser(ctx context.Context, u *pos.UsuarioSesion) context.Context {
	return context.WithValue(ctx, userContextKey, u)
}

type authServer struct {
	svc          *pos.Service
	cookieSecure bool
	cookieName   string
	validate     func(context.Context, string) (*pos.UsuarioSesion, error)
}

func newAuthServer(svc *pos.Service) *authServer {
	secure := strings.EqualFold(os.Getenv("SESSION_COOKIE_SECURE"), "true")
	return &authServer{
		svc:          svc,
		cookieSecure: secure,
		cookieName:   "bengalpos_sesion",
	}
}

func (a *authServer) cookieTTL() time.Duration {
	if a.svc == nil {
		return pos.DefaultSessionTTL
	}
	return a.svc.SessionTTL()
}

func (a *authServer) setSessionCookie(w http.ResponseWriter, token string, expira time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     a.cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   a.cookieSecure,
		Expires:  expira,
		MaxAge:   int(a.cookieTTL().Seconds()),
	})
}

func (a *authServer) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     a.cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   a.cookieSecure,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func (a *authServer) checkSession(ctx context.Context, token string) (*pos.UsuarioSesion, error) {
	if a.validate != nil {
		return a.validate(ctx, token)
	}
	if a.svc == nil {
		return nil, pos.ErrNoAutenticado
	}
	return a.svc.ValidateSession(ctx, token)
}

func (a *authServer) sessionToken(r *http.Request) string {
	c, err := r.Cookie(a.cookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func isPublicPath(path string) bool {
	if path == "/healthz" {
		return true
	}
	if path == "/api/auth/login" || path == "/api/auth/logout" {
		return true
	}
	return false
}

func (a *authServer) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) {
			// Still allow resolving a session if present for /api/auth/logout
			// idempotency, but do not require it.
			if r.URL.Path == "/api/auth/logout" {
				if token := a.sessionToken(r); token != "" {
					if u, err := a.checkSession(r.Context(), token); err == nil {
						ctx := withUser(r.Context(), u)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		token := a.sessionToken(r)
		u, err := a.checkSession(r.Context(), token)
		if err != nil {
			writeError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(withUser(r.Context(), u)))
	})
}

func (a *authServer) login(w http.ResponseWriter, r *http.Request) {
	if a.svc == nil {
		writeError(w, pos.ErrNoAutenticado)
		return
	}
	var in struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	res, err := a.svc.Login(r.Context(), in.Login, in.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	a.setSessionCookie(w, res.Token, res.ExpiraEn)
	writeJSON(w, http.StatusOK, map[string]any{"usuario": res.Usuario})
}

func (a *authServer) logout(w http.ResponseWriter, r *http.Request) {
	token := a.sessionToken(r)
	if a.svc != nil {
		_ = a.svc.Logout(r.Context(), token)
	}
	a.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (a *authServer) me(w http.ResponseWriter, r *http.Request) {
	u := getUser(r)
	if u == nil {
		writeError(w, pos.ErrNoAutenticado)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"usuario": u})
}

func (a *authServer) listUsuarios(w http.ResponseWriter, r *http.Request) {
	u := getUser(r)
	if u == nil {
		writeError(w, pos.ErrNoAutenticado)
		return
	}
	if u.Rol != "dueno" && u.Rol != "administrador" {
		writeError(w, pos.ErrNoAutorizado)
		return
	}
	incluir, err := queryBool(r, "incluir_inactivos")
	if err != nil {
		writeError(w, err)
		return
	}
	lista, err := a.svc.ListUsuarios(r.Context(), incluir)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"usuarios": lista})
}

func (a *authServer) createUsuario(w http.ResponseWriter, r *http.Request) {
	u := getUser(r)
	if u == nil {
		writeError(w, pos.ErrNoAutenticado)
		return
	}
	var in pos.CreateUsuarioInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.UsuarioID = u.ID
	usuario, err := a.svc.CreateUsuario(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, usuario)
}

func (a *authServer) updateUsuario(w http.ResponseWriter, r *http.Request) {
	u := getUser(r)
	if u == nil {
		writeError(w, pos.ErrNoAutenticado)
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var in pos.UpdateUsuarioInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.UsuarioID = u.ID
	usuario, err := a.svc.UpdateUsuario(r.Context(), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, usuario)
}

// SessionCookieSecure reports whether the cookie should be marked Secure.
// Exported for tests and configuration inspection.
func SessionCookieSecure() bool {
	return strings.EqualFold(os.Getenv("SESSION_COOKIE_SECURE"), "true")
}

// LoadSessionTTL parses SESSION_TTL (a Go duration) with a safe 12h default.
func LoadSessionTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv("SESSION_TTL"))
	if raw == "" {
		return pos.DefaultSessionTTL
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return pos.DefaultSessionTTL
	}
	return d
}
