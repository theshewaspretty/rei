package server

import (
	"net/netip"
	"testing"
	"time"

	"infrascanner/internal/model"
)

func testAgent(id, name, ip string, lastSeen time.Time, ports ...int) *AgentRecord {
	rep := &model.Report{
		IntervalSec: 60,
		Host:        model.Host{Hostname: name, OS: "linux", Platform: "ubuntu", PlatformVersion: "24.04", PrimaryIP: ip, PrimaryCIDR: "10.0.0.0/24", MemTotal: 100, MemUsed: 10},
		Interfaces:  []model.Interface{{Name: "eth0", Up: true, Addrs: []string{ip + "/24"}}},
	}
	for _, p := range ports {
		rep.Listeners = append(rep.Listeners, model.Listener{Proto: "tcp", IP: "0.0.0.0", Port: p, Process: "svc"})
	}
	return &AgentRecord{ID: id, Hostname: name, LastSeen: lastSeen, Report: rep}
}

// sampleTopology 는 서버 3대(웹, DB, 응답 없는 서버)와 그 사이의 흐름으로 구성도를 만든다.
func sampleTopology(t *testing.T) *Topology {
	t.Helper()
	now := time.Now()
	agents := []*AgentRecord{
		testAgent("w", "web", "10.0.0.1", now, 443),
		testAgent("d", "db", "10.0.0.2", now, 5432, 23),
		testAgent("o", "old", "10.0.0.3", now.Add(-time.Hour), 3389),
	}
	index := map[netip.Addr]string{}
	for _, a := range agents {
		index[netip.MustParseAddr(a.Report.Host.PrimaryIP)] = a.ID
	}
	row := func(agent, dir, peer string, port int, proc string) FlowRow {
		return FlowRow{AgentID: agent, Direction: dir, Proto: "tcp", PeerIP: peer, Port: port, Process: proc, LastSeen: now, MaxConns: 2}
	}
	rows := []FlowRow{
		row("w", model.DirOut, "10.0.0.2", 5432, "app"),      // web -> db: 양쪽에서 본 같은 연결
		row("d", model.DirIn, "10.0.0.1", 5432, "postgres"),  //
		row("w", model.DirOut, "10.0.0.77", 1521, "app"),     // 에이전트 없는 내부 서버
		row("w", model.DirOut, "8.8.8.8", 443, "app"),        // 인터넷
		row("w", model.DirIn, peerInternet, 443, "nginx"),    // 인터넷에서 들어옴
		row("o", model.DirIn, peerInternet, 3389, "svchost"), // 인터넷에서 RDP
		row("d", model.DirIn, peerClients, 5432, "postgres"), // 내부 클라이언트
		row("gone", model.DirOut, "10.0.0.2", 5432, "app"),   // 삭제된 에이전트의 흐름은 무시
	}
	return buildTopology(agents, index, rows, map[string]Point{"a:w": {X: 5, Y: 6}, "a:gone": {X: 1, Y: 1}}, 24*time.Hour, now)
}

