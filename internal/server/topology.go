package server

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"infrascanner/internal/model"
)

var errNotFound = errors.New("not found")

const topoCacheTTL = 5 * time.Second

// 구성도 JSON. 노드·연결선의 필드 이름은 test.html 의 구성도 형식과 같다.
type Topology struct {
	GeneratedAt time.Time        `json:"generated_at"`
	WindowSec   int              `json:"window_sec"`
	Zones       []TopoZone       `json:"zones"`
	Nodes       []TopoNode       `json:"nodes"`
	Edges       []TopoEdge       `json:"edges"`
	Layout      map[string]Point `json:"layout"`
	Stats       TopoStats        `json:"stats"`
	Findings    []Finding        `json:"findings"`
}

// 화면에 보이는 문장(title, subtitle)의 기본값은 영어다. 관제 화면은 id / kind / status 를 보고
// 선택된 언어로 바꿔 그린다 (web/static/app.js 의 localizeTopo).
type TopoZone struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Accent   string `json:"accent"`
	Count    int    `json:"count"` // 이 영역에 속한 노드 수
}

type TopoNode struct {
	ID       string     `json:"id"`
	Kind     string     `json:"kind"` // host, unmanaged, internet, clients
	Zone     string     `json:"zone,omitempty"`
	Title    string     `json:"title"`
	Subtitle string     `json:"subtitle"`
	Role     string     `json:"role,omitempty"` // host 만: db, queue, web, server (열린 포트로 추정한 역할)
	Icon     string     `json:"icon"`
	Accent   string     `json:"accent"`
	Tags     []string   `json:"tags"`
	Status   string     `json:"status"` // online, offline, pending, unmanaged, external
	IP       string     `json:"ip,omitempty"`
	LastSeen *time.Time `json:"last_seen,omitempty"`
}

type TopoEdge struct {
	ID       string        `json:"id"`
	Source   string        `json:"source"`
	Target   string        `json:"target"`
	Label    string        `json:"label"`
	Color    string        `json:"color"`
	Plain    bool          `json:"plain"` // 포트 기준으로 평문일 수 있는 서비스가 섞여 있다
	Services []EdgeService `json:"services"`
}

type EdgeService struct {
	Proto       string    `json:"proto"`
	Port        int       `json:"port"`
	Name        string    `json:"name"`
	Cat         string    `json:"cat"` // web, db, queue, mgmt, infra, other
	Plain       bool      `json:"plain"`
	ClientProcs []string  `json:"client_procs,omitempty"`
	ServerProcs []string  `json:"server_procs,omitempty"`
	MaxConns    int       `json:"max_conns"`
	LastSeen    time.Time `json:"last_seen"`
}

type TopoStats struct {
	Hosts     int `json:"hosts"`
	Online    int `json:"online"`
	Offline   int `json:"offline"`
	Unmanaged int `json:"unmanaged"`
	Edges     int `json:"edges"`
	Zones     int `json:"zones"`
}

const (
	nodeInternet  = "x:internet"
	nodeClients   = "x:clients"
	zoneUnmanaged = "z:unmanaged"

	titleInternet = "Internet / external"
	titleClients  = "Internal clients"
)

// topology 는 캐시된 구성도를 돌려주거나 새로 만든다.
func (s *Server) topology(window time.Duration) (*Topology, error) {
	s.topoMu.Lock()
	defer s.topoMu.Unlock()
	if c, ok := s.topoCache[window]; ok && time.Since(c.at) < topoCacheTTL {
		return c.topo, nil
	}
	now := time.Now()
	agents, index := s.snapshot()
	rows, err := s.store.Flows(now.Add(-window))
	if err != nil {
		return nil, err
	}
	layout, err := s.store.Layout()
	if err != nil {
		return nil, err
	}
	topo := buildTopology(agents, index, rows, layout, window, now)
	s.topoCache[window] = cachedTopo{at: now, topo: topo}
	return topo, nil
}

type svcKey struct {
	proto string
	port  int
}

type svcAgg struct {
	clientProcs map[string]struct{}
	serverProcs map[string]struct{}
	maxConns    int
	lastSeen    time.Time
}

type edgeAgg struct {
	src, dst string
	svcs     map[svcKey]*svcAgg
}

