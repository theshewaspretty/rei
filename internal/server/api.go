package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"path/filepath"
	"strings"
	"time"

	"infrascanner/internal/model"
)

const (
	maxReportBytes = 4 << 20
	maxListeners   = 4000
	maxFlows       = 2000
	maxInterfaces  = 256

	// 에이전트가 없는 상대에게서 들어온 연결은 IP 를 남기지 않고 이 둘로 묶는다.
	// 접속해 오는 쪽(사용자 PC, 인터넷)은 수가 끝없이 늘 수 있기 때문이다.
	peerInternet = "internet"
	peerClients  = "clients"
)

var windows = map[string]time.Duration{
	"1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour,
}

func parseWindow(r *http.Request) time.Duration {
	if d, ok := windows[r.URL.Query().Get("window")]; ok {
		return d
	}
	return 24 * time.Hour
}

// ---- 에이전트 -----------------------------------------------------------------

func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	ip := "enroll:" + clientIP(r)
	if d := s.limiter.blocked(ip); d > 0 {
		writeError(w, http.StatusTooManyRequests, "too_many_enrolls", "등록 시도가 너무 많습니다")
		return
	}
	var req model.EnrollRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "요청 형식이 올바르지 않습니다")
		return
	}
	if !constEq(req.Token, s.cfg.EnrollToken) {
		s.limiter.fail(ip)
		s.logger.Printf("에이전트 등록 거부(토큰 불일치): %s", clientIP(r))
		writeError(w, http.StatusUnauthorized, "bad_enroll_token", "등록 토큰이 올바르지 않습니다")
		return
	}
	req.MachineID, req.Hostname = clip(req.MachineID, 200), clip(req.Hostname, 255)
	if req.MachineID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "machine_id 가 필요합니다")
		return
	}

	token := randomToken(32)
	rec, err := s.store.EnrollAgent(randomToken(9), req.MachineID, req.Hostname, hashToken(token), time.Now())
	if err != nil {
		s.logger.Printf("에이전트 등록 실패: %v", err)
		writeError(w, http.StatusInternalServerError, "enroll_failed", "등록을 저장하지 못했습니다")
		return
	}
	s.mu.Lock()
	s.agents[rec.ID] = rec
	s.mu.Unlock()
	s.invalidateTopology()
	s.logger.Printf("에이전트 등록: %s (%s) from %s", rec.ID, rec.Hostname, clientIP(r))
	writeJSON(w, http.StatusOK, model.EnrollResponse{AgentID: rec.ID, AgentToken: token})
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.authAgent(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "agent_auth_failed", "에이전트 인증에 실패했습니다")
		return
	}
	var rep model.Report
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReportBytes)).Decode(&rep); err != nil {
		writeError(w, http.StatusBadRequest, "bad_report", "보고 형식이 올바르지 않습니다")
		return
	}
	if err := sanitizeReport(&rep); err != nil {
		writeError(w, http.StatusBadRequest, "bad_report", err.Error())
		return
	}

	s.mu.RLock()
	index := s.ipIndex
	s.mu.RUnlock()
	flows := collapseInbound(rep.Flows, index)
	rep.Flows = nil // 흐름은 flows 테이블에 누적하고, 스냅샷에는 넣지 않는다

	now := time.Now()
	if err := s.store.SaveReport(agent.ID, &rep, flows, clientIP(r), now); err != nil {
		s.logger.Printf("보고 저장 실패 (%s): %v", agent.ID, err)
		writeError(w, http.StatusInternalServerError, "report_failed", "보고를 저장하지 못했습니다")
		return
	}

	next := *agent
	next.Hostname, next.LastSeen, next.RemoteAddr, next.Report = rep.Host.Hostname, now, clientIP(r), &rep
	s.mu.Lock()
	if _, still := s.agents[agent.ID]; still { // 저장하는 사이 삭제됐으면 되살리지 않는다
		s.agents[agent.ID] = &next
		s.rebuildIPIndexLocked()
	}
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// sanitizeReport 는 에이전트가 보낸 값을 그대로 믿지 않고 길이와 범위를 맞춘다.
func sanitizeReport(r *model.Report) error {
	h := &r.Host
	h.Hostname = clip(strings.TrimSpace(h.Hostname), 255)
	if h.Hostname == "" {
		return errors.New("hostname 이 비어 있습니다")
	}
	for _, p := range []*string{&h.Zone, &h.OS, &h.Platform, &h.PlatformVersion, &h.KernelVersion, &h.Arch, &h.Virtualization, &h.CPUModel, &r.AgentVersion} {
		*p = clip(strings.TrimSpace(*p), 120)
	}
	if r.IntervalSec < 15 || r.IntervalSec > 3600 {
		r.IntervalSec = 60
	}
	if a, err := netip.ParseAddr(h.PrimaryIP); err == nil {
		h.PrimaryIP = a.Unmap().String()
	} else {
		h.PrimaryIP = ""
	}
	if p, err := netip.ParsePrefix(h.PrimaryCIDR); err == nil {
		h.PrimaryCIDR = p.Masked().String()
	} else {
		h.PrimaryCIDR = ""
	}
	if len(h.Disks) > 64 {
		h.Disks = h.Disks[:64]
	}
	for i := range h.Disks {
		h.Disks[i].Mount, h.Disks[i].FSType = clip(h.Disks[i].Mount, 200), clip(h.Disks[i].FSType, 40)
	}

	if len(r.Interfaces) > maxInterfaces {
		r.Interfaces = r.Interfaces[:maxInterfaces]
	}
	for i := range r.Interfaces {
		ifc := &r.Interfaces[i]
		ifc.Name, ifc.MAC = clip(ifc.Name, 120), clip(ifc.MAC, 40)
		addrs := ifc.Addrs[:0]
		for _, c := range ifc.Addrs {
			if p, err := netip.ParsePrefix(c); err == nil && len(addrs) < 64 {
				addrs = append(addrs, p.String())
			}
		}
		ifc.Addrs = addrs
	}

	listeners := r.Listeners[:0]
	for _, l := range r.Listeners {
		if len(listeners) >= maxListeners || !validProto(l.Proto) || !validPort(l.Port) {
			continue
		}
		if a, err := netip.ParseAddr(l.IP); err == nil {
			l.IP = a.Unmap().String()
		} else {
			l.IP = ""
		}
		l.Process = clip(l.Process, 120)
		listeners = append(listeners, l)
	}
	r.Listeners = listeners

	flows := r.Flows[:0]
	for _, f := range r.Flows {
		if len(flows) >= maxFlows || f.Proto != "tcp" || !validPort(f.Port) {
			continue
		}
		if f.Direction != model.DirOut && f.Direction != model.DirIn && f.Direction != model.DirLocal {
			continue
		}
		a, err := netip.ParseAddr(f.PeerIP)
		if err != nil {
			continue
		}
		f.PeerIP = a.Unmap().String()
		f.Process, f.PeerProcess = clip(f.Process, 120), clip(f.PeerProcess, 120)
		if f.Count < 1 {
			f.Count = 1
		}
		flows = append(flows, f)
	}
	r.Flows = flows

	if len(r.Warnings) > 20 {
		r.Warnings = r.Warnings[:20]
	}
	for i := range r.Warnings {
		r.Warnings[i] = clip(r.Warnings[i], 300)
	}
	return nil
}

