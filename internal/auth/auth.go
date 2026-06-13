package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName = "wgui_session"
	sessionTTL = 12 * time.Hour
)

type session struct {
	user    string
	expires time.Time
}

// credFile is the on-disk credential format (bcrypt hash, never plaintext).
type credFile struct {
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
}

// Manager holds sessions and the admin credentials. Credentials persist to a
// JSON file so a password changed via the UI survives restarts and overrides
// the bootstrap env values.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]session

	credPath     string
	username     string
	passwordHash []byte
}

// NewManager loads credentials from credPath if present, otherwise seeds them
// from the bootstrap env username/password (hashing the password).
func NewManager(credPath, envUser, envPass string) (*Manager, error) {
	m := &Manager{
		sessions: make(map[string]session),
		credPath: credPath,
	}
	if raw, err := os.ReadFile(credPath); err == nil {
		var cf credFile
		if err := json.Unmarshal(raw, &cf); err != nil {
			return nil, err
		}
		m.username = cf.Username
		m.passwordHash = []byte(cf.PasswordHash)
		return m, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(envPass), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	m.username = envUser
	m.passwordHash = hash
	return m, nil
}

// Verify checks credentials: constant-time username compare + bcrypt password.
func (m *Manager) Verify(user, pass string) bool {
	m.mu.Lock()
	username := m.username
	hash := m.passwordHash
	m.mu.Unlock()

	uOK := subtle.ConstantTimeCompare([]byte(user), []byte(username)) == 1
	pOK := bcrypt.CompareHashAndPassword(hash, []byte(pass)) == nil
	return uOK && pOK
}

// ChangeCredentials verifies the current password, then updates the username
// (kept if empty) and password, persisting the new bcrypt hash.
func (m *Manager) ChangeCredentials(currentPass, newUser, newPass string) error {
	if newPass == "" {
		return errors.New("新密码不能为空")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if bcrypt.CompareHashAndPassword(m.passwordHash, []byte(currentPass)) != nil {
		return errors.New("当前密码不正确")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if newUser != "" {
		m.username = newUser
	}
	m.passwordHash = hash
	return m.saveCreds()
}

// saveCreds persists credentials atomically (0600). Caller holds m.mu.
func (m *Manager) saveCreds() error {
	raw, err := json.MarshalIndent(credFile{
		Username:     m.username,
		PasswordHash: string(m.passwordHash),
	}, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(m.credPath)
	tmp, err := os.CreateTemp(dir, ".cred-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, m.credPath)
}

func newToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Issue creates a session and writes the cookie.
func (m *Manager) Issue(w http.ResponseWriter, user string) {
	token := newToken()
	m.mu.Lock()
	m.sessions[token] = session{user: user, expires: time.Now().Add(sessionTTL)}
	m.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionTTL.Seconds()),
		// Secure is intentionally omitted: the panel is expected to sit behind
		// a TLS-terminating reverse proxy. Enable when serving HTTPS directly.
	})
}

// Current returns the logged-in user for a request, or "" if unauthenticated.
func (m *Manager) Current(r *http.Request) string {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[c.Value]
	if !ok || time.Now().After(s.expires) {
		delete(m.sessions, c.Value)
		return ""
	}
	return s.user
}

// Clear destroys the session referenced by the request cookie.
func (m *Manager) Clear(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		m.mu.Lock()
		delete(m.sessions, c.Value)
		m.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// Require wraps a handler, returning 401 when no valid session is present.
func (m *Manager) Require(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if m.Current(r) == "" {
			http.Error(w, `{"error":"未登录或会话已过期"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
