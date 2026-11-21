// Package model 은 에이전트와 서버가 주고받는 보고 형식을 정의한다.
package model

import "time"

// Version 은 빌드 시 -ldflags 로 덮어쓴다.
var Version = "dev"

// 흐름 방향.
const (
	DirOut   = "out"   // 이 서버가 다른 곳으로 접속
	DirIn    = "in"    // 다른 곳에서 이 서버의 리스닝 포트로 접속
	DirLocal = "local" // 같은 서버 안의 프로세스끼리 접속
)

// Report 는 에이전트가 주기적으로 보내는 서버 1대의 스냅샷이다.
type Report struct {
	AgentVersion   string      `json:"agent_version"`
	CollectedAt    time.Time   `json:"collected_at"`
	IntervalSec    int         `json:"interval_sec"`
	Host           Host        `json:"host"`
	Interfaces     []Interface `json:"interfaces"`
	Listeners      []Listener  `json:"listeners"`
	Flows          []Flow      `json:"flows,omitempty"`
	FlowsTruncated bool        `json:"flows_truncated,omitempty"`
	Warnings       []string    `json:"warnings,omitempty"`
}

type Host struct {
	Hostname        string  `json:"hostname"`
	Zone            string  `json:"zone,omitempty"` // 설치 시 지정한 영역 이름(선택)
	OS              string  `json:"os"`             // windows, linux
	Platform        string  `json:"platform"`
	PlatformVersion string  `json:"platform_version"`
	KernelVersion   string  `json:"kernel_version"`
	Arch            string  `json:"arch"`
	Virtualization  string  `json:"virtualization,omitempty"`
	UptimeSec       uint64  `json:"uptime_sec"`
	CPUModel        string  `json:"cpu_model,omitempty"`
	CPUCores        int     `json:"cpu_cores"`
	CPUPercent      float64 `json:"cpu_percent"`
	MemTotal        uint64  `json:"mem_total"`
	MemUsed         uint64  `json:"mem_used"`
	Disks           []Disk  `json:"disks,omitempty"`
	PrimaryIP       string  `json:"primary_ip,omitempty"`   // 수집 서버로 나갈 때 쓰는 주소
	PrimaryCIDR     string  `json:"primary_cidr,omitempty"` // 그 주소가 속한 서브넷
}

type Disk struct {
	Mount  string `json:"mount"`
	FSType string `json:"fstype,omitempty"`
	Total  uint64 `json:"total"`
	Used   uint64 `json:"used"`
}

type Interface struct {
	Name  string   `json:"name"`
	MAC   string   `json:"mac,omitempty"`
	Up    bool     `json:"up"`
	Addrs []string `json:"addrs"` // CIDR 표기
}

type Listener struct {
	Proto   string `json:"proto"` // tcp, udp
	IP      string `json:"ip"`
	Port    int    `json:"port"`
	PID     int32  `json:"pid,omitempty"`
	Process string `json:"process,omitempty"`
}

// Flow 는 보고 주기 동안 관측된 TCP 연결을 (방향, 상대, 서비스 포트, 프로세스) 단위로 묶은 것이다.
type Flow struct {
	Direction   string `json:"direction"`
	Proto       string `json:"proto"`
	PeerIP      string `json:"peer_ip"`
	Port        int    `json:"port"`                   // 서비스 포트 (out: 상대 포트, in: 내 리스닝 포트)
	Process     string `json:"process,omitempty"`      // 이 서버에서 소켓을 가진 프로세스
	PeerProcess string `json:"peer_process,omitempty"` // local 흐름에서 리스닝 쪽 프로세스
	Count       int    `json:"count"`                  // 한 번의 샘플에서 동시에 열려 있던 최대 연결 수
}

type EnrollRequest struct {
	Token     string `json:"token"`
	MachineID string `json:"machine_id"`
	Hostname  string `json:"hostname"`
}

type EnrollResponse struct {
	AgentID    string `json:"agent_id"`
	AgentToken string `json:"agent_token"`
}
