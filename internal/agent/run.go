package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"

	"github.com/shirou/gopsutil/v4/host"

	"infrascanner/internal/model"
)

// Enroll 은 등록 토큰으로 서버에 이 장비를 등록하고, 발급받은 자격을 저장한 뒤
// 설정 파일에서 등록 토큰을 지운다. 이미 등록돼 있으면 아무것도 하지 않는다.
func Enroll(ctx context.Context, cfgPath string, cfg *Config, client *Client) (State, error) {
	dir := filepath.Dir(cfgPath)
	st, err := LoadState(dir)
	if err != nil {
		return st, fmt.Errorf("state.json 읽기 실패: %w", err)
	}
	if st.AgentID != "" && st.AgentToken != "" {
		return st, nil
	}
	if cfg.EnrollToken == "" {
		return st, errors.New("등록 정보가 없습니다. install -token 으로 다시 등록하세요")
	}

	hostname, _ := os.Hostname()
	machineID := hostname
	if info, err := host.InfoWithContext(ctx); err == nil && info.HostID != "" {
		// 템플릿으로 복제한 VM 은 장비 ID 가 같을 수 있어 호스트 이름을 함께 쓴다.
		machineID = info.HostID + ":" + hostname
	}
	resp, err := client.Enroll(ctx, model.EnrollRequest{Token: cfg.EnrollToken, MachineID: machineID, Hostname: hostname})
	if err != nil {
		return st, fmt.Errorf("등록 실패: %w", err)
	}
	st = State{AgentID: resp.AgentID, AgentToken: resp.AgentToken}
	if err := SaveState(dir, st); err != nil {
		return st, fmt.Errorf("state.json 저장 실패: %w", err)
	}
	cfg.EnrollToken = ""
	if err := SaveConfig(cfgPath, *cfg); err != nil {
		return st, fmt.Errorf("agent.json 저장 실패: %w", err)
	}
	return st, nil
}

// Run 은 ctx 가 끝날 때까지 수집과 보고를 반복한다.
func Run(ctx context.Context, cfgPath string, logger *log.Logger) error {
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("설정을 읽을 수 없습니다: %w", err)
	}
	client, err := NewClient(cfg)
	if err != nil {
		return err
	}
	interval := time.Duration(cfg.IntervalSec) * time.Second
	sampleEvery := time.Duration(cfg.SampleSec) * time.Second
	col := NewCollector(cfg.Zone, client.HostPort())
	logger.Printf("infra-agent %s 시작: server=%s 보고 %s / 샘플 %s", model.Version, cfg.Server, interval, sampleEvery)

	st, err := LoadState(filepath.Dir(cfgPath))
	if err != nil {
		return fmt.Errorf("state.json 읽기 실패: %w", err)
	}
	send := func() {
		sctx, cancel := context.WithTimeout(ctx, 40*time.Second)
		defer cancel()
		if st.AgentID == "" || st.AgentToken == "" {
			enrolled, err := Enroll(sctx, cfgPath, &cfg, client)
			if err != nil {
				logger.Printf("%v", err)
				return
			}
			st = enrolled
			logger.Printf("등록 완료: agent_id=%s", st.AgentID)
		}
		rep := col.Report(sctx, interval)
		err := client.Report(sctx, st, rep)
		switch {
		case err == nil:
			col.Reset()
		case errors.Is(err, ErrUnauthorized):
			logger.Printf("보고 거부됨: %v (서버에서 삭제됐다면 install -token 으로 다시 등록하세요)", err)
		default:
			logger.Printf("보고 실패, 다음 주기에 다시 보냅니다: %v", err)
		}
	}

	if err := col.Sample(ctx); err != nil {
		logger.Printf("%v", err)
	}
	send()

	// 여러 서버가 같은 시각에 몰려 보고하지 않도록 첫 주기를 조금 흩는다.
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(time.Duration(rand.Int64N(int64(sampleEvery)))):
	}

	sampleTick := time.NewTicker(sampleEvery)
	defer sampleTick.Stop()
	reportTick := time.NewTicker(interval)
	defer reportTick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sampleTick.C:
			if err := col.Sample(ctx); err != nil && ctx.Err() == nil {
				logger.Printf("%v", err)
			}
		case <-reportTick.C:
			send()
		}
	}
}
