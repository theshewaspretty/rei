// Package demo 는 시연용 가상 서버 13대와 그 사이의 통신을 정의한다.
// cmd/demo-seed 는 이것을 실제 수집 서버에 보내고, cmd/demo-site 는 이것으로 정적 데모 페이지를 만든다.
// 이름과 주소는 모두 지어낸 값이다. 공인 주소는 문서 예시용 대역(RFC 5737: 198.51.100.0/24, 203.0.113.0/24)만 쓴다.
package demo

import (
	"fmt"
	"time"

	"infrascanner/internal/model"
)

// IntervalSec 은 가상 서버의 보고 주기다.
const IntervalSec = 20

const gb = 1 << 30

type svc struct {
	port int
	proc string
	bind string // 비우면 0.0.0.0
}

// Host 는 가상 서버 1대다.
type Host struct {
	Name string
	IP   string
	// StopsReporting: 처음에만 보고하고 멈춘다 ("응답 없음" 시연).
	StopsReporting bool

	cidr, zone         string
	os, platform, ver  string
	cores              int
	memGB, memUsedGB   uint64
	diskGB, diskUsedGB uint64
	cpu                float64
	services           []svc
}

// link 는 from 이 to 의 port 로 접속한다는 뜻이다. to 가 가상 서버면 양쪽 보고에 모두 들어간다.
type link struct {
	from, to string // 호스트 이름 또는 IP
	port     int
	proc     string
	conns    int
}

func linux(name, ip, cidr, zone, platform, ver string, cores int, mem, memUsed, disk, diskUsed uint64, cpu float64, services ...svc) Host {
	services = append(services, svc{22, "sshd", ""}, svc{9100, "node_exporter", ""})
	return Host{Name: name, IP: ip, cidr: cidr, zone: zone, os: "linux", platform: platform, ver: ver, cores: cores,
		memGB: mem, memUsedGB: memUsed, diskGB: disk, diskUsedGB: diskUsed, cpu: cpu, services: services}
}

var hosts = []Host{
	linux("web-01", "10.10.1.11", "10.10.1.0/24", "", "ubuntu", "24.04", 4, 8, 3, 80, 31, 22.5, svc{80, "nginx", ""}, svc{443, "nginx", ""}),
	linux("web-02", "10.10.1.12", "10.10.1.0/24", "", "ubuntu", "24.04", 4, 8, 3, 80, 29, 18.1, svc{80, "nginx", ""}, svc{443, "nginx", ""}),
	linux("api-01", "10.10.2.21", "10.10.2.0/24", "", "rocky", "9.5", 8, 16, 9, 120, 55, 41.0, svc{8080, "java", ""}),
	linux("api-02", "10.10.2.22", "10.10.2.0/24", "", "rocky", "9.5", 8, 16, 10, 120, 57, 38.4, svc{8080, "java", ""}),
	linux("batch-01", "10.10.2.31", "10.10.2.0/24", "", "rocky", "9.5", 4, 16, 6, 200, 96, 12.7),
	linux("mq-01", "10.10.2.41", "10.10.2.0/24", "", "ubuntu", "22.04", 4, 8, 4, 100, 37, 9.3, svc{5672, "beam.smp", ""}),
	linux("cache-01", "10.10.2.42", "10.10.2.0/24", "", "ubuntu", "22.04", 4, 16, 15, 60, 12, 15.2, svc{6379, "redis-server", ""}),
	linux("db-primary", "10.10.3.11", "10.10.3.0/24", "", "rocky", "9.5", 16, 64, 47, 1000, 930, 55.8, svc{5432, "postgres", ""}),
	linux("db-replica", "10.10.3.12", "10.10.3.0/24", "", "rocky", "9.5", 16, 64, 39, 1000, 918, 21.4, svc{5432, "postgres", ""}),
	linux("mongo-01", "10.10.3.21", "10.10.3.0/24", "", "ubuntu", "22.04", 8, 32, 18, 500, 210, 17.0, svc{27017, "mongod", ""}),
	linux("mon-01", "10.10.9.21", "10.10.9.0/24", "운영관리", "ubuntu", "24.04", 4, 16, 7, 300, 140, 26.3, svc{9090, "prometheus", ""}, svc{443, "grafana", ""}),
	linux("bastion-01", "10.10.9.5", "10.10.9.0/24", "운영관리", "ubuntu", "24.04", 2, 4, 1, 40, 9, 3.1),
	{Name: "LEGACY-ERP", IP: "10.10.9.40", cidr: "10.10.9.0/24", zone: "운영관리", os: "windows", platform: "Microsoft Windows Server 2016 Standard", ver: "10.0.14393",
		cores: 4, memGB: 16, memUsedGB: 11, diskGB: 300, diskUsedGB: 214, cpu: 33.0, StopsReporting: true,
		services: []svc{{3389, "svchost.exe", ""}, {445, "System", ""}, {1433, "sqlservr.exe", ""}, {23, "tlntsvr.exe", ""}, {21, "ftpsvc.exe", ""}, {5985, "System", ""}}},
}