// resolver 는 흐름의 상대 주소를 구성도 노드로 바꾼다.
type resolver struct {
	index     map[netip.Addr]string
	unmanaged map[string]struct{} // 에이전트가 없는 내부 서버 IP
}

// target 은 이 서버가 접속한 상대다. 에이전트가 없는 내부 주소는 "미설치 서버" 노드가 된다.
func (r *resolver) target(agentID, peer string) (string, bool) {
	a, err := netip.ParseAddr(peer)
	if err != nil {
		return "", false
	}
	if id, ok := r.index[a]; ok {
		return "a:" + id, id != agentID
	}
	if isInternal(a) {
		r.unmanaged[peer] = struct{}{}
		return "u:" + peer, true
	}
	return nodeInternet, true
}

// source 는 이 서버로 접속해 온 상대다. 에이전트가 없는 쪽은 저장할 때 이미 묶여 있다.
func (r *resolver) source(agentID, peer string) (string, bool) {
	switch peer {
	case peerInternet:
		return nodeInternet, true
	case peerClients:
		return nodeClients, true
	}
	a, err := netip.ParseAddr(peer)
	if err != nil {
		return "", false
	}
	if id, ok := r.index[a]; ok {
		return "a:" + id, id != agentID
	}
	if isInternal(a) {
		return nodeClients, true
	}
	return nodeInternet, true
}

