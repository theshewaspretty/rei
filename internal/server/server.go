// Package server 는 에이전트 보고를 받아 저장하고, 구성도와 점검 결과를 웹으로 제공한다.
package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	ListenAddr    string
	DataDir       string
	AdminPassword string
	EnrollToken   string
	TLSMode       string // selfsigned, files, off
	TLSCertFile   string
	TLSKeyFile    string
	TLSHosts      []string
	CookieSecure  bool
	RetentionDays int
	DownloadsDir  string
}

// ConfigFromEnv 는 환경 변수에서 설정을 읽고, 운영에 쓰면 안 되는 값은 거부한다.
func ConfigFromEnv() (Config, error) {
	env := func(key, def string) string {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
		return def
	}
	c := Config{
		ListenAddr:    env("LISTEN_ADDR", ":8443"),
		DataDir:       env("DATA_DIR", "./data"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
		EnrollToken:   os.Getenv("ENROLL_TOKEN"),
		TLSMode:       strings.ToLower(env("TLS_MODE", "selfsigned")),
		TLSCertFile:   env("TLS_CERT_FILE", ""),
		TLSKeyFile:    env("TLS_KEY_FILE", ""),
		DownloadsDir:  env("DOWNLOADS_DIR", "./downloads"),
	}
	for _, h := range strings.Split(env("TLS_HOSTS", ""), ",") {
		if h = strings.TrimSpace(h); h != "" {
			c.TLSHosts = append(c.TLSHosts, h)
		}
	}
	var err error
	if c.RetentionDays, err = strconv.Atoi(env("RETENTION_DAYS", "30")); err != nil || c.RetentionDays < 1 {
		return c, errors.New("RETENTION_DAYS 는 1 이상의 숫자여야 합니다")
	}
	if c.CookieSecure, err = strconv.ParseBool(env("COOKIE_SECURE", "true")); err != nil {
		return c, errors.New("COOKIE_SECURE 는 true 또는 false 여야 합니다")
	}
	if len(c.AdminPassword) < 12 {
		return c, errors.New("ADMIN_PASSWORD 를 12자 이상으로 설정하세요")
	}
	if len(c.EnrollToken) < 24 {
		return c, errors.New("ENROLL_TOKEN 을 24자 이상의 무작위 값으로 설정하세요")
	}
	switch c.TLSMode {
	case "selfsigned", "off":
	case "files":
		if c.TLSCertFile == "" || c.TLSKeyFile == "" {
			return c, errors.New("TLS_MODE=files 에는 TLS_CERT_FILE 과 TLS_KEY_FILE 이 필요합니다")
		}
	default:
		return c, fmt.Errorf("TLS_MODE 는 selfsigned, files, off 중 하나여야 합니다: %q", c.TLSMode)
	}
	return c, nil
}

type download struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Server struct {
	cfg    Config
	store  *Store
	logger *log.Logger
	static fs.FS

	mu      sync.RWMutex
	agents  map[string]*AgentRecord
	ipIndex map[netip.Addr]string // 주소 -> 에이전트 ID. 여러 서버가 같이 가진 주소는 넣지 않는다

	sessions  *sessionStore
	limiter   *loginLimiter
	downloads []download
	certSHA   string // 서버 인증서 SHA-256 지문 (TLS 사용 시)

	topoMu    sync.Mutex
	topoCache map[time.Duration]cachedTopo
}

type cachedTopo struct {
	at   time.Time
	topo *Topology
}

func New(cfg Config, static fs.FS, logger *log.Logger) (*Server, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, err
	}
	store, err := OpenStore(filepath.Join(cfg.DataDir, "infra-scanner.db"))
	if err != nil {
		return nil, fmt.Errorf("DB 열기 실패: %w", err)
	}
	s := &Server{
		cfg: cfg, store: store, logger: logger, static: static,
		agents:    map[string]*AgentRecord{},
		sessions:  newSessionStore(),
		limiter:   newLoginLimiter(),
		topoCache: map[time.Duration]cachedTopo{},
	}
	agents, err := store.LoadAgents()
	if err != nil {
		store.Close()
		return nil, err
	}
	for _, a := range agents {
		s.agents[a.ID] = a
	}
	s.rebuildIPIndexLocked()
	s.downloads = scanDownloads(cfg.DownloadsDir)
	return s, nil
}

func (s *Server) Close() error { return s.store.Close() }

