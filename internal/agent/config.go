package agent

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Config 는 agent.json 의 내용이다.
type Config struct {
	Server           string `json:"server"`                       // 예: https://scanner.dc.local:8443
	EnrollToken      string `json:"enroll_token,omitempty"`       // 등록에 성공하면 파일에서 지운다
	Zone             string `json:"zone,omitempty"`               // 구성도에서 묶을 영역 이름
	IntervalSec      int    `json:"interval_sec"`                 // 보고 주기
	SampleSec        int    `json:"sample_sec"`                   // 소켓 샘플링 주기
	CAFile           string `json:"ca_file,omitempty"`            // 사내 CA 인증서(PEM)
	ServerCertSHA256 string `json:"server_cert_sha256,omitempty"` // 자체 서명 인증서 지문 고정
	AllowHTTP        bool   `json:"allow_http,omitempty"`         // 평문 HTTP 허용(시험용)
}

// State 는 등록 후 발급받은 에이전트 자격이다.
type State struct {
	AgentID    string `json:"agent_id"`
	AgentToken string `json:"agent_token"`
}

func DefaultDir() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("ProgramData")
		if base == "" {
			base = `C:\ProgramData`
		}
		return filepath.Join(base, "InfraScanner")
	}
	return "/etc/infra-scanner"
}

func (c *Config) applyDefaults() {
	if c.IntervalSec == 0 {
		c.IntervalSec = 60
	}
	if c.SampleSec == 0 {
		c.SampleSec = 10
	}
}

func (c *Config) Validate() error {
	c.applyDefaults()
	u, err := url.Parse(c.Server)
	if err != nil || u.Host == "" {
		return fmt.Errorf("server 주소가 올바르지 않습니다: %q", c.Server)
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !c.AllowHTTP {
			return errors.New("http:// 서버는 토큰과 수집 정보가 평문으로 전송됩니다. https:// 를 쓰거나, 시험 환경이면 -allow-http 를 지정하세요")
		}
	default:
		return fmt.Errorf("server 주소는 https:// 로 시작해야 합니다: %q", c.Server)
	}
	if c.IntervalSec < 15 || c.IntervalSec > 3600 {
		return fmt.Errorf("interval_sec 은 15~3600 사이여야 합니다: %d", c.IntervalSec)
	}
	if c.SampleSec < 2 || c.SampleSec > c.IntervalSec {
		return fmt.Errorf("sample_sec 은 2 이상, interval_sec 이하여야 합니다: %d", c.SampleSec)
	}
	if c.ServerCertSHA256 != "" {
		fp := normFingerprint(c.ServerCertSHA256)
		if b, err := hex.DecodeString(fp); err != nil || len(b) != 32 {
			return errors.New("server_cert_sha256 은 SHA-256 지문(16진수 64자)이어야 합니다")
		}
		c.ServerCertSHA256 = fp
	}
	return nil
}

func normFingerprint(s string) string {
	return strings.ToLower(strings.NewReplacer(":", "", " ", "").Replace(strings.TrimSpace(s)))
}

func LoadConfig(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, c.Validate()
}

func SaveConfig(path string, c Config) error {
	return writeJSON(path, c)
}

func statePath(dir string) string { return filepath.Join(dir, "state.json") }

func LoadState(dir string) (State, error) {
	var s State
	b, err := os.ReadFile(statePath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

func SaveState(dir string, s State) error {
	return writeJSON(statePath(dir), s)
}

// writeJSON 은 임시 파일에 쓴 뒤 이름을 바꿔, 중간에 끊겨도 반쯤 쓰인 파일이 남지 않게 한다.
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
