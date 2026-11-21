package agent

import (
	"context"
	"fmt"
	stdnet "net"
	"net/netip"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	psnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"

	"infrascanner/internal/model"
)

const (
	maxFlowsPerReport = 1000 // 한 번에 보내는 흐름 수 상한
	maxPendingFlows   = 5000 // 전송 실패가 이어질 때 메모리에 쌓아 두는 상한
	sockStream        = 1
	sockDgram         = 2
)

// Collector 는 주기적으로 소켓을 샘플링해 흐름을 모으고, 보고 시점에 서버 정보를 붙인다.
// 하나의 고루틴에서만 호출한다.
type Collector struct {
	zone       string
	serverAddr string // host:port. 기본 주소 판별용
	flows      map[flowKey]int
	listeners  []model.Listener
	truncated  bool
	sampleErr  string
}

func NewCollector(zone, serverAddr string) *Collector {
	return &Collector{zone: zone, serverAddr: serverAddr, flows: map[flowKey]int{}}
}

// Sample 은 현재 열려 있는 소켓을 한 번 읽어 흐름에 합친다.
// 폴링 방식이므로 샘플 사이에 열렸다 닫힌 짧은 연결은 잡히지 않는다.
func (c *Collector) Sample(ctx context.Context) error {
	conns, err := psnet.ConnectionsWithContext(ctx, "inet")
	if err != nil {
		c.sampleErr = "connections: " + err.Error()
		return fmt.Errorf("list connections: %w", err)
	}
	c.sampleErr = ""

	self := int32(os.Getpid())
	socks := make([]sock, 0, len(conns))
	for _, cn := range conns {
		if cn.Pid == self {
			continue // 에이전트가 수집 서버로 보고하는 연결은 점검 대상의 통신이 아니다
		}
		var proto string
		switch cn.Type {
		case sockStream:
			proto = "tcp"
		case sockDgram:
			proto = "udp"
		default:
			continue
		}
		s := sock{Proto: proto, Status: cn.Status, LPort: int(cn.Laddr.Port), RPort: int(cn.Raddr.Port), PID: cn.Pid}
		s.LIP, _ = normAddr(cn.Laddr.IP)
		s.RIP, _ = normAddr(cn.Raddr.IP)
		socks = append(socks, s)
	}

	names := map[int32]string{}
	procName := func(pid int32) string {
		if pid <= 0 {
			return ""
		}
		if n, ok := names[pid]; ok {
			return n
		}
		n := ""
		if p, err := process.NewProcessWithContext(ctx, pid); err == nil {
			n, _ = p.NameWithContext(ctx)
		}
		names[pid] = n
		return n
	}

	listeners, flows := classify(socks, ownAddrs(), procName)
	c.listeners = listeners
	for k, n := range flows {
		if cur, ok := c.flows[k]; ok {
			if n > cur {
				c.flows[k] = n
			}
			continue
		}
		if len(c.flows) >= maxPendingFlows {
			c.truncated = true
			continue
		}
		c.flows[k] = n
	}
	return nil
}

// Report 는 지금까지 모은 흐름과 현재 서버 상태로 보고서를 만든다.
// 전송에 성공하면 Reset 을 호출한다.
func (c *Collector) Report(ctx context.Context, interval time.Duration) model.Report {
	r := model.Report{
		AgentVersion: model.Version,
		CollectedAt:  time.Now().UTC(),
		IntervalSec:  int(interval / time.Second),
		Listeners:    c.listeners,
	}
	// 경고는 "<코드>: <원인>" 형식이다. 관제 화면이 코드를 선택된 언어로 바꿔 보여 준다
	// (web/static/i18n.js 의 warn.<코드>).
	warn := func(code string, err error) {
		r.Warnings = append(r.Warnings, fmt.Sprintf("%s: %v", code, err))
	}
	if c.sampleErr != "" {
		r.Warnings = append(r.Warnings, c.sampleErr)
	}

	h := model.Host{Zone: c.zone, OS: runtime.GOOS, Arch: runtime.GOARCH}
	if info, err := host.InfoWithContext(ctx); err == nil {
		h.Hostname = info.Hostname
		h.Platform = info.Platform
		h.PlatformVersion = info.PlatformVersion
		h.KernelVersion = info.KernelVersion
		h.UptimeSec = info.Uptime
		if info.VirtualizationRole == "guest" {
			h.Virtualization = info.VirtualizationSystem
		}
	} else {
		warn("host_info", err)
	}
	if h.Hostname == "" {
		h.Hostname, _ = os.Hostname()
	}

	if n, err := cpu.CountsWithContext(ctx, true); err == nil {
		h.CPUCores = n
	}
	if infos, err := cpu.InfoWithContext(ctx); err == nil && len(infos) > 0 {
		h.CPUModel = strings.TrimSpace(infos[0].ModelName)
	}
	// interval 0: 직전 호출 이후의 평균. 첫 호출은 0 이 나온다.
	if pct, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(pct) > 0 {
		h.CPUPercent = round1(pct[0])
	}
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		h.MemTotal, h.MemUsed = vm.Total, vm.Used
	} else {
		warn("memory", err)
	}
	h.Disks = collectDisks(ctx)

	r.Interfaces = collectInterfaces()
	if ip, ok := primaryAddr(c.serverAddr); ok {
		h.PrimaryIP = ip.String()
		h.PrimaryCIDR = cidrOf(ip)
	}
	r.Host = h

	flows := make([]model.Flow, 0, len(c.flows))
	for k, n := range c.flows {
		flows = append(flows, model.Flow{
			Direction: k.Dir, Proto: k.Proto, PeerIP: k.Peer, Port: k.Port,
			Process: k.Proc, PeerProcess: k.PeerProc, Count: n,
		})
	}
	sort.Slice(flows, func(i, j int) bool {
		a, b := flows[i], flows[j]
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		if a.Direction != b.Direction {
			return a.Direction < b.Direction
		}
		if a.PeerIP != b.PeerIP {
			return a.PeerIP < b.PeerIP
		}
		if a.Port != b.Port {
			return a.Port < b.Port
		}
		return a.Process < b.Process
	})
	r.FlowsTruncated = c.truncated
	if len(flows) > maxFlowsPerReport {
		flows = flows[:maxFlowsPerReport]
		r.FlowsTruncated = true
	}
	r.Flows = flows
	return r
}