// collapseInbound 는 에이전트가 없는 상대에게서 들어온 흐름을 인터넷/내부 클라이언트로 묶고,
// 묶으면서 같아진 항목을 합친다. 나가는 흐름과 에이전트끼리의 흐름은 그대로 둔다.
func collapseInbound(flows []model.Flow, index map[netip.Addr]string) []model.Flow {
	type key struct {
		dir, peer, proc string
		port            int
	}
	merged := map[key]int{}
	out := make([]model.Flow, 0, len(flows))
	for _, f := range flows {
		if f.Direction == model.DirIn {
			a, _ := netip.ParseAddr(f.PeerIP)
			if _, known := index[a]; !known {
				if isInternal(a) {
					f.PeerIP = peerClients
				} else {
					f.PeerIP = peerInternet
				}
			}
		}
		k := key{f.Direction, f.PeerIP, f.Process, f.Port}
		if i, dup := merged[k]; dup {
			out[i].Count += f.Count
			continue
		}
		merged[k] = len(out)
		out = append(out, f)
	}
	return out
}

func validProto(p string) bool { return p == "tcp" || p == "udp" }
func validPort(p int) bool     { return p >= 1 && p <= 65535 }

// clip 은 문자열을 max 바이트 이하로 자르되 UTF-8 글자 중간에서 끊지 않는다.
func clip(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut]
}

