// infra-server: 에이전트 보고를 받아 구성도와 점검 결과를 제공하는 웹 서버.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"infrascanner/internal/model"
	"infrascanner/internal/server"
	"infrascanner/web"
)

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck": // 컨테이너 HEALTHCHECK 에서 호출한다
			if err := server.Healthcheck(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "version":
			fmt.Println(model.Version)
			return
		default:
			logger.Fatalf("알 수 없는 명령: %s (설정은 환경 변수로 합니다. README 참고)", os.Args[1])
		}
	}

	cfg, err := server.ConfigFromEnv()
	if err != nil {
		logger.Fatalf("설정 오류: %v", err)
	}
	srv, err := server.New(cfg, web.Static(), logger)
	if err != nil {
		logger.Fatalf("시작 실패: %v", err)
	}
	defer srv.Close()

	tlsCfg, err := srv.TLSConfig()
	if err != nil {
		logger.Fatalf("TLS 설정 실패: %v", err)
	}

	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          logger,
	}

	stop := make(chan struct{})
	go srv.PruneLoop(stop)

	errc := make(chan error, 1)
	go func() {
		if tlsCfg != nil {
			logger.Printf("infra-server %s: https://%s (TLS_MODE=%s)", model.Version, cfg.ListenAddr, cfg.TLSMode)
			logger.Printf("서버 인증서 SHA-256 지문: %s", srv.CertSHA256())
			errc <- httpSrv.ListenAndServeTLS("", "")
			return
		}
		logger.Printf("infra-server %s: http://%s (TLS_MODE=off)", model.Version, cfg.ListenAddr)
		logger.Printf("경고: TLS 가 꺼져 있습니다. 토큰과 수집 정보가 평문으로 오가므로, 앞단에 TLS 를 끝내는 프록시가 있을 때만 쓰세요.")
		errc <- httpSrv.ListenAndServe()
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("서버 중단: %v", err)
		}
	case <-sig:
		logger.Printf("종료 신호를 받았습니다")
	}
	close(stop)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpSrv.Shutdown(ctx)
}
