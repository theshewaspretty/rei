package server

import (
	"net/netip"
	"time"

	"infrascanner/internal/model"
)

// HostDetail 은 서버 1대의 상세 화면에 필요한 전부다.
type HostDetail struct {
	ID         string         `json:"id"`
	NodeID     string         `json:"node_id"`
	Hostname   string         `json:"hostname"`
	Status     string         `json:"status"`
	FirstSeen  time.Time      `json:"first_seen"`
	LastSeen   *time.Time     `json:"last_seen,omitempty"`
	RemoteAddr string         `json:"remote_addr"`
	WindowSec  int            `json:"window_sec"`
	Report     *model.Report  `json:"report,omitempty"`
	Listeners  []ListenerView `json:"listeners"`
	Outbound   []FlowView     `json:"outbound"`
	Inbound    []FlowView     `json:"inbound"`
	Local      []FlowView     `json:"local"`
	Findings   []Finding      `json:"findings"`
}

type ListenerView struct {
	model.Listener
	Service  string `json:"service"`
	Loopback bool   `json:"loopback"` // 이 서버 안에서만 접속 가능
}

type FlowView struct {
	PeerIP      string    `json:"peer_ip,omitempty"`
	PeerName    string    `json:"peer_name,omitempty"`
	PeerNodeID  string    `json:"peer_node_id,omitempty"`
	Proto       string    `json:"proto"`
	Port        int       `json:"port"`
	Service     string    `json:"service"`
	Plain       bool      `json:"plain"`
	Process     string    `json:"process,omitempty"`
	PeerProcess string    `json:"peer_process,omitempty"`
	MaxConns    int       `json:"max_conns"`
	LastSeen    time.Time `json:"last_seen"`
}

func (s *Server) hostDetail(agentID string, window time.Duration) (*HostDetail, error) {
	agents, index := s.snapshot()
	var rec *AgentRecord
	names := map[string]string{}
	for _, a := range agents {
		names[a.ID] = a.Hostname
		if a.ID == agentID {
			rec = a
		}
	}
	if rec == nil {
		return nil, errNotFound
	}
	now := time.Now()
	d := &HostDetail{
		ID: rec.ID, NodeID: "a:" + rec.ID, Hostname: rec.Hostname, Status: agentStatus(rec, now),
		FirstSeen: rec.FirstSeen.UTC(), RemoteAddr: rec.RemoteAddr, WindowSec: int(window / time.Second),
		Report: rec.Report, Listeners: []ListenerView{}, Outbound: []FlowView{}, Inbound: []FlowView{}, Local: []FlowView{}, Findings: []Finding{},
	}
	if !rec.LastSeen.IsZero() {
		t := rec.LastSeen.UTC()
		d.LastSeen = &t
	}
	if rec.Report != nil {
		for _, l := range rec.Report.Listeners {
			svc := ""
			if info, ok := wellKnown[l.Port]; ok && l.Proto == "tcp" {
				svc = info.Name
			}
			d.Listeners = append(d.Listeners, ListenerView{Listener: l, Service: svc, Loopback: isLoopbackBind(l.IP)})
		}
	}

	rows, err := s.store.AgentFlows(agentID, now.Add(-window))
	if err != nil {
		return nil, err
	}
	for _, f := range rows {
		info := lookupService(f.Port)
		v := FlowView{
			Proto: f.Proto, Port: f.Port, Service: info.Name, Plain: info.Plain,
			Process: f.Process, PeerProcess: f.PeerProcess, MaxConns: f.MaxConns, LastSeen: f.LastSeen.UTC(),
		}
		switch f.PeerIP {
		case peerInternet:
			v.PeerName, v.PeerNodeID = titleInternet, nodeInternet
		case peerClients:
			v.PeerName, v.PeerNodeID = titleClients, nodeClients
		default:
			v.PeerIP = f.PeerIP
			if a, err := netip.ParseAddr(f.PeerIP); err == nil {
				if id, ok := index[a]; ok {
					v.PeerName, v.PeerNodeID = names[id], "a:"+id
				} else if f.Direction == model.DirOut {
					if isInternal(a) {
						v.PeerNodeID = "u:" + f.PeerIP
					} else {
						v.PeerNodeID = nodeInternet
					}
				}
			}
		}
		switch f.Direction {
		case model.DirOut:
			d.Outbound = append(d.Outbound, v)
		case model.DirIn:
			d.Inbound = append(d.Inbound, v)
		case model.DirLocal:
			d.Local = append(d.Local, v)
		}
	}

	if topo, err := s.topology(window); err == nil {
		for _, f := range topo.Findings {
			if f.NodeID == d.NodeID {
				d.Findings = append(d.Findings, f)
			}
		}
	}
	return d, nil
}
