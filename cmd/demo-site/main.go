// demo-site: 수집 서버 없이 열리는 정적 데모 페이지(docs/index.html)를 만든다.
// GitHub Pages 에 그대로 올리거나 파일을 브라우저로 바로 열 수 있다.
//
//	go run ./cmd/demo-site
//
// 화면은 web/static 의 파일을 그대로 넣고, 데이터는 internal/demo 의 가상 서버를 실제 수집 서버 코드에
// 보고시켜 얻은 응답을 넣는다. 그래서 화면이나 서버를 고친 뒤 다시 실행하면 데모도 같이 따라온다.
// 같은 소스에서는 항상 같은 파일이 나오며, main_test.go 가 docs/index.html 이 최신인지 확인한다.
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"infrascanner/internal/demo"
	"infrascanner/internal/model"
	"infrascanner/internal/server"
	"infrascanner/web"
)

// demoJS 는 화면의 API 호출에 페이지 안의 데이터로 답하는 스크립트다.
//
//go:embed demo.js
var demoJS string

// epoch 는 데이터 안의 "지금"이다. 고정해 두어야 같은 소스에서 같은 파일이 나온다.
// 페이지를 열면 demo.js 가 모든 시각을 실제 현재 시각에 맞춰 옮긴다.
var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

const (
	idPrefix   = "demo-" // 에이전트 ID 는 등록할 때마다 무작위로 정해지므로 "demo-<호스트명>" 으로 바꿔 넣는다
	exampleURL = "https://scanner.example.internal:8443"
	dataOpen   = `<script id="demo-data" type="application/json">`
)

// siteData 는 페이지에 넣는 데이터 전부다. demo.js 가 이것으로 /api/v1/* 응답을 만든다.
type siteData struct {
	Topology *server.Topology              `json:"topology"`
	Hosts    map[string]*server.HostDetail `json:"hosts"` // 에이전트 ID -> 서버 상세
	Install  map[string]any                `json:"install"`
}

func main() {
	out := flag.String("out", filepath.Join("docs", "index.html"), "만들 파일")
	flag.Parse()
	page, err := build()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, page, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("%s 를 만들었습니다 (%d KB)", *out, len(page)/1024)
}

func build() ([]byte, error) {
	data, err := collect()
	if err != nil {
		return nil, fmt.Errorf("데모 데이터: %w", err)
	}
	return assemble(data)
}

