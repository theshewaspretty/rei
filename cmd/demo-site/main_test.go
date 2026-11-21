package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"infrascanner/internal/demo"
)

func pageData(t *testing.T, page []byte) siteData {
	t.Helper()
	_, rest, ok := strings.Cut(string(page), dataOpen)
	if !ok {
		t.Fatal("페이지에 데모 데이터가 없습니다")
	}
	raw, _, _ := strings.Cut(rest, "</script>")
	var d siteData
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("데모 데이터가 JSON 이 아닙니다: %v", err)
	}
	return d
}

// 만들어진 페이지가 데모로 쓸 만한 내용을 담고 있는지 확인한다.
func TestBuild(t *testing.T) {
	page, err := build()
	if err != nil {
		t.Fatal(err)
	}
	d := pageData(t, page)
	hosts := demo.Hosts()

	if got := d.Topology.Stats.Hosts; got != len(hosts) {
		t.Errorf("서버 수 = %d, %d 이어야 합니다", got, len(hosts))
	}
	if got := d.Topology.Stats.Offline; got != 1 {
		t.Errorf("응답 없는 서버 = %d, 1 이어야 합니다", got)
	}
	if !d.Topology.GeneratedAt.Equal(epoch) {
		t.Errorf("generated_at = %v, %v 이어야 합니다", d.Topology.GeneratedAt, epoch)
	}
	if len(d.Topology.Edges) < 30 || len(d.Topology.Findings) < 10 {
		t.Errorf("연결 %d개, 점검 항목 %d건: 데모로 쓰기에 너무 적습니다", len(d.Topology.Edges), len(d.Topology.Findings))
	}
	nodes := map[string]string{} // 노드 ID -> 상태
	for _, n := range d.Topology.Nodes {
		nodes[n.ID] = n.Status
	}
	for _, h := range hosts {
		id := idPrefix + h.Name
		status, ok := nodes["a:"+id]
		if !ok {
			t.Errorf("%s: 구성도에 노드가 없습니다", h.Name)
			continue
		}
		if want := map[bool]string{true: "offline", false: "online"}[h.StopsReporting]; status != want {
			t.Errorf("%s: 상태 = %q, %q 이어야 합니다", h.Name, status, want)
		}
		detail := d.Hosts[id]
		if detail == nil || detail.Report == nil {
			t.Errorf("%s: 서버 상세가 없습니다", h.Name)
			continue
		}
		if len(detail.Listeners) == 0 || len(detail.Outbound)+len(detail.Inbound) == 0 {
			t.Errorf("%s: 리스닝 포트나 통신 흐름이 비어 있습니다", h.Name)
		}
	}
	// 연결과 점검 항목이 가리키는 노드는 모두 구성도에 있어야 한다 (ID 를 바꿔 넣다가 어긋나지 않았는지).
	for _, e := range d.Topology.Edges {
		for _, id := range []string{e.Source, e.Target} {
			if _, ok := nodes[id]; !ok {
				t.Errorf("연결 %s 의 노드 %s 가 구성도에 없습니다", e.ID, id)
			}
		}
	}
	for _, f := range d.Topology.Findings {
		if f.NodeID != "" {
			if _, ok := nodes[f.NodeID]; !ok {
				t.Errorf("점검 항목 %s 의 노드 %s 가 구성도에 없습니다", f.Code, f.NodeID)
			}
		}
	}

	again, err := build()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(page, again) {
		t.Error("같은 소스에서 다른 페이지가 나왔습니다. 무작위 값이나 실제 시각이 섞여 들어갔는지 확인하세요")
	}
}

// docs/index.html 이 지금의 web/static · internal/demo · 서버 코드로 만든 것과 같은지 확인한다.
// 화면이나 서버를 고치고 데모를 다시 만들지 않으면 여기서 걸린다.
func TestDocsIndexIsUpToDate(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "index.html")
	have, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v\n저장소 루트에서 `go run ./cmd/demo-site` 를 실행해 만드세요", err)
	}
	want, err := build()
	if err != nil {
		t.Fatal(err)
	}
	// 체크아웃 설정에 따라 줄바꿈이 CRLF 로 바뀌어 있을 수 있다.
	if !bytes.Equal(bytes.ReplaceAll(have, []byte("\r\n"), []byte("\n")), want) {
		t.Errorf("%s 가 최신이 아닙니다. 저장소 루트에서 `go run ./cmd/demo-site` 를 실행한 뒤 함께 커밋하세요", path)
	}
}