// PruneLoop 는 보존 기간이 지난 흐름을 하루에 한 번 지운다.
func (s *Server) PruneLoop(stop <-chan struct{}) {
	prune := func() {
		before := time.Now().AddDate(0, 0, -s.cfg.RetentionDays)
		if n, err := s.store.PruneFlows(before); err != nil {
			s.logger.Printf("흐름 정리 실패: %v", err)
		} else if n > 0 {
			s.logger.Printf("보존 기간(%d일)이 지난 흐름 %d건 삭제", s.cfg.RetentionDays, n)
		}
	}
	prune()
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			prune()
		}
	}
}

// rebuildIPIndexLocked 는 각 에이전트가 보고한 인터페이스 주소로 주소 -> 에이전트 표를 다시 만든다.
// docker0 처럼 여러 서버가 똑같이 가진 주소는 누구인지 알 수 없으므로 뺀다.
func (s *Server) rebuildIPIndexLocked() {
	idx := map[netip.Addr]string{}
	dup := map[netip.Addr]bool{}
	for id, a := range s.agents {
		if a.Report == nil {
			continue
		}
		for _, ifc := range a.Report.Interfaces {
			for _, cidr := range ifc.Addrs {
				p, err := netip.ParsePrefix(cidr)
				if err != nil {
					continue
				}
				ip := p.Addr().Unmap()
				if ip.IsLoopback() || ip.IsLinkLocalUnicast() || dup[ip] {
					continue
				}
				if other, ok := idx[ip]; ok && other != id {
					delete(idx, ip)
					dup[ip] = true
					continue
				}
				idx[ip] = id
			}
		}
	}
	s.ipIndex = idx
}

func (s *Server) snapshot() ([]*AgentRecord, map[netip.Addr]string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agents := make([]*AgentRecord, 0, len(s.agents))
	for _, a := range s.agents {
		agents = append(agents, a)
	}
	sort.Slice(agents, func(i, j int) bool {
		if agents[i].Hostname != agents[j].Hostname {
			return agents[i].Hostname < agents[j].Hostname
		}
		return agents[i].ID < agents[j].ID
	})
	return agents, s.ipIndex
}

func (s *Server) invalidateTopology() {
	s.topoMu.Lock()
	s.topoCache = map[time.Duration]cachedTopo{}
	s.topoMu.Unlock()
}

func scanDownloads(dir string) []download {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []download
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "infra-agent") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		sum := sha256.Sum256(b)
		out = append(out, download{Name: e.Name(), Size: int64(len(b)), SHA256: hex.EncodeToString(sum[:])})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func randomToken(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		panic(err) // 난수원을 못 쓰면 토큰을 만들 수 없다
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// status 는 마지막 보고 시각으로 온라인 여부를 판단한다. 보고 주기의 3배(최소 3분) 동안 소식이 없으면 offline.
func agentStatus(a *AgentRecord, now time.Time) string {
	if a.Report == nil || a.LastSeen.IsZero() {
		return "pending"
	}
	grace := 3 * time.Duration(a.Report.IntervalSec) * time.Second
	if grace < 3*time.Minute {
		grace = 3 * time.Minute
	}
	if now.Sub(a.LastSeen) > grace {
		return "offline"
	}
	return "online"
}

// Handler 는 전체 라우팅을 만든다.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok\n")) })

	// 에이전트
	mux.HandleFunc("POST /api/v1/agent/enroll", s.handleEnroll)
	mux.HandleFunc("POST /api/v1/agent/report", s.handleReport)

	// 관리자
	mux.HandleFunc("POST /api/v1/login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/logout", s.handleLogout)
	mux.Handle("GET /api/v1/me", s.admin(s.handleMe))
	mux.Handle("GET /api/v1/topology", s.admin(s.handleTopology))
	mux.Handle("GET /api/v1/hosts/{id}", s.admin(s.handleHost))
	mux.Handle("DELETE /api/v1/hosts/{id}", s.admin(s.handleDeleteHost))
	mux.Handle("PUT /api/v1/layout", s.admin(s.handleSaveLayout))
	mux.Handle("DELETE /api/v1/layout", s.admin(s.handleResetLayout))
	mux.Handle("GET /api/v1/install-info", s.admin(s.handleInstallInfo))

	// 에이전트 실행 파일. 설치 대상 서버에서 받아야 하므로 로그인 없이 내려준다.
	mux.HandleFunc("GET /downloads/{name}", s.handleDownload)

	mux.Handle("GET /", http.FileServerFS(s.static))

	return s.securityHeaders(mux)
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'")
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