func buildTopology(agents []*AgentRecord, index map[netip.Addr]string, rows []FlowRow, layout map[string]Point, window time.Duration, now time.Time) *Topology {
	topo := &Topology{GeneratedAt: now.UTC(), WindowSec: int(window / time.Second), Layout: map[string]Point{}}

	// 1. 에이전트가 설치된 서버와 영역
	zones := map[string]*TopoZone{}
	zoneCount := map[string]int{}
	var prefixes []prefixZone
	known := map[string]bool{}
	for _, a := range agents {
		known[a.ID] = true
		zid, ztitle := zoneOf(a)
		if _, ok := zones[zid]; !ok {
			zones[zid] = &TopoZone{ID: zid, Title: ztitle, Accent: "slate"}
		}
		zoneCount[zid]++
		if a.Report != nil {
			if p, err := netip.ParsePrefix(a.Report.Host.PrimaryCIDR); err == nil {
				prefixes = append(prefixes, prefixZone{p, zid})
			}
		}
		n := hostNode(a, now)
		n.Zone = zid
		topo.Nodes = append(topo.Nodes, n)
		topo.Stats.Hosts++
		switch n.Status {
		case "online":
			topo.Stats.Online++
		case "offline":
			topo.Stats.Offline++
		}
	}

	// 2. 흐름을 (출발 노드, 도착 노드) 단위로 묶는다
	res := &resolver{index: index, unmanaged: map[string]struct{}{}}
	edges := map[string]*edgeAgg{}
	for _, f := range rows {
		if !known[f.AgentID] {
			continue
		}
		self := "a:" + f.AgentID
		var src, dst string
		var ok bool
		switch f.Direction {
		case model.DirOut:
			src = self
			dst, ok = res.target(f.AgentID, f.PeerIP)
		case model.DirIn:
			dst = self
			src, ok = res.source(f.AgentID, f.PeerIP)
		}
		if !ok {
			continue
		}
		key := src + ">" + dst
		e := edges[key]
		if e == nil {
			e = &edgeAgg{src: src, dst: dst, svcs: map[svcKey]*svcAgg{}}
			edges[key] = e
		}
		sk := svcKey{f.Proto, f.Port}
		sv := e.svcs[sk]
		if sv == nil {
			sv = &svcAgg{clientProcs: map[string]struct{}{}, serverProcs: map[string]struct{}{}}
			e.svcs[sk] = sv
		}
		if f.Process != "" {
			if f.Direction == model.DirOut {
				sv.clientProcs[f.Process] = struct{}{}
			} else {
				sv.serverProcs[f.Process] = struct{}{}
			}
		}
		if f.MaxConns > sv.maxConns {
			sv.maxConns = f.MaxConns
		}
		if f.LastSeen.After(sv.lastSeen) {
			sv.lastSeen = f.LastSeen
		}
	}

	for _, e := range edges {
		te := TopoEdge{ID: "e:" + e.src + ">" + e.dst, Source: e.src, Target: e.dst}
		for k, sv := range e.svcs {
			info := lookupService(k.port)
			te.Services = append(te.Services, EdgeService{
				Proto: k.proto, Port: k.port, Name: info.Name, Cat: info.Cat, Plain: info.Plain,
				ClientProcs: sortedKeys(sv.clientProcs, 5), ServerProcs: sortedKeys(sv.serverProcs, 5),
				MaxConns: sv.maxConns, LastSeen: sv.lastSeen.UTC(),
			})
			te.Plain = te.Plain || info.Plain
		}
		// 알려진 서비스를 앞에, 그 안에서는 포트 순으로 둔다. 맨 앞 서비스가 연결선의 라벨과 색이 된다.
		sort.Slice(te.Services, func(i, j int) bool {
			_, ki := wellKnown[te.Services[i].Port]
			_, kj := wellKnown[te.Services[j].Port]
			if ki != kj {
				return ki
			}
			return te.Services[i].Port < te.Services[j].Port
		})
		te.Label = serviceLabel(te.Services[0].Port)
		if extra := len(te.Services) - 1; extra > 0 {
			te.Label += fmt.Sprintf(" +%d", extra)
		}
		te.Color = catColor[lookupService(te.Services[0].Port).Cat]
		topo.Edges = append(topo.Edges, te)
	}
	sort.Slice(topo.Edges, func(i, j int) bool { return topo.Edges[i].ID < topo.Edges[j].ID })

	// 3. 에이전트가 없는 내부 서버, 인터넷, 내부 클라이언트 노드
	servedBy := map[string][]string{} // 노드 -> 그 노드가 제공하는 서비스 이름
	used := map[string]bool{}
	for _, e := range topo.Edges {
		used[e.Source], used[e.Target] = true, true
		for _, sv := range e.Services {
			servedBy[e.Target] = appendUnique(servedBy[e.Target], sv.Name)
		}
	}
	unmanagedIPs := make([]string, 0, len(res.unmanaged))
	for ip := range res.unmanaged {
		unmanagedIPs = append(unmanagedIPs, ip)
	}
	sort.Slice(unmanagedIPs, func(i, j int) bool {
		a, _ := netip.ParseAddr(unmanagedIPs[i])
		b, _ := netip.ParseAddr(unmanagedIPs[j])
		return a.Less(b)
	})
	for _, ip := range unmanagedIPs {
		id := "u:" + ip
		a, _ := netip.ParseAddr(ip)
		zid := zoneForAddr(a, prefixes)
		if zid == "" {
			zid = zoneUnmanaged
			if _, ok := zones[zid]; !ok {
				zones[zid] = &TopoZone{ID: zid, Title: "Unclassified internal servers", Accent: "slate"}
			}
		}
		zoneCount[zid]++
		tags := servedBy[id]
		if len(tags) > 3 {
			tags = tags[:3]
		}
		topo.Nodes = append(topo.Nodes, TopoNode{
			ID: id, Kind: "unmanaged", Zone: zid, Title: ip, Subtitle: "No agent installed",
			Icon: "help", Accent: "slate", Tags: tags, Status: "unmanaged", IP: ip,
		})
		topo.Stats.Unmanaged++
	}
	if used[nodeInternet] {
		topo.Nodes = append(topo.Nodes, TopoNode{
			ID: nodeInternet, Kind: "internet", Title: titleInternet, Subtitle: "Public IP ranges",
			Icon: "globe", Accent: "rose", Tags: []string{"External"}, Status: "external",
		})
	}
	if used[nodeClients] {
		topo.Nodes = append(topo.Nodes, TopoNode{
			ID: nodeClients, Kind: "clients", Title: titleClients, Subtitle: "Internal sources without an agent",
			Icon: "users", Accent: "slate", Tags: []string{"Internal"}, Status: "external",
		})
	}

	// 4. 영역
	for id, z := range zones {
		z.Count = zoneCount[id]
		z.Subtitle = fmt.Sprintf("%d nodes", z.Count)
		topo.Zones = append(topo.Zones, *z)
	}
	sort.Slice(topo.Zones, func(i, j int) bool {
		a, b := topo.Zones[i], topo.Zones[j]
		if (a.ID == zoneUnmanaged) != (b.ID == zoneUnmanaged) {
			return b.ID == zoneUnmanaged // 미분류는 맨 뒤
		}
		return a.Title < b.Title
	})

	// 5. 저장된 배치는 지금 있는 노드 것만 돌려준다
	for _, n := range topo.Nodes {
		if p, ok := layout[n.ID]; ok {
			topo.Layout[n.ID] = p
		}
	}

	topo.Stats.Edges, topo.Stats.Zones = len(topo.Edges), len(topo.Zones)
	topo.Findings = buildFindings(agents, topo, now)
	return topo
}