// collect 는 임시 폴더에 수집 서버를 띄우고, 가상 서버들을 등록·보고시킨 뒤 관제 화면이 받는 응답을 모은다.
func collect() (*siteData, error) {
	dir, err := os.MkdirTemp("", "infra-demo-site-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	const password, enrollToken = "demo-site-admin-password", "demo-site-enroll-token-not-a-secret"
	now := epoch.AddDate(0, 0, -45) // 등록은 45일 전에 한 것으로 한다
	srv, err := server.New(server.Config{
		DataDir: dir, DownloadsDir: filepath.Join(dir, "downloads"),
		AdminPassword: password, EnrollToken: enrollToken, TLSMode: "off", RetentionDays: 30,
		Clock: func() time.Time { return now },
	}, web.Static(), log.New(io.Discard, "", 0))
	if err != nil {
		return nil, err
	}
	defer srv.Close()
	handler := srv.Handler()

	call := func(method, path, remoteIP string, prepare func(*http.Request), in any) (*httptest.ResponseRecorder, error) {
		var body io.Reader
		if in != nil {
			b, err := json.Marshal(in)
			if err != nil {
				return nil, err
			}
			body = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, path, body)
		req.RemoteAddr = remoteIP + ":40000"
		if prepare != nil {
			prepare(req)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code >= 300 {
			return nil, fmt.Errorf("%s %s: %d %s", method, path, rec.Code, strings.TrimSpace(rec.Body.String()))
		}
		return rec, nil
	}

	// 1. 등록
	hosts := demo.Hosts()
	bearer := map[string]string{}  // 호스트명 -> Authorization 값
	agentID := map[string]string{} // 호스트명 -> 서버가 준 에이전트 ID
	for _, h := range hosts {
		rec, err := call("POST", "/api/v1/agent/enroll", h.IP, nil, model.EnrollRequest{Token: enrollToken, MachineID: "demo:" + h.Name, Hostname: h.Name})
		if err != nil {
			return nil, err
		}
		var res model.EnrollResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			return nil, err
		}
		agentID[h.Name] = res.AgentID
		bearer[h.Name] = "Bearer " + res.AgentID + "." + res.AgentToken
	}

	// 2. 보고. 처음에는 주소만 알리고, 그다음부터 흐름을 보낸다 (cmd/demo-seed 와 같은 순서).
	//    "응답 없음"을 보여 줄 서버는 처음에만 보고하고, 나머지는 6분 뒤에 다시 보고한다.
	report := func(withFlows, first bool) error {
		for _, h := range hosts {
			if h.StopsReporting && !first {
				continue
			}
			auth := bearer[h.Name]
			if _, err := call("POST", "/api/v1/agent/report", h.IP, func(r *http.Request) { r.Header.Set("Authorization", auth) }, h.Report(now, withFlows)); err != nil {
				return fmt.Errorf("%s: %w", h.Name, err)
			}
		}
		return nil
	}
	now = epoch.Add(-6 * time.Minute)
	if err := report(false, true); err != nil {
		return nil, err
	}
	now = now.Add(demo.IntervalSec * time.Second)
	if err := report(true, true); err != nil {
		return nil, err
	}
	now = epoch
	if err := report(true, false); err != nil {
		return nil, err
	}

	// 3. 관제 화면이 받는 응답
	const adminIP = "10.20.5.31"
	csrf := func(r *http.Request) { r.Header.Set("X-Requested-With", "infra-scanner") }
	rec, err := call("POST", "/api/v1/login", adminIP, csrf, map[string]string{"password": password})
	if err != nil {
		return nil, err
	}
	cookies := rec.Result().Cookies()
	get := func(path string, out any) error {
		rec, err := call("GET", path, adminIP, func(r *http.Request) {
			for _, c := range cookies {
				r.AddCookie(c)
			}
		}, nil)
		if err != nil {
			return err
		}
		// 무작위 ID 를 고정된 이름으로 바꾼 뒤에 읽는다. ID 는 노드·연결·점검 항목 곳곳에 들어 있다.
		text := rec.Body.String()
		for name, id := range agentID {
			text = strings.ReplaceAll(text, id, idPrefix+name)
		}
		return json.Unmarshal([]byte(text), out)
	}

	data := &siteData{
		Hosts: map[string]*server.HostDetail{},
		Install: map[string]any{
			// 실제 파일이 아니므로 해시와 지문은 0 으로 채운다.
			"downloads": []map[string]any{
				{"name": "infra-agent-linux-amd64", "size": 7536640, "sha256": strings.Repeat("0", 64)},
				{"name": "infra-agent-linux-arm64", "size": 6946816, "sha256": strings.Repeat("0", 64)},
				{"name": "infra-agent-windows-amd64.exe", "size": 7681024, "sha256": strings.Repeat("0", 64)},
			},
			"cert_sha256": strings.Repeat("0", 64),
			"tls_mode":    "selfsigned",
			"server_url":  exampleURL,
		},
	}
	if err := get("/api/v1/topology?window=24h", &data.Topology); err != nil {
		return nil, err
	}
	normalize(data.Topology)
	for _, h := range hosts {
		var d server.HostDetail
		if err := get("/api/v1/hosts/"+agentID[h.Name]+"?window=24h", &d); err != nil {
			return nil, err
		}
		d.Findings = []server.Finding{}
		for _, f := range data.Topology.Findings {
			if f.NodeID == d.NodeID {
				d.Findings = append(d.Findings, f)
			}
		}
		data.Hosts[idPrefix+h.Name] = &d
	}
	return data, nil
}

// normalize 는 ID 순으로 정렬된 목록을 바꿔 넣은 ID 기준으로 다시 정렬한다.
// 서버가 쓰는 순서(연결: ID, 점검 항목: 등급 · 코드 · 대상)와 같다.
func normalize(t *server.Topology) {
	sort.Slice(t.Edges, func(i, j int) bool { return t.Edges[i].ID < t.Edges[j].ID })
	rank := map[string]int{"high": 0, "medium": 1, "low": 2}
	subject := func(f server.Finding) string {
		target := f.NodeID
		if f.EdgeID != "" {
			target = f.EdgeID
		}
		args, _ := json.Marshal(f.Args) // 같은 대상의 같은 코드(디스크 여러 개 등)를 가른다
		return target + " " + string(args)
	}
	sort.SliceStable(t.Findings, func(i, j int) bool {
		a, b := t.Findings[i], t.Findings[j]
		if rank[a.Severity] != rank[b.Severity] {
			return rank[a.Severity] < rank[b.Severity]
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return subject(a) < subject(b)
	})
}

// assemble 은 web/static/index.html 에 스타일, 번역, 데이터, 스크립트를 모두 넣어 파일 하나로 만든다.
func assemble(data *siteData) ([]byte, error) {
	static := web.Static()
	read := func(name string) (string, error) {
		b, err := fs.ReadFile(static, name)
		return strings.ReplaceAll(string(b), "\r\n", "\n"), err
	}
	var html, css, i18n, app string
	for name, dst := range map[string]*string{"index.html": &html, "app.css": &css, "i18n.js": &i18n, "app.js": &app} {
		text, err := read(name)
		if err != nil {
			return nil, err
		}
		*dst = text
	}
	shim := strings.ReplaceAll(demoJS, "\r\n", "\n")

	// <script> 안에 넣은 글이 태그를 일찍 닫거나 HTML 주석으로 읽히지 않게 한다.
	script := func(name, js string) (string, error) {
		if strings.Contains(js, "<!--") {
			return "", fmt.Errorf("%s 에 <!-- 가 있어 페이지에 넣을 수 없습니다", name)
		}
		return "<script>\n" + strings.ReplaceAll(js, "</script", `<\/script`) + "\n</script>", nil
	}
	if strings.Contains(strings.ToLower(css), "</style") {
		return nil, fmt.Errorf("app.css 에 </style 이 있어 페이지에 넣을 수 없습니다")
	}
	payload, err := json.Marshal(data) // <, >, & 는 \u003c 처럼 바뀌어 나오므로 태그를 닫을 수 없다
	if err != nil {
		return nil, err
	}
	var scripts [3]string
	for i, s := range []struct{ name, js string }{{"i18n.js", i18n}, {"demo.js", shim}, {"app.js", app}} {
		if scripts[i], err = script(s.name, s.js); err != nil {
			return nil, err
		}
	}

	replacements := []struct{ old, new string }{
		{"<!DOCTYPE html>\n", "<!DOCTYPE html>\n<!--\n  Infra Scanner static demo. Generated by \"go run ./cmd/demo-site\"; do not edit by hand.\n" +
			"  The console is the code in web/static, unchanged. The data is fictional (internal/demo).\n-->\n"},
		{`<meta name="robots" content="noindex">`, `<meta name="description" content="Infra Scanner demo: a data-center topology drawn automatically from what agents on the servers observe. All servers and addresses are fictional.">`},
		{"<title>Infra Scanner</title>", "<title>Infra Scanner · Demo</title>"},
		{`<link rel="stylesheet" href="app.css">`, "<style>\n" + css + "\n</style>"},
		{`<script src="i18n.js" defer></script>`, scripts[0] + "\n  " + dataOpen + string(payload) + "</script>\n  " + scripts[1]},
		{`<script src="app.js" defer></script>`, scripts[2]},
	}
	for _, r := range replacements {
		if strings.Count(html, r.old) != 1 {
			return nil, fmt.Errorf("web/static/index.html 에 %q 가 정확히 한 번 있어야 합니다", r.old)
		}
		html = strings.Replace(html, r.old, r.new, 1)
	}
	return []byte(html), nil
}