var links = []link{
	// 외부 -> 웹
	{"203.0.113.24", "web-01", 443, "", 18}, {"198.51.100.7", "web-01", 443, "", 9}, {"203.0.113.77", "web-02", 443, "", 14},
	// 웹 -> API
	{"web-01", "api-01", 8080, "nginx", 12}, {"web-01", "api-02", 8080, "nginx", 11}, {"web-02", "api-01", 8080, "nginx", 10}, {"web-02", "api-02", 8080, "nginx", 13},
	// API -> DB / 캐시 / 큐 / 외부 API
	{"api-01", "db-primary", 5432, "java", 20}, {"api-02", "db-primary", 5432, "java", 20},
	{"api-01", "cache-01", 6379, "java", 8}, {"api-02", "cache-01", 6379, "java", 8},
	{"api-01", "mq-01", 5672, "java", 4}, {"api-02", "mq-01", 5672, "java", 4},
	{"api-01", "mongo-01", 27017, "java", 6},
	{"api-01", "198.51.100.80", 443, "java", 3}, {"api-02", "198.51.100.80", 443, "java", 2},
	// 배치
	{"batch-01", "mq-01", 5672, "python3", 2}, {"batch-01", "db-replica", 5432, "python3", 5},
	{"batch-01", "10.10.3.50", 1521, "python3", 2}, // 에이전트 없는 Oracle
	{"batch-01", "LEGACY-ERP", 1433, "python3", 1}, {"batch-01", "LEGACY-ERP", 21, "curl", 1},
	// DB 복제
	{"db-replica", "db-primary", 5432, "postgres", 1},
	// 관리 접속
	{"bastion-01", "web-01", 22, "ssh", 1}, {"bastion-01", "api-01", 22, "ssh", 1}, {"bastion-01", "db-primary", 22, "ssh", 2},
	{"10.20.5.31", "bastion-01", 22, "", 2}, // 운영자 PC
	{"10.20.5.31", "mon-01", 443, "", 1},
	{"203.0.113.200", "LEGACY-ERP", 3389, "", 1}, // 공인 IP 에서 RDP
	{"10.20.5.44", "LEGACY-ERP", 23, "", 1},
}

var byName = map[string]*Host{}

func init() {
	for i := range hosts {
		byName[hosts[i].Name] = &hosts[i]
	}
	// 모니터링 서버가 전 서버의 node_exporter 를 수집하고, 모든 리눅스 서버가 사내 DNS / LDAP 을 쓴다.
	for _, h := range hosts {
		if h.os != "linux" {
			continue
		}
		if h.Name != "mon-01" {
			links = append(links, link{"mon-01", h.Name, 9100, "prometheus", 1})
		}
		links = append(links, link{h.Name, "10.10.9.10", 389, "sssd", 1})
	}
}

// Hosts 는 가상 서버 목록이다.
func Hosts() []Host { return append([]Host(nil), hosts...) }

// Report 는 이 서버가 at 시각에 보내는 보고다.
// 첫 보고는 withFlows 를 꺼서 주소만 알린다. 수집 서버가 모든 주소를 알고 난 뒤에 흐름을 보내야 서버끼리의 연결로 묶인다.
func (h Host) Report(at time.Time, withFlows bool) model.Report {
	r := model.Report{
		AgentVersion: "demo", CollectedAt: at.UTC(), IntervalSec: IntervalSec,
		Host: model.Host{
			Hostname: h.Name, Zone: h.zone, OS: h.os, Platform: h.platform, PlatformVersion: h.ver,
			KernelVersion: map[string]string{"linux": "5.14.0-503.el9.x86_64", "windows": "10.0.14393"}[h.os], Arch: "x86_64",
			Virtualization: "vmware", UptimeSec: 86400 * 37, CPUModel: "Intel(R) Xeon(R) Gold 6338", CPUCores: h.cores, CPUPercent: h.cpu,
			MemTotal: h.memGB * gb, MemUsed: h.memUsedGB * gb, PrimaryIP: h.IP, PrimaryCIDR: h.cidr,
			Disks: []model.Disk{{Mount: map[string]string{"linux": "/", "windows": "C:"}[h.os], FSType: map[string]string{"linux": "xfs", "windows": "NTFS"}[h.os], Total: h.diskGB * gb, Used: h.diskUsedGB * gb}},
		},
		Interfaces: []model.Interface{{Name: map[string]string{"linux": "ens192", "windows": "Ethernet0"}[h.os], MAC: "00:50:56:" + macTail(h.IP), Up: true, Addrs: []string{h.IP + h.cidr[len(h.cidr)-3:]}}},
	}
	for i, s := range h.services {
		bind := s.bind
		if bind == "" {
			bind = "0.0.0.0"
		}
		r.Listeners = append(r.Listeners, model.Listener{Proto: "tcp", IP: bind, Port: s.port, PID: int32(900 + i*37), Process: s.proc})
	}
	if !withFlows {
		return r
	}
	addr := func(nameOrIP string) string {
		if p, ok := byName[nameOrIP]; ok {
			return p.IP
		}
		return nameOrIP
	}
	for _, l := range links {
		if l.from == h.Name {
			r.Flows = append(r.Flows, model.Flow{Direction: model.DirOut, Proto: "tcp", PeerIP: addr(l.to), Port: l.port, Process: l.proc, Count: l.conns})
		}
		if l.to == h.Name {
			proc := ""
			for _, s := range h.services {
				if s.port == l.port {
					proc = s.proc
				}
			}
			r.Flows = append(r.Flows, model.Flow{Direction: model.DirIn, Proto: "tcp", PeerIP: addr(l.from), Port: l.port, Process: proc, Count: l.conns})
		}
	}
	return r
}

func macTail(ip string) string {
	var a, b, c, d int
	fmt.Sscanf(ip, "%d.%d.%d.%d", &a, &b, &c, &d)
	return fmt.Sprintf("%02x:%02x:%02x", b, c, d)
}
