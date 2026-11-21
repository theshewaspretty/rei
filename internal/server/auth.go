package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookie   = "infra_session"
	sessionLifetime = 12 * time.Hour
	csrfHeader      = "X-Requested-With"
	csrfValue       = "infra-scanner"
	maxLoginFails   = 5
	loginLockout    = 5 * time.Minute
)

// ---- 관리자 세션 --------------------------------------------------------------

// sessionStore 는 메모리에만 세션을 둔다. 서버를 다시 시작하면 다시 로그인해야 한다.
type sessionStore struct {
	mu sync.Mutex
	m  map[string]time.Time // 세션 ID -> 만료 시각
}

func newSessionStore() *sessionStore { return &sessionStore{m: map[string]time.Time{}} }

func (s *sessionStore) create() string {
	id := randomToken(32)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, exp := range s.m {
		if now.After(exp) {
			delete(s.m, k)
		}
	}
	s.m[id] = now.Add(sessionLifetime)
	return id
}

func (s *sessionStore) valid(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.m[id]
	return ok && time.Now().Before(exp)
}

func (s *sessionStore) drop(id string) {
	s.mu.Lock()
	delete(s.m, id)
	s.mu.Unlock()
}

// ---- 로그인 시도 제한 ----------------------------------------------------------

type loginLimiter struct {
	mu sync.Mutex
	m  map[string]*loginState
}

type loginState struct {
	fails int
	until time.Time
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{m: map[string]*loginState{}} }

// blocked 는 잠금이 풀릴 때까지 남은 시간을 돌려준다.
func (l *loginLimiter) blocked(ip string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if st, ok := l.m[ip]; ok {
		if d := time.Until(st.until); d > 0 {
			return d
		}
	}
	return 0
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.m[ip]
	if st == nil {
		st = &loginState{}
		l.m[ip] = st
	}
	st.fails++
	if st.fails >= maxLoginFails {
		st.fails = 0
		st.until = time.Now().Add(loginLockout)
	}
}

func (l *loginLimiter) success(ip string) {
	l.mu.Lock()
	delete(l.m, ip)
	l.mu.Unlock()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func constEq(a, b string) bool {
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

// ---- 핸들러 -------------------------------------------------------------------

// admin 은 관리자 세션이 있어야 통과시킨다. 값을 바꾸는 요청은 전용 헤더까지 확인해
// 다른 사이트가 쿠키만으로 요청을 보내는 것을 막는다.
func (s *Server) admin(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || !s.sessions.valid(c.Value) {
			writeError(w, http.StatusUnauthorized, "login_required", "로그인이 필요합니다")
			return
		}
		if r.Method != http.MethodGet && r.Header.Get(csrfHeader) != csrfValue {
			writeError(w, http.StatusForbidden, "forbidden", "허용되지 않은 요청입니다")
			return
		}
		next(w, r)
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if d := s.limiter.blocked(ip); d > 0 {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"code":            "too_many_logins",
			"error":           "로그인 시도가 너무 많습니다. " + d.Round(time.Second).String() + " 뒤에 다시 시도하세요",
			"retry_after_sec": int(d/time.Second) + 1,
		})
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if r.Header.Get(csrfHeader) != csrfValue || json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "요청 형식이 올바르지 않습니다")
		return
	}
	if !constEq(req.Password, s.cfg.AdminPassword) {
		s.limiter.fail(ip)
		s.logger.Printf("관리자 로그인 실패: %s", ip)
		writeError(w, http.StatusUnauthorized, "bad_password", "비밀번호가 일치하지 않습니다")
		return
	}
	s.limiter.success(ip)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: s.sessions.create(), Path: "/",
		MaxAge: int(sessionLifetime / time.Second), HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.sessions.drop(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// authAgent 는 "Authorization: Bearer <agent_id>.<token>" 을 확인한다.
func (s *Server) authAgent(r *http.Request) (*AgentRecord, bool) {
	bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return nil, false
	}
	id, token, ok := strings.Cut(bearer, ".")
	if !ok {
		return nil, false
	}
	s.mu.RLock()
	a := s.agents[id]
	s.mu.RUnlock()
	if a == nil || subtle.ConstantTimeCompare(a.tokenHash, hashToken(token)) != 1 {
		return nil, false
	}
	return a, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError 의 code 는 언어와 무관한 식별자다. 관제 화면은 code 로 선택된 언어의 문장을 고르고
// (web/static/i18n.js 의 err.<code>), msg 는 에이전트 로그와 API 를 직접 쓰는 사람을 위한 설명이다.
func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"code": code, "error": msg})
}
