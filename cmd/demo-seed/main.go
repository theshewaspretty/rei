// demo-seed: 가상의 서버 여러 대를 흉내 내어 수집 서버에 등록·보고한다.
// 실제 에이전트와 같은 API 를 쓰므로, 화면 시연과 서버 동작 확인에 쓴다. 운영 서버에는 실행하지 말 것.
// 가상 서버의 정의는 internal/demo 에 있다.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"infrascanner/internal/agent"
	"infrascanner/internal/demo"
	"infrascanner/internal/model"
)

func main() {
	server := flag.String("server", "https://localhost:8443", "수집 서버 주소")
	token := flag.String("token", os.Getenv("ENROLL_TOKEN"), "등록 토큰 (기본: 환경 변수 ENROLL_TOKEN)")
	pin := flag.String("server-cert-sha256", "", "서버 인증서 SHA-256 지문")
	allowHTTP := flag.Bool("allow-http", false, "평문 HTTP 허용")
	loop := flag.Bool("loop", false, "20초마다 계속 보고")
	resume := flag.Bool("resume", false, "이전 실행에 이어서: 보고를 멈춘 것으로 시연하는 서버(LEGACY-ERP)는 다시 보고하지 않음")
	flag.Parse()
	if *token == "" {
		log.Fatal("-token 또는 환경 변수 ENROLL_TOKEN 이 필요합니다")
	}

	cfg := agent.Config{Server: *server, EnrollToken: *token, ServerCertSHA256: *pin, AllowHTTP: *allowHTTP, IntervalSec: demo.IntervalSec, SampleSec: 5}
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}
	client, err := agent.NewClient(cfg)
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	hosts := demo.Hosts()
	states := map[string]agent.State{}
	for _, h := range hosts {
		res, err := client.Enroll(ctx, model.EnrollRequest{Token: *token, MachineID: "demo:" + h.Name, Hostname: h.Name})
		if err != nil {
			log.Fatalf("%s 등록 실패: %v", h.Name, err)
		}
		states[h.Name] = agent.State{AgentID: res.AgentID, AgentToken: res.AgentToken}
	}
	log.Printf("가상 서버 %d대 등록", len(hosts))

	send := func(withFlows bool, first bool) error {
		for _, h := range hosts {
			if h.StopsReporting && !first {
				continue
			}
			if err := client.Report(ctx, states[h.Name], h.Report(time.Now(), withFlows)); err != nil {
				return fmt.Errorf("%s 보고 실패: %w", h.Name, err)
			}
		}
		return nil
	}
	// 첫 보고는 주소만 알린다. 서버가 모든 주소를 알고 난 뒤에 흐름을 보내야 서버끼리의 연결로 묶인다.
	for _, withFlows := range []bool{false, true} {
		if err := send(withFlows, !*resume); err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("보고 완료")
	if !*loop {
		return
	}
	log.Printf("%d초마다 계속 보고합니다 (Ctrl+C 로 종료). LEGACY-ERP 는 보고를 멈춰 약 3분 뒤 '응답 없음'이 됩니다.", demo.IntervalSec)
	t := time.NewTicker(demo.IntervalSec * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := send(true, false); err != nil && ctx.Err() == nil {
				log.Printf("%v (다음 주기에 다시 시도)", err)
			}
		}
	}
}
