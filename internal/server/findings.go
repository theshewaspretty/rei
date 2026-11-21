package server

import (
	"sort"
	"strings"
	"time"
)

// Finding 은 수집된 사실에서 나온 점검 항목 1건이다.
// 문장은 서버에서 만들지 않는다. 화면이 Code 와 Args 로 선택된 언어의 문장을 만든다
// (web/static/i18n.js 의 f.<code>.title / f.<code>.detail).
type Finding struct {
	Severity string         `json:"severity"` // high, medium, low
	Code     string         `json:"code"`
	Args     map[string]any `json:"args"`
	NodeID   string         `json:"node_id,omitempty"`
	EdgeID   string         `json:"edge_id,omitempty"`

	subject string // 같은 등급·코드 안에서의 정렬 기준
}

// findingCodes 는 서버가 만들 수 있는 점검 코드 전부다. 번역이 빠지지 않았는지 테스트에서 확인한다.
var findingCodes = []string{
	"agent_pending", "agent_offline", "disk_full", "disk_high", "mem_high",
	"telnet_listen", "ftp_listen", "agent_unprivileged", "flows_truncated", "agent_warning",
	"internet_exposed", "plaintext_protocol", "plaintext_flow", "unmanaged_host",
}

var severityRank = map[string]int{"high": 0, "medium": 1, "low": 2}

const (
	diskWarnPct = 90
	diskCritPct = 95
	memWarnPct  = 90
)

func buildFindings(agents []*AgentRecord, topo *Topology, now time.Time) []Finding {
	type args = map[string]any
	out := []Finding{}
	add := func(sev, code, subject, nodeID, edgeID string, a args) {
		out = append(out, Finding{Severity: sev, Code: code, Args: a, NodeID: nodeID, EdgeID: edgeID, subject: subject})
	}

	for _, a := range agents {
		nodeID := "a:" + a.ID
		host := a.Hostname
		status := agentStatus(a, now)
		switch status {
		case "pending":
			add("low", "agent_pending", host, nodeID, "", args{"host": host})
			continue
		case "offline":
			add("high", "agent_offline", host, nodeID, "", args{"host": host, "age_sec": int(now.Sub(a.LastSeen) / time.Second)})
		}
		rep := a.Report
		h := rep.Host
		if status == "offline" {
			h.Disks, h.MemTotal = nil, 0 // 응답이 없는 서버의 사용률은 지난 값이라 점검하지 않는다
		}

		for _, d := range h.Disks {
			if d.Total == 0 {
				continue
			}
			pct := float64(d.Used) / float64(d.Total) * 100
			da := args{"host": host, "mount": d.Mount, "pct": roundPct(pct)}
			switch {
			case pct >= diskCritPct:
				add("high", "disk_full", host+d.Mount, nodeID, "", da)
			case pct >= diskWarnPct:
				add("medium", "disk_high", host+d.Mount, nodeID, "", da)
			}
		}
		if h.MemTotal > 0 {
			if pct := float64(h.MemUsed) / float64(h.MemTotal) * 100; pct >= memWarnPct {
				add("medium", "mem_high", host, nodeID, "", args{"host": host, "pct": roundPct(pct)})
			}
		}

		unnamed, tcpListeners := 0, 0
		for _, l := range rep.Listeners {
			if l.Proto != "tcp" {
				continue
			}
			tcpListeners++
			if l.Process == "" {
				unnamed++
			}
			if isLoopbackBind(l.IP) {
				continue
			}
			switch l.Port {
			case 23:
				add("high", "telnet_listen", host, nodeID, "", args{"host": host})
			case 21:
				add("medium", "ftp_listen", host, nodeID, "", args{"host": host})
			}
		}
		if tcpListeners > 0 && unnamed*2 > tcpListeners {
			add("low", "agent_unprivileged", host, nodeID, "", args{"host": host, "total": tcpListeners, "unnamed": unnamed})
		}
		if rep.FlowsTruncated {
			add("low", "flows_truncated", host, nodeID, "", args{"host": host})
		}
		for _, w := range rep.Warnings {
			add("low", "agent_warning", host+w, nodeID, "", args{"host": host, "message": w})
		}
	}

	for _, e := range topo.Edges {
		var exposed, plainHigh, plainLow []string
		for _, sv := range e.Services {
			info := lookupService(sv.Port)
			label := serviceLabel(sv.Port)
			if e.Source == nodeInternet && (info.Cat == catDB || info.Cat == catMgmt) {
				exposed = append(exposed, label)
			}
			if sv.Plain && e.Target != nodeInternet {
				if sv.Port == 21 || sv.Port == 23 {
					plainHigh = append(plainHigh, label)
				} else {
					plainLow = append(plainLow, label)
				}
			}
		}
		// source / target 은 노드 ID 다. "인터넷" 같은 묶음 노드의 이름은 언어마다 달라 화면에서 바꾼다.
		if len(exposed) > 0 {
			add("high", "internet_exposed", e.ID, e.Target, e.ID, args{"target": e.Target, "services": exposed})
		}
		if len(plainHigh) > 0 {
			add("medium", "plaintext_protocol", e.ID, "", e.ID, args{"source": e.Source, "target": e.Target, "services": plainHigh})
		}
		if len(plainLow) > 0 {
			add("low", "plaintext_flow", e.ID, "", e.ID, args{"source": e.Source, "target": e.Target, "services": plainLow})
		}
	}

	clients := map[string]int{}
	for _, e := range topo.Edges {
		if strings.HasPrefix(e.Target, "u:") {
			clients[e.Target]++
		}
	}
	for _, n := range topo.Nodes {
		if n.Kind != "unmanaged" {
			continue
		}
		services := n.Tags
		if services == nil {
			services = []string{}
		}
		add("low", "unmanaged_host", n.ID, n.ID, "", args{"ip": n.IP, "n": clients[n.ID], "services": services})
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if severityRank[a.Severity] != severityRank[b.Severity] {
			return severityRank[a.Severity] < severityRank[b.Severity]
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.subject < b.subject
	})
	return out
}

func roundPct(v float64) int { return int(v + 0.5) }
