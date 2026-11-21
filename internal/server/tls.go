package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// TLSConfig 는 TLS_MODE 에 맞는 설정을 만든다. off 면 nil 을 돌려준다.
// 인증서의 SHA-256 지문을 기록해 두어, 에이전트 설치 화면에서 지문 고정 값으로 보여 준다.
func (s *Server) TLSConfig() (*tls.Config, error) {
	var certFile, keyFile string
	switch s.cfg.TLSMode {
	case "off":
		return nil, nil
	case "files":
		certFile, keyFile = s.cfg.TLSCertFile, s.cfg.TLSKeyFile
	default: // selfsigned
		dir := filepath.Join(s.cfg.DataDir, "tls")
		certFile, keyFile = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
		if _, err := os.Stat(certFile); errors.Is(err, os.ErrNotExist) {
			if err := generateSelfSigned(dir, certFile, keyFile, s.cfg.TLSHosts); err != nil {
				return nil, err
			}
			s.logger.Printf("자체 서명 인증서를 만들었습니다: %s", certFile)
		}
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(cert.Certificate[0])
	s.certSHA = hex.EncodeToString(sum[:])
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, nil
}

// CertSHA256 은 서버 인증서 지문이다. TLS 를 쓰지 않으면 빈 문자열.
func (s *Server) CertSHA256() string { return s.certSHA }

func generateSelfSigned(dir, certFile, keyFile string, hosts []string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return err
	}
	now := time.Now()
	tpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "infra-scanner", Organization: []string{"Infra Scanner"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(3, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tpl.IPAddresses = append(tpl.IPAddresses, ip)
		} else {
			tpl.DNSNames = append(tpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return err
	}
	return os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600)
}