type prefixZone struct {
	prefix netip.Prefix
	zone   string
}

// zoneOf: 설치 때 지정한 영역 이름이 있으면 그것으로, 없으면 서브넷으로 묶는다.
func zoneOf(a *AgentRecord) (id, title string) {
	if a.Report == nil {
		return "z:pending", "Awaiting report"
	}
	if z := a.Report.Host.Zone; z != "" {
		return "z:l:" + z, z
	}
	if c := a.Report.Host.PrimaryCIDR; c != "" {
		return "z:n:" + c, c
	}
	return "z:none", "Unknown subnet"
}

// zoneForAddr 는 주소가 속한 가장 좁은 서브넷의 영역을 찾는다.
func zoneForAddr(a netip.Addr, prefixes []prefixZone) string {
	best, bestBits := "", -1
	for _, p := range prefixes {
		if p.prefix.Contains(a) && p.prefix.Bits() > bestBits {
			best, bestBits = p.zone, p.prefix.Bits()
		}
	}
	return best
}

func hostNode(a *AgentRecord, now time.Time) TopoNode {
	n := TopoNode{
		ID: "a:" + a.ID, Kind: "host", Title: a.Hostname, Subtitle: defaultRole.Label, Role: roleServer,
		Icon: defaultRole.Icon, Accent: defaultRole.Accent, Tags: []string{}, Status: agentStatus(a, now),
	}
	if !a.LastSeen.IsZero() {
		t := a.LastSeen.UTC()
		n.LastSeen = &t
	}
	if a.Report == nil {
		n.Subtitle = "Waiting for the first report"
		return n
	}
	h := a.Report.Host
	n.IP = h.PrimaryIP

	// 외부에서 닿을 수 있는(루프백 전용이 아닌) TCP 리스너로 역할을 추정한다.
	cats := map[string]bool{}
	var svcNames []string
	for _, l := range a.Report.Listeners {
		if l.Proto != "tcp" || isLoopbackBind(l.IP) {
			continue
		}
		info, ok := wellKnown[l.Port]
		if !ok {
			continue
		}
		cats[info.Cat] = true
		if info.Cat == catDB || info.Cat == catQueue || info.Cat == catWeb {
			svcNames = appendUnique(svcNames, info.Name)
		}
	}
	for _, c := range rolePriority {
		if cats[c] {
			r := roles[c]
			n.Subtitle, n.Role, n.Icon, n.Accent = r.Label, c, r.Icon, r.Accent
			break
		}
	}

	if os := osLabel(h); os != "" {
		n.Tags = append(n.Tags, os)
	}
	if h.PrimaryIP != "" {
		n.Tags = append(n.Tags, h.PrimaryIP)
	}
	for _, s := range svcNames {
		if len(n.Tags) >= 4 {
			break
		}
		n.Tags = append(n.Tags, s)
	}
	return n
}

func isLoopbackBind(ip string) bool {
	a, err := netip.ParseAddr(ip)
	return err == nil && a.IsLoopback()
}

// osLabel 은 "Windows Server 2019 Standard", "Ubuntu 22.04" 같은 짧은 표시 이름이다.
func osLabel(h model.Host) string {
	if h.OS == "windows" {
		return strings.TrimPrefix(h.Platform, "Microsoft ")
	}
	if h.Platform == "" {
		return h.OS
	}
	name := strings.ToUpper(h.Platform[:1]) + h.Platform[1:]
	if h.PlatformVersion != "" {
		name += " " + h.PlatformVersion
	}
	return name
}

func sortedKeys(m map[string]struct{}, limit int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
