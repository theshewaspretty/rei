package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"infrascanner/internal/model"
)

// ErrUnauthorized 는 서버가 에이전트 토큰을 거부했다는 뜻이다.
var ErrUnauthorized = errors.New("서버가 에이전트 인증을 거부했습니다")

type Client struct {
	base *url.URL
	http *http.Client
}

func NewClient(cfg Config) (*Client, error) {
	base, err := url.Parse(strings.TrimRight(cfg.Server, "/"))
	if err != nil {
		return nil, err
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("ca_file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("ca_file 에 인증서가 없습니다: %s", cfg.CAFile)
		}
		tlsCfg.RootCAs = pool
	}
	if cfg.ServerCertSHA256 != "" {
		want, _ := hex.DecodeString(cfg.ServerCertSHA256)
		// 지문 고정: 체인 검증 대신 서버 인증서 자체가 설치 시 받은 지문과 같은지만 본다.
		tlsCfg.InsecureSkipVerify = true
		tlsCfg.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("서버 인증서가 없습니다")
			}
			sum := sha256.Sum256(cs.PeerCertificates[0].Raw)
			if subtle.ConstantTimeCompare(sum[:], want) != 1 {
				return fmt.Errorf("서버 인증서 지문이 설정값과 다릅니다 (받은 값 %x)", sum)
			}
			return nil
		}
	}
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSClientConfig:       tlsCfg,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          2,
	}
	return &Client{base: base, http: &http.Client{Transport: tr, Timeout: 30 * time.Second}}, nil
}

// HostPort 는 기본 주소 판별에 쓰는 서버의 host:port 다.
func (c *Client) HostPort() string {
	if c.base.Port() != "" {
		return c.base.Host
	}
	if c.base.Scheme == "http" {
		return net.JoinHostPort(c.base.Hostname(), "80")
	}
	return net.JoinHostPort(c.base.Hostname(), "443")
}

func (c *Client) Enroll(ctx context.Context, req model.EnrollRequest) (model.EnrollResponse, error) {
	var resp model.EnrollResponse
	err := c.post(ctx, "/api/v1/agent/enroll", "", req, &resp)
	if err == nil && (resp.AgentID == "" || resp.AgentToken == "") {
		err = errors.New("서버 응답에 에이전트 자격이 없습니다")
	}
	return resp, err
}

func (c *Client) Report(ctx context.Context, st State, r model.Report) error {
	return c.post(ctx, "/api/v1/agent/report", st.AgentID+"."+st.AgentToken, r, nil)
}

func (c *Client) post(ctx context.Context, path, bearer string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base.String()+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "infra-agent/"+model.Version)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%w: %s", ErrUnauthorized, strings.TrimSpace(string(data)))
	case resp.StatusCode/100 != 2:
		return fmt.Errorf("서버 응답 %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}