// ---- 관리자 -------------------------------------------------------------------

func (s *Server) handleTopology(w http.ResponseWriter, r *http.Request) {
	topo, err := s.topology(parseWindow(r))
	if err != nil {
		s.logger.Printf("구성도 생성 실패: %v", err)
		writeError(w, http.StatusInternalServerError, "topology_failed", "구성도를 만들지 못했습니다")
		return
	}
	writeJSON(w, http.StatusOK, topo)
}

func (s *Server) handleHost(w http.ResponseWriter, r *http.Request) {
	detail, err := s.hostDetail(r.PathValue("id"), parseWindow(r))
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "host_not_found", "서버를 찾을 수 없습니다")
		return
	}
	if err != nil {
		s.logger.Printf("서버 상세 조회 실패: %v", err)
		writeError(w, http.StatusInternalServerError, "host_read_failed", "서버 정보를 읽지 못했습니다")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleDeleteHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	rec, ok := s.agents[id]
	if ok {
		delete(s.agents, id)
		s.rebuildIPIndexLocked()
	}
	s.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, "host_not_found", "서버를 찾을 수 없습니다")
		return
	}
	if err := s.store.DeleteAgent(id); err != nil {
		s.logger.Printf("에이전트 삭제 실패 (%s): %v", id, err)
		writeError(w, http.StatusInternalServerError, "delete_failed", "삭제하지 못했습니다")
		return
	}
	s.invalidateTopology()
	s.logger.Printf("에이전트 삭제: %s (%s)", id, rec.Hostname)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSaveLayout(w http.ResponseWriter, r *http.Request) {
	var points map[string]Point
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&points); err != nil || len(points) > 20000 {
		writeError(w, http.StatusBadRequest, "bad_request", "요청 형식이 올바르지 않습니다")
		return
	}
	for id := range points {
		if len(id) > 200 {
			writeError(w, http.StatusBadRequest, "bad_request", "요청 형식이 올바르지 않습니다")
			return
		}
	}
	if err := s.store.SaveLayout(points); err != nil {
		s.logger.Printf("배치 저장 실패: %v", err)
		writeError(w, http.StatusInternalServerError, "layout_save_failed", "배치를 저장하지 못했습니다")
		return
	}
	s.invalidateTopology()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResetLayout(w http.ResponseWriter, _ *http.Request) {
	if err := s.store.ResetLayout(); err != nil {
		s.logger.Printf("배치 초기화 실패: %v", err)
		writeError(w, http.StatusInternalServerError, "layout_reset_failed", "배치를 초기화하지 못했습니다")
		return
	}
	s.invalidateTopology()
	w.WriteHeader(http.StatusNoContent)
}

// handleDownload 는 시작할 때 확인해 둔 에이전트 실행 파일만 내려준다. 폴더 목록이나 다른 파일은 보여 주지 않는다.
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	for _, d := range s.downloads {
		if d.Name == name {
			w.Header().Set("Content-Type", "application/octet-stream")
			http.ServeFile(w, r, filepath.Join(s.cfg.DownloadsDir, d.Name))
			return
		}
	}
	http.NotFound(w, r)
}

func (s *Server) handleInstallInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"downloads":   s.downloads,
		"cert_sha256": s.certSHA,
		"tls_mode":    s.cfg.TLSMode,
	})
}
