package api

import (
	"errors"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/podhmo/go-spa-sample/internal/auth"
	"github.com/podhmo/go-spa-sample/internal/store"
)

const (
	// sessionCookieName carries the opaque session token; only its
	// SHA-256 is stored. Cookie attrs mirror the MPA (HttpOnly, Lax, no
	// Secure — the app runs on plain-HTTP localhost).
	sessionCookieName = "session"
	// sessionTTL is the server-side lifetime; the cookie is a browser
	// session cookie and the store renews slidingly on reads.
	sessionTTL = 30 * 24 * time.Hour
	// Password/name bounds mirror the HTML register form.
	maxNameRunes      = 100
	minPasswordRunes  = 8
	maxPasswordRunes  = 256
	failedLoginDelay  = 200 * time.Millisecond
	sessionCookiePath = "/"
)

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     sessionCookiePath,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     sessionCookiePath,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

type authBody struct {
	Name     *string `json:"name"`
	Password *string `json:"password"`
}

// POST /api/v1/auth/register {"name","password"} → 201 {"user"}.
// Registration logs in immediately, like the MPA's form: a fresh session
// cookie rides the response.
func (h *handler) register(w http.ResponseWriter, r *http.Request) {
	var in authBody
	if !decodeBody(w, r, &in) {
		return
	}
	name, password := deref(in.Name), deref(in.Password)
	fields := map[string]string{}
	if name == "" {
		fields["name"] = "Name is required."
	} else if utf8.RuneCountInString(name) > maxNameRunes {
		fields["name"] = "Name is too long (100 characters max)."
	}
	switch {
	case password == "":
		fields["password"] = "Password is required."
	case utf8.RuneCountInString(password) < minPasswordRunes:
		fields["password"] = "Password is too short (8 characters min)."
	case utf8.RuneCountInString(password) > maxPasswordRunes:
		fields["password"] = "Password is too long (256 characters max)."
	}
	if len(fields) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", fields)
		return
	}
	// Check the name before hashing: argon2 is deliberately expensive and
	// this endpoint is public, so obviously-taken names must be cheap.
	// The insert below can still race — ErrDuplicateName stays the
	// authoritative rejection.
	if _, err := h.store.UserByName(r.Context(), name); err == nil {
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.",
			map[string]string{"name": "This name is already taken."})
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
		return
	}
	userID, err := h.store.CreateUser(r.Context(), name, hash)
	if err != nil {
		if errors.Is(err, store.ErrDuplicateName) {
			writeError(w, http.StatusUnprocessableEntity, "Validation failed.",
				map[string]string{"name": "This name is already taken."})
		} else {
			writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
		}
		return
	}
	token, err := h.store.CreateSession(r.Context(), userID, sessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
		return
	}
	setSessionCookie(w, token)
	u, err := h.store.UserByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": userDTO(u)})
}

// POST /api/v1/auth/login {"name","password"} → 200 {"user"}.
// Deliberately vague failures + a fixed delay, same as the HTML handler.
func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	var in authBody
	if !decodeBody(w, r, &in) {
		return
	}
	name, password := deref(in.Name), deref(in.Password)
	if name == "" || password == "" {
		fields := map[string]string{}
		if name == "" {
			fields["name"] = "Name is required."
		}
		if password == "" {
			fields["password"] = "Password is required."
		}
		writeError(w, http.StatusUnprocessableEntity, "Validation failed.", fields)
		return
	}
	user, err := h.store.AuthenticateUser(r.Context(), name, password)
	if err != nil {
		if errors.Is(err, store.ErrInvalidCredentials) {
			time.Sleep(failedLoginDelay)
			writeError(w, http.StatusUnprocessableEntity, "Invalid name or password.", nil)
		} else {
			writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
		}
		return
	}
	token, err := h.store.CreateSession(r.Context(), user.ID, sessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
		return
	}
	setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{"user": userDTO(user)})
}

// POST /api/v1/auth/logout → 204. Missing sessions are a no-op; the
// cookie is expired either way.
func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		_ = h.store.DeleteSession(r.Context(), c.Value)
	}
	clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/auth/me → 200 {"user"} for the session's user, or 401 —
// an anonymous (shared-token) request has no identity to report either.
func (h *handler) me(w http.ResponseWriter, r *http.Request) {
	id := actorID(r)
	if id == 0 {
		writeError(w, http.StatusUnauthorized, "Unauthorized.", nil)
		return
	}
	u, err := h.store.UserByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal server error.", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": userDTO(u)})
}
