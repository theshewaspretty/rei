package agent

import (
	"net/netip"
	"sort"

	"infrascanner/internal/model"
)

// udpEphemeralStart 이상의 UDP 포트는 클라이언트용 임시 포트로 보고 리스너에서 제외한다.
const udpEphemeralStart = 32768

// sock 은 OS에서 읽은 소켓 1개다.
type sock struct {
	Proto  string // tcp, udp
	Status string // LISTEN, ESTABLISHED ...
	LIP    netip.Addr
	LPort  int
	RIP    netip.Addr
	RPort  int
	PID    int32
}

type flowKey struct {
	Dir      string
	Proto    string
	Peer     string
	Port     int
	Proc     string
	PeerProc string
}

type listenKey struct {
	Proto string
	Port  int
}

// classify 는 소켓 목록을 리스너와 방향이 정해진 흐름으로 나눈다.
//
// 방향은 로컬 포트가 리스닝 중인지로 판단한다. 리스닝 포트로 들어온 연결은 in,
// 그 외는 out 이다. 상대가 자기 자신이면 local 이며, 같은 연결의 서버 쪽 소켓은 버린다.
func classify(socks []sock, own map[netip.Addr]struct{}, procName func(int32) string) ([]model.Listener, map[flowKey]int) {
	listenProc := map[listenKey]string{}
	seen := map[model.Listener]struct{}{}
	var listeners []model.Listener

	for _, s := range socks {
		isListener := false
		switch s.Proto {
		case "tcp":
			isListener = s.Status == "LISTEN"
		case "udp":
			isListener = s.RPort == 0 && s.LPort > 0 && s.LPort < udpEphemeralStart
		}
		if !isListener {
			continue
		}
		name := procName(s.PID)
		lk := listenKey{s.Proto, s.LPort}
		if listenProc[lk] == "" {
			listenProc[lk] = name
		}
		ip := ""
		if s.LIP.IsValid() {
			ip = s.LIP.String()
		}
		l := model.Listener{Proto: s.Proto, IP: ip, Port: s.LPort, PID: s.PID, Process: name}
		if _, dup := seen[l]; dup {
			continue
		}
		seen[l] = struct{}{}
		listeners = append(listeners, l)
	}
	sort.Slice(listeners, func(i, j int) bool {
		a, b := listeners[i], listeners[j]
		if a.Port != b.Port {
			return a.Port < b.Port
		}
		if a.Proto != b.Proto {
			return a.Proto < b.Proto
		}
		return a.IP < b.IP
	})

	flows := map[flowKey]int{}
	for _, s := range socks {
		if s.Proto != "tcp" || s.Status != "ESTABLISHED" || !s.RIP.IsValid() || s.RPort == 0 {
			continue
		}
		_, localIsService := listenProc[listenKey{"tcp", s.LPort}]
		_, peerIsSelf := own[s.RIP]
		peerIsSelf = peerIsSelf || s.RIP.IsLoopback()

		var k flowKey
		switch {
		case peerIsSelf:
			if localIsService {
				continue // 같은 연결의 서버 쪽 소켓. 클라이언트 쪽에서 한 번만 센다.
			}
			peerProc, peerIsService := listenProc[listenKey{"tcp", s.RPort}]
			if !peerIsService {
				continue // 리스닝 포트가 아닌 임시 소켓 쌍(프로세스 내부 IPC)은 서비스 통신이 아니다.
			}
			k = flowKey{
				Dir: model.DirLocal, Proto: "tcp", Peer: s.RIP.String(), Port: s.RPort,
				Proc: procName(s.PID), PeerProc: peerProc,
			}
		case localIsService:
			k = flowKey{Dir: model.DirIn, Proto: "tcp", Peer: s.RIP.String(), Port: s.LPort, Proc: procName(s.PID)}
		default:
			k = flowKey{Dir: model.DirOut, Proto: "tcp", Peer: s.RIP.String(), Port: s.RPort, Proc: procName(s.PID)}
		}
		flows[k]++
	}
	return listeners, flows
}

// normAddr 는 IPv4-mapped IPv6 와 zone 을 정리한다.
func normAddr(s string) (netip.Addr, bool) {
	if s == "" || s == "*" {
		return netip.Addr{}, false
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap().WithZone(""), true
}