// Reset 은 전송에 성공한 흐름을 비운다.
func (c *Collector) Reset() {
	c.flows = map[flowKey]int{}
	c.truncated = false
}

func collectDisks(ctx context.Context) []model.Disk {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return nil
	}
	skipFS := map[string]bool{"squashfs": true, "overlay": true, "tmpfs": true, "devtmpfs": true, "iso9660": true, "cdfs": true}
	seen := map[string]bool{}
	var out []model.Disk
	for _, p := range parts {
		fs := strings.ToLower(p.Fstype)
		if skipFS[fs] || seen[p.Device] {
			continue
		}
		u, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}
		seen[p.Device] = true
		out = append(out, model.Disk{Mount: p.Mountpoint, FSType: p.Fstype, Total: u.Total, Used: u.Used})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mount < out[j].Mount })
	return out
}

func collectInterfaces() []model.Interface {
	ifs, err := stdnet.Interfaces()
	if err != nil {
		return nil
	}
	var out []model.Interface
	for _, ifc := range ifs {
		if ifc.Flags&stdnet.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		var cidrs []string
		for _, a := range addrs {
			ipn, ok := a.(*stdnet.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(ipn.IP)
			if !ok {
				continue
			}
			ip = ip.Unmap()
			if ip.IsLinkLocalUnicast() {
				continue
			}
			ones, _ := ipn.Mask.Size()
			cidrs = append(cidrs, netip.PrefixFrom(ip, ones).String())
		}
		if len(cidrs) == 0 {
			continue
		}
		out = append(out, model.Interface{
			Name: ifc.Name, MAC: ifc.HardwareAddr.String(), Up: ifc.Flags&stdnet.FlagUp != 0, Addrs: cidrs,
		})
	}
	return out
}

// ownAddrs 는 이 서버가 가진 모든 주소다.
func ownAddrs() map[netip.Addr]struct{} {
	own := map[netip.Addr]struct{}{}
	addrs, err := stdnet.InterfaceAddrs()
	if err != nil {
		return own
	}
	for _, a := range addrs {
		if ipn, ok := a.(*stdnet.IPNet); ok {
			if ip, ok := netip.AddrFromSlice(ipn.IP); ok {
				own[ip.Unmap()] = struct{}{}
			}
		}
	}
	return own
}

// primaryAddr 는 수집 서버로 나갈 때 OS가 고르는 로컬 주소다. UDP 소켓만 열고 패킷은 보내지 않는다.
func primaryAddr(serverAddr string) (netip.Addr, bool) {
	// 수집 서버가 같은 장비이거나 이름을 못 찾으면 기본 경로로 나갈 때의 주소를 쓴다.
	// 192.0.2.1 은 문서용으로 예약된 주소(TEST-NET-1)이고, UDP 는 연결만으로는 패킷이 나가지 않는다.
	for _, target := range []string{serverAddr, "192.0.2.1:9"} {
		if target == "" {
			continue
		}
		conn, err := stdnet.DialTimeout("udp", target, 2*time.Second)
		if err != nil {
			continue
		}
		local, ok := conn.LocalAddr().(*stdnet.UDPAddr)
		conn.Close()
		if !ok {
			continue
		}
		if ip, ok := netip.AddrFromSlice(local.IP); ok && !ip.Unmap().IsLoopback() && !ip.IsUnspecified() {
			return ip.Unmap(), true
		}
	}
	// 기본 경로도 없으면 켜져 있는 인터페이스의 IPv4 중 가장 작은 주소를 쓴다.
	var fallback netip.Addr
	ifs, _ := stdnet.Interfaces()
	for _, ifc := range ifs {
		if ifc.Flags&stdnet.FlagLoopback != 0 || ifc.Flags&stdnet.FlagUp == 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*stdnet.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(ipn.IP)
			if !ok {
				continue
			}
			if ip = ip.Unmap(); ip.Is4() && !ip.IsLinkLocalUnicast() && (!fallback.IsValid() || ip.Less(fallback)) {
				fallback = ip
			}
		}
	}
	return fallback, fallback.IsValid()
}

func cidrOf(ip netip.Addr) string {
	addrs, err := stdnet.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		ipn, ok := a.(*stdnet.IPNet)
		if !ok {
			continue
		}
		cur, ok := netip.AddrFromSlice(ipn.IP)
		if !ok || cur.Unmap() != ip {
			continue
		}
		ones, _ := ipn.Mask.Size()
		return netip.PrefixFrom(ip, ones).Masked().String()
	}
	return ""
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}
