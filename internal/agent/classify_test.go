package agent

import (
	"net/netip"
	"testing"

	"infrascanner/internal/model"
)

func addr(s string) netip.Addr { return netip.MustParseAddr(s) }

func TestClassify(t *testing.T) {
	own := map[netip.Addr]struct{}{addr("10.0.0.5"): {}}
	names := map[int32]string{1: "nginx", 2: "app", 3: "curl"}
	procName := func(pid int32) string { return names[pid] }

	socks := []sock{
		// 리스너
		{Proto: "tcp", Status: "LISTEN", LIP: addr("0.0.0.0"), LPort: 443, PID: 1},
		{Proto: "tcp", Status: "LISTEN", LIP: addr("127.0.0.1"), LPort: 8000, PID: 2},
		{Proto: "udp", LIP: addr("0.0.0.0"), LPort: 53, PID: 2},
		{Proto: "udp", LIP: addr("0.0.0.0"), LPort: 51000, PID: 3}, // 임시 UDP 포트: 리스너 아님
		// 외부에서 443 으로 들어온 연결 2개
		{Proto: "tcp", Status: "ESTABLISHED", LIP: addr("10.0.0.5"), LPort: 443, RIP: addr("10.0.0.9"), RPort: 50001, PID: 1},
		{Proto: "tcp", Status: "ESTABLISHED", LIP: addr("10.0.0.5"), LPort: 443, RIP: addr("10.0.0.9"), RPort: 50002, PID: 1},
		// DB 로 나가는 연결
		{Proto: "tcp", Status: "ESTABLISHED", LIP: addr("10.0.0.5"), LPort: 40000, RIP: addr("10.0.0.7"), RPort: 5432, PID: 2},
		// nginx -> 로컬 app: 클라이언트 쪽과 서버 쪽 소켓
		{Proto: "tcp", Status: "ESTABLISHED", LIP: addr("127.0.0.1"), LPort: 41000, RIP: addr("127.0.0.1"), RPort: 8000, PID: 1},
		{Proto: "tcp", Status: "ESTABLISHED", LIP: addr("127.0.0.1"), LPort: 8000, RIP: addr("127.0.0.1"), RPort: 41000, PID: 2},
		// 리스닝 포트가 없는 임시 소켓 쌍
		{Proto: "tcp", Status: "ESTABLISHED", LIP: addr("127.0.0.1"), LPort: 42000, RIP: addr("127.0.0.1"), RPort: 42001, PID: 3},
		{Proto: "tcp", Status: "ESTABLISHED", LIP: addr("127.0.0.1"), LPort: 42001, RIP: addr("127.0.0.1"), RPort: 42000, PID: 3},
		// 연결 중인 소켓은 세지 않는다
		{Proto: "tcp", Status: "SYN_SENT", LIP: addr("10.0.0.5"), LPort: 43000, RIP: addr("10.0.0.8"), RPort: 80, PID: 3},
	}

	listeners, flows := classify(socks, own, procName)

	if len(listeners) != 3 {
		t.Fatalf("리스너 3개를 기대했는데 %d개: %+v", len(listeners), listeners)
	}
	if listeners[0].Port != 53 || listeners[1].Port != 443 || listeners[2].Port != 8000 {
		t.Errorf("리스너가 포트 순으로 정렬되지 않음: %+v", listeners)
	}

	want := map[flowKey]int{
		{Dir: model.DirIn, Proto: "tcp", Peer: "10.0.0.9", Port: 443, Proc: "nginx"}:                       2,
		{Dir: model.DirOut, Proto: "tcp", Peer: "10.0.0.7", Port: 5432, Proc: "app"}:                       1,
		{Dir: model.DirLocal, Proto: "tcp", Peer: "127.0.0.1", Port: 8000, Proc: "nginx", PeerProc: "app"}: 1,
	}
	if len(flows) != len(want) {
		t.Fatalf("흐름 %d개를 기대했는데 %d개: %+v", len(want), len(flows), flows)
	}
	for k, n := range want {
		if flows[k] != n {
			t.Errorf("%+v: %d 를 기대했는데 %d", k, n, flows[k])
		}
	}
}

func TestClassifyOwnAddressIsLocal(t *testing.T) {
	own := map[netip.Addr]struct{}{addr("10.0.0.5"): {}}
	socks := []sock{
		{Proto: "tcp", Status: "LISTEN", LIP: addr("0.0.0.0"), LPort: 6379, PID: 1},
		{Proto: "tcp", Status: "ESTABLISHED", LIP: addr("10.0.0.5"), LPort: 45000, RIP: addr("10.0.0.5"), RPort: 6379, PID: 2},
	}
	_, flows := classify(socks, own, func(int32) string { return "" })
	if len(flows) != 1 {
		t.Fatalf("흐름 1개를 기대했는데 %d개", len(flows))
	}
	for k := range flows {
		if k.Dir != model.DirLocal {
			t.Errorf("자기 주소로 가는 연결은 local 이어야 하는데 %q", k.Dir)
		}
	}
}

func TestNormAddr(t *testing.T) {
	cases := map[string]string{
		"::ffff:10.1.2.3": "10.1.2.3",
		"fe80::1%eth0":    "fe80::1",
		"192.168.0.1":     "192.168.0.1",
	}
	for in, want := range cases {
		got, ok := normAddr(in)
		if !ok || got.String() != want {
			t.Errorf("normAddr(%q) = %v, %v; want %s", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "*", "not-an-ip"} {
		if _, ok := normAddr(bad); ok {
			t.Errorf("normAddr(%q) 는 실패해야 한다", bad)
		}
	}
}
