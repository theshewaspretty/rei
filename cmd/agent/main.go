// infra-agent: 서버에 설치해 리스닝 포트와 통신 흐름을 수집 서버로 보내는 에이전트.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/kardianos/service"

	"infrascanner/internal/agent"
	"infrascanner/internal/model"
)

const usage = `infra-agent %s

사용법:
  infra-agent install -server https://HOST:8443 -token ENROLL_TOKEN [옵션]   서비스로 설치하고 시작
  infra-agent uninstall [-purge]                                            서비스 제거
  infra-agent run [-config PATH]                                            포그라운드 실행
  infra-agent once [-zone NAME]                                             한 번 수집해 JSON 출력(전송 안 함)
  infra-agent version

install 옵션:
  -zone NAME             구성도에서 묶을 영역 이름 (없으면 서브넷으로 묶음)
  -interval N            보고 주기(초), 기본 60
  -ca FILE               사내 CA 인증서(PEM)
  -server-cert-sha256 FP 자체 서명 서버 인증서의 SHA-256 지문
  -allow-http            평문 HTTP 서버 허용 (시험용)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, usage, model.Version)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "install":
		err = cmdInstall(os.Args[2:])
	case "uninstall":
		err = cmdUninstall(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "once":
		err = cmdOnce(os.Args[2:])
	case "version":
		fmt.Println(model.Version)
	default:
		fmt.Fprintf(os.Stderr, usage, model.Version)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "오류:", err)
		os.Exit(1)
	}
}

func defaultConfigPath() string { return filepath.Join(agent.DefaultDir(), "agent.json") }

func installedBinary() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("ProgramFiles")
		if base == "" {
			base = `C:\Program Files`
		}
		return filepath.Join(base, "InfraScanner", "infra-agent.exe")
	}
	return "/usr/local/bin/infra-agent"
}

// ---- 서비스 -----------------------------------------------------------------

type program struct {
	cfgPath string
	logger  *log.Logger
	cancel  context.CancelFunc
	done    chan struct{}
}

func (p *program) Start(service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})
	go func() {
		defer close(p.done)
		if err := agent.Run(ctx, p.cfgPath, p.logger); err != nil {
			p.logger.Printf("중단: %v", err)
			os.Exit(1) // 서비스 관리자가 다시 시작하도록 실패로 끝낸다
		}
	}()
	return nil
}

func (p *program) Stop(service.Service) error {
	if p.cancel != nil {
		p.cancel()
		select {
		case <-p.done:
		case <-time.After(10 * time.Second):
		}
	}
	return nil
}

func newService(p *program, cfgPath string) (service.Service, error) {
	return service.New(p, &service.Config{
		Name:        "InfraScannerAgent",
		DisplayName: "Infra Scanner Agent",
		Description: "리스닝 포트와 서버 간 통신 흐름을 수집해 Infra Scanner 서버로 보냅니다.",
		Executable:  installedBinary(),
		Arguments:   []string{"run", "-config", cfgPath},
		Option: service.KeyValue{
			"Restart":   "always",  // systemd
			"OnFailure": "restart", // Windows 서비스
		},
	})
}

// ---- 명령 -------------------------------------------------------------------

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	cfgPath := fs.String("config", defaultConfigPath(), "설정 파일 경로")
	fs.Parse(args)

	if service.Interactive() {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return agent.Run(ctx, *cfgPath, log.New(os.Stderr, "", log.LstdFlags))
	}

	logger := log.New(serviceLogWriter(filepath.Dir(*cfgPath)), "", log.LstdFlags)
	p := &program{cfgPath: *cfgPath, logger: logger}
	s, err := newService(p, *cfgPath)
	if err != nil {
		return err
	}
	return s.Run()
}

// serviceLogWriter: Windows 서비스는 stderr 가 없으므로 파일에 남긴다. Linux 는 journald 가 stderr 를 받는다.
func serviceLogWriter(dir string) io.Writer {
	if runtime.GOOS != "windows" {
		return os.Stderr
	}
	path := filepath.Join(dir, "agent.log")
	if fi, err := os.Stat(path); err == nil && fi.Size() > 5<<20 {
		os.Remove(path + ".1")
		os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return io.Discard
	}
	return f
}

func cmdOnce(args []string) error {
	fs := flag.NewFlagSet("once", flag.ExitOnError)
	zone := fs.String("zone", "", "영역 이름")
	fs.Parse(args)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	col := agent.NewCollector(*zone, "")
	if err := col.Sample(ctx); err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(col.Report(ctx, time.Minute))
}

func cmdInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	server := fs.String("server", "", "수집 서버 주소 (https://HOST:PORT)")
	token := fs.String("token", "", "등록 토큰")
	zone := fs.String("zone", "", "영역 이름")
	interval := fs.Int("interval", 60, "보고 주기(초)")
	caFile := fs.String("ca", "", "사내 CA 인증서(PEM)")
	pin := fs.String("server-cert-sha256", "", "서버 인증서 SHA-256 지문")
	allowHTTP := fs.Bool("allow-http", false, "평문 HTTP 허용")
	fs.Parse(args)

	if *server == "" {
		return errors.New("-server 가 필요합니다")
	}
	if !isElevated() {
		if runtime.GOOS == "windows" {
			return errors.New("관리자 권한이 필요합니다 (관리자 권한 PowerShell 에서 실행하세요)")
		}
		return errors.New("root 권한이 필요합니다 (sudo 로 실행하세요)")
	}

	dir := agent.DefaultDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("설정 폴더를 만들 수 없습니다 (관리자 권한으로 실행했는지 확인하세요): %w", err)
	}
	if err := restrictDir(dir); err != nil {
		return fmt.Errorf("설정 폴더 권한 설정 실패: %w", err)
	}
	cfgPath := filepath.Join(dir, "agent.json")

	cfg := agent.Config{
		Server: *server, EnrollToken: *token, Zone: *zone, IntervalSec: *interval,
		ServerCertSHA256: *pin, AllowHTTP: *allowHTTP,
	}
	if *caFile != "" {
		// 서비스가 계속 읽을 수 있도록 설정 폴더로 복사해 둔다.
		dst := filepath.Join(dir, "ca.pem")
		if err := copyFile(*caFile, dst, 0o600); err != nil {
			return fmt.Errorf("CA 인증서 복사 실패: %w", err)
		}
		cfg.CAFile = dst
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	st, err := agent.LoadState(dir)
	if err != nil {
		return err
	}
	if *token != "" && st.AgentID != "" {
		// 새 토큰을 줬다면 다시 등록한다. 같은 장비는 서버에서 같은 항목으로 이어진다.
		if err := agent.SaveState(dir, agent.State{}); err != nil {
			return err
		}
	} else if *token == "" && st.AgentID == "" {
		return errors.New("-token 이 필요합니다")
	}
	if err := agent.SaveConfig(cfgPath, cfg); err != nil {
		return err
	}

	// 서비스를 등록하기 전에 여기서 등록해, 주소·토큰·인증서 문제를 설치 시점에 바로 알린다.
	client, err := agent.NewClient(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	st, err = agent.Enroll(ctx, cfgPath, &cfg, client)
	if err != nil {
		return err
	}
	fmt.Println("등록 완료: agent_id =", st.AgentID)

	s, err := newService(&program{}, cfgPath)
	if err != nil {
		return err
	}
	_ = s.Stop()      // 재설치·업그레이드: 실행 중이면 멈춘다
	_ = s.Uninstall() // 이전 등록이 있으면 지우고 새로 등록한다

	self, err := os.Executable()
	if err != nil {
		return err
	}
	dst := installedBinary()
	if !sameFile(self, dst) {
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := copyFile(self, dst, 0o755); err != nil {
			return fmt.Errorf("실행 파일 복사 실패: %w", err)
		}
	}
	if err := s.Install(); err != nil {
		return fmt.Errorf("서비스 등록 실패: %w", err)
	}
	if err := s.Start(); err != nil {
		return fmt.Errorf("서비스 시작 실패: %w", err)
	}
	fmt.Println("서비스 시작: InfraScannerAgent")
	fmt.Println("실행 파일:", dst)
	fmt.Println("설정 파일:", cfgPath)
	return nil
}

func cmdUninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	purge := fs.Bool("purge", false, "설정과 등록 정보도 삭제")
	fs.Parse(args)

	s, err := newService(&program{}, defaultConfigPath())
	if err != nil {
		return err
	}
	_ = s.Stop()
	if err := s.Uninstall(); err != nil {
		return fmt.Errorf("서비스 제거 실패: %w", err)
	}
	fmt.Println("서비스 제거: InfraScannerAgent")
	if *purge {
		if err := os.RemoveAll(agent.DefaultDir()); err != nil {
			return err
		}
		fmt.Println("삭제:", agent.DefaultDir())
	}
	fmt.Println("실행 파일은 남아 있습니다:", installedBinary())
	return nil
}

// ---- 파일 -------------------------------------------------------------------

// restrictDir 는 에이전트 토큰이 든 폴더를 관리자만 읽게 한다.
// Windows 의 ProgramData 는 기본으로 일반 사용자도 읽을 수 있어 상속을 끊는다.
func restrictDir(dir string) error {
	if runtime.GOOS != "windows" {
		return os.Chmod(dir, 0o700)
	}
	// 그룹 이름은 언어마다 달라 SID 로 지정한다: SYSTEM, Administrators.
	out, err := exec.Command("icacls", dir, "/inheritance:r",
		"/grant:r", "*S-1-5-18:(OI)(CI)F", "*S-1-5-32-544:(OI)(CI)F").CombinedOutput()
	if err != nil {
		return fmt.Errorf("icacls: %v: %s", err, out)
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func sameFile(a, b string) bool {
	fa, err1 := os.Stat(a)
	fb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(fa, fb)
}
