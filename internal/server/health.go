package server

import (
	"bytes"
	"crypto/tls"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Healthcheck 는 같은 컨테이너에서 실행 중인 서버의 /healthz 를 확인한다 (Docker HEALTHCHECK 용).
// 셸도 curl 도 없는 이미지라서 서버 실행 파일이 직접 확인한다.
func Healthcheck() error {
	env := func(key, def string) string {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
		return def
	}
	host, port, err := net.SplitHostPort(env("LISTEN_ADDR", ":8443"))
	if err != nil {
		return fmt.Errorf("LISTEN_ADDR: %w", err)
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}

	client := &http.Client{Timeout: 4 * time.Second}
	scheme := "http"
	if mode := strings.ToLower(env("TLS_MODE", "selfsigned")); mode != "off" {
		scheme = "https"
		certFile := env("TLS_CERT_FILE", "")
		if mode != "files" {
			certFile = filepath.Join(env("DATA_DIR", "./data"), "tls", "cert.pem")
		}
		raw, err := os.ReadFile(certFile)
		if err != nil {
			return err
		}
		block, _ := pem.Decode(raw)
		if block == nil {
			return fmt.Errorf("%s: PEM 인증서가 아닙니다", certFile)
		}
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// 인증서의 이름은 외부 주소용이라 127.0.0.1 과 맞지 않는다.
			// 이름 검증 대신, 상대가 내놓은 인증서가 이 서버의 인증서 파일과 같은지 직접 비교한다.
			InsecureSkipVerify: true,
			VerifyConnection: func(cs tls.ConnectionState) error {
				if len(cs.PeerCertificates) == 0 || !bytes.Equal(cs.PeerCertificates[0].Raw, block.Bytes) {
					return errors.New("서버 인증서가 설정된 인증서 파일과 다릅니다")
				}
				return nil
			},
		}}
	}
	resp, err := client.Get(scheme + "://" + net.JoinHostPort(host, port) + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("/healthz 응답 %d", resp.StatusCode)
	}
	return nil
}