func TestBuildTopology(t *testing.T) {
	topo := sampleTopology(t)

	nodes := map[string]TopoNode{}
	for _, n := range topo.Nodes {
		nodes[n.ID] = n
	}
	for _, id := range []string{"a:w", "a:d", "a:o", "u:10.0.0.77", nodeInternet, nodeClients} {
		if _, ok := nodes[id]; !ok {
			t.Errorf("노드 %s 가 없습니다", id)
		}
	}
	if len(topo.Nodes) != 6 {
		t.Errorf("노드 수 = %d, 6 이어야 합니다", len(topo.Nodes))
	}
	if got := nodes["a:d"].Subtitle; got != roles[catDB].Label {
		t.Errorf("db 역할 = %q", got)
	}
	if got := nodes["a:o"].Status; got != "offline" {
		t.Errorf("old 상태 = %q, offline 이어야 합니다", got)
	}
	if got := nodes["u:10.0.0.77"].Zone; got != "z:n:10.0.0.0/24" {
		t.Errorf("미설치 서버 영역 = %q", got)
	}

	edges := map[string]TopoEdge{}
	for _, e := range topo.Edges {
		edges[e.ID] = e
	}
	wantEdges := []string{"e:a:w>a:d", "e:a:w>u:10.0.0.77", "e:a:w>x:internet", "e:x:internet>a:w", "e:x:internet>a:o", "e:x:clients>a:d"}
	if len(edges) != len(wantEdges) {
		t.Errorf("연결 수 = %d, %d 이어야 합니다: %v", len(edges), len(wantEdges), topo.Edges)
	}
	for _, id := range wantEdges {
		if _, ok := edges[id]; !ok {
			t.Errorf("연결 %s 가 없습니다", id)
		}
	}
	wd := edges["e:a:w>a:d"]
	if len(wd.Services) != 1 || wd.Services[0].Port != 5432 {
		t.Fatalf("web->db 서비스 = %+v", wd.Services)
	}
	if s := wd.Services[0]; len(s.ClientProcs) != 1 || s.ClientProcs[0] != "app" || len(s.ServerProcs) != 1 || s.ServerProcs[0] != "postgres" {
		t.Errorf("web->db 프로세스 = 접속 %v, 수신 %v", s.ClientProcs, s.ServerProcs)
	}
	if wd.Color != "purple" {
		t.Errorf("web->db 색 = %q", wd.Color)
	}

	if len(topo.Layout) != 1 || topo.Layout["a:w"].X != 5 {
		t.Errorf("배치 = %v", topo.Layout)
	}
	if topo.Stats.Hosts != 3 || topo.Stats.Online != 2 || topo.Stats.Offline != 1 || topo.Stats.Unmanaged != 1 {
		t.Errorf("통계 = %+v", topo.Stats)
	}

	codes := map[string]string{}
	for _, f := range topo.Findings {
		codes[f.Code+"|"+f.NodeID] = f.Severity
	}
	for key, sev := range map[string]string{
		"agent_offline|a:o":          "high",
		"telnet_listen|a:d":          "high",
		"internet_exposed|a:o":       "high",
		"unmanaged_host|u:10.0.0.77": "low",
	} {
		if codes[key] != sev {
			t.Errorf("점검 항목 %s = %q, %q 이어야 합니다", key, codes[key], sev)
		}
	}
	if _, ok := codes["internet_exposed|a:w"]; ok {
		t.Error("인터넷에서 443 으로 들어오는 것은 노출 항목이 아닙니다")
	}

	// 점검 항목은 문장이 아니라 코드와 인자로 나간다 (문장은 화면이 선택된 언어로 만든다).
	for _, f := range topo.Findings {
		switch f.Code + "|" + f.NodeID {
		case "agent_offline|a:o":
			if f.Args["host"] != "old" || f.Args["age_sec"].(int) < 3500 {
				t.Errorf("agent_offline 인자 = %v", f.Args)
			}
		case "internet_exposed|a:o":
			if f.Args["target"] != "a:o" || len(f.Args["services"].([]string)) != 1 {
				t.Errorf("internet_exposed 인자 = %v", f.Args)
			}
		}
	}
	for _, z := range topo.Zones {
		if z.ID == "z:n:10.0.0.0/24" && z.Count != 4 {
			t.Errorf("영역 노드 수 = %d, 4 여야 합니다 (서버 3 + 미설치 1)", z.Count)
		}
	}
}

func TestCollapseInbound(t *testing.T) {
	index := map[netip.Addr]string{netip.MustParseAddr("10.0.0.1"): "w"}
	in := func(peer string, n int) model.Flow {
		return model.Flow{Direction: model.DirIn, Proto: "tcp", PeerIP: peer, Port: 443, Process: "nginx", Count: n}
	}
	got := collapseInbound([]model.Flow{
		in("10.0.0.1", 1), in("10.0.5.5", 2), in("10.0.5.6", 3), in("8.8.8.8", 4), in("1.1.1.1", 5),
		{Direction: model.DirOut, Proto: "tcp", PeerIP: "10.0.5.5", Port: 22, Count: 1},
	}, index)
	want := map[string]int{"in|10.0.0.1": 1, "in|" + peerClients: 5, "in|" + peerInternet: 9, "out|10.0.5.5": 1}
	if len(got) != len(want) {
		t.Fatalf("흐름 수 = %d, %d 이어야 합니다: %+v", len(got), len(want), got)
	}
	for _, f := range got {
		if want[f.Direction+"|"+f.PeerIP] != f.Count {
			t.Errorf("%s %s = %d", f.Direction, f.PeerIP, f.Count)
		}
	}
}
