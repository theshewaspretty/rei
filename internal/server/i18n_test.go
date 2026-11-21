package server

import (
	"encoding/json"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"

	"infrascanner/web"
)

// 관제 화면의 번역(web/static/i18n.js)이 빠짐없이 맞물려 있는지 확인한다.
// 언어를 더하거나 문장을 고칠 때 한쪽 언어만 빠뜨리는 실수를 여기서 잡는다.

var uiLanguages = []string{"ko", "en", "zh"}

// 관제 화면이 받을 수 있는 서버 오류 코드 (writeError 의 code).
var adminErrorCodes = []string{
	"login_required", "forbidden", "too_many_logins", "bad_request", "bad_password",
	"topology_failed", "host_not_found", "host_read_failed", "delete_failed",
	"layout_save_failed", "layout_reset_failed",
}

// app.js 가 문자열을 이어 붙여 만드는 키. 정규식으로는 찾을 수 없어 따로 적는다.
var dynamicKeys = []string{
	"status.online", "status.offline", "status.pending", "status.unmanaged", "status.external",
	"sev.high", "sev.medium", "sev.low",
	"window.1h", "window.6h", "window.24h", "window.7d", "window.30d",
	"legend.sky", "legend.purple", "legend.amber", "legend.rose", "legend.emerald", "legend.slate",
	"zone.pending", "zone.none", "zone.unmanaged",
	"node.internet.title", "node.internet.sub", "node.clients.title", "node.clients.sub",
	"node.internet.tag", "node.clients.tag",
	"role.server", "role.db", "role.queue", "role.web",
	"about.unmanaged", "about.internet", "about.clients",
	"install.win", "install.root", "install.tls.selfsigned", "install.tls.off", "install.tls.files",
	"th.peer", "th.peerProc", "side.noResults", "side.noHosts",
	"warn.connections", "warn.host_info", "warn.memory",
	"f.unmanaged_host.detail_svc",
}

func readStatic(t *testing.T, name string) string {
	t.Helper()
	b, err := fs.ReadFile(web.Static(), name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return string(b)
}

func loadDict(t *testing.T) map[string]map[string]string {
	t.Helper()
	src := readStatic(t, "i18n.js")
	const marker = "window.INFRA_I18N = "
	start := strings.Index(src, marker)
	end := strings.LastIndex(src, "}")
	if start < 0 || end < start {
		t.Fatal("i18n.js 에서 window.INFRA_I18N 을 찾을 수 없습니다")
	}
	var dict map[string]map[string]string
	if err := json.Unmarshal([]byte(src[start+len(marker):end+1]), &dict); err != nil {
		t.Fatalf("i18n.js 의 사전이 JSON 형식이 아닙니다: %v", err)
	}
	return dict
}

var placeholderRe = regexp.MustCompile(`\{(\w+)\}`)

func placeholders(s string) string {
	set := map[string]bool{}
	for _, m := range placeholderRe.FindAllStringSubmatch(s, -1) {
		set[m[1]] = true
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func TestI18nLanguagesMatch(t *testing.T) {
	dict := loadDict(t)
	en := dict["en"]
	if len(en) == 0 {
		t.Fatal("영어 사전이 없습니다")
	}
	if len(dict) != len(uiLanguages) {
		t.Errorf("언어 수 = %d, %d 이어야 합니다", len(dict), len(uiLanguages))
	}
	for key := range en {
		if base, ok := strings.CutSuffix(key, "_one"); ok {
			if _, exists := en[base]; !exists {
				t.Errorf("en: 단수형 %q 의 기본 키 %q 가 없습니다", key, base)
			}
		}
	}
	for _, lang := range uiLanguages {
		d := dict[lang]
		if d == nil {
			t.Errorf("%s 사전이 없습니다", lang)
			continue
		}
		for key, text := range en {
			if strings.HasSuffix(key, "_one") {
				if got := placeholders(text); got != placeholders(en[strings.TrimSuffix(key, "_one")]) {
					t.Errorf("en: %q 의 자리표시자 {%s} 가 기본 키와 다릅니다", key, got)
				}
				continue // 단수형은 영어에만 있다
			}
			other, ok := d[key]
			if !ok {
				t.Errorf("%s: 키 %q 가 없습니다", lang, key)
				continue
			}
			if strings.TrimSpace(other) == "" {
				t.Errorf("%s: 키 %q 가 비어 있습니다", lang, key)
			}
			if want, got := placeholders(text), placeholders(other); want != got {
				t.Errorf("%s: %q 의 자리표시자 {%s}, 영어는 {%s}", lang, key, got, want)
			}
		}
		for key := range d {
			if _, ok := en[key]; !ok {
				t.Errorf("%s: 영어에 없는 키 %q", lang, key)
			}
		}
	}
}

func TestI18nCoversServerCodes(t *testing.T) {
	en := loadDict(t)["en"]
	for _, code := range findingCodes {
		for _, part := range []string{"title", "detail"} {
			if _, ok := en["f."+code+"."+part]; !ok {
				t.Errorf("점검 코드 %q 의 %s 문장이 없습니다", code, part)
			}
		}
	}
	for _, code := range adminErrorCodes {
		if _, ok := en["err."+code]; !ok {
			t.Errorf("오류 코드 %q 의 문장이 없습니다", code)
		}
	}
	for _, r := range hostRoles {
		if _, ok := en["role."+r]; !ok {
			t.Errorf("서버 역할 %q 의 문장이 없습니다", r)
		}
	}
	// 서버가 기본값으로 내려주는 영어 이름과 화면의 영어 문장이 어긋나지 않게 한다.
	if got := en["role."+roleServer]; got != defaultRole.Label {
		t.Errorf("role.%s = %q, 서버 기본값은 %q", roleServer, got, defaultRole.Label)
	}
	for cat, r := range roles {
		if got := en["role."+cat]; got != r.Label {
			t.Errorf("role.%s = %q, 서버 기본값은 %q", cat, got, r.Label)
		}
	}
}

func TestI18nKeysUsedByUIExist(t *testing.T) {
	en := loadDict(t)["en"]
	used := map[string]string{}
	for _, m := range regexp.MustCompile(`\b[tT]\('([\w.]+)'\s*[,)]`).FindAllStringSubmatch(readStatic(t, "app.js"), -1) {
		used[m[1]] = "app.js"
	}
	for _, m := range regexp.MustCompile(`data-i18n(?:-title|-placeholder)?="([\w.]+)"`).FindAllStringSubmatch(readStatic(t, "index.html"), -1) {
		used[m[1]] = "index.html"
	}
	for _, k := range dynamicKeys {
		used[k] = "app.js (조합 키)"
	}
	if len(used) < 100 {
		t.Fatalf("화면에서 쓰는 키를 %d개밖에 찾지 못했습니다. 찾는 정규식을 확인하세요", len(used))
	}
	for key, where := range used {
		if _, ok := en[key]; !ok {
			t.Errorf("%s 가 쓰는 키 %q 가 사전에 없습니다", where, key)
		}
	}
	// 반대로, 사전에만 있고 어디서도 쓰지 않는 키는 지워야 할 죽은 문장이다.
	for key := range en {
		base := strings.TrimSuffix(key, "_one")
		if _, ok := used[base]; ok {
			continue
		}
		if strings.HasPrefix(base, "f.") || strings.HasPrefix(base, "err.") {
			continue // 서버 코드에서 오는 키 (TestI18nCoversServerCodes 가 확인)
		}
		t.Errorf("사전의 키 %q 를 화면에서 쓰지 않습니다", key)
	}
}

// 서버가 실제로 만든 점검 항목이 모두 등록된 코드이고, 화면이 문장을 만들 때 쓰는 인자를 담고 있는지 확인한다.
func TestFindingsCarryArgs(t *testing.T) {
	dict := loadDict(t)
	known := map[string]bool{}
	for _, c := range findingCodes {
		known[c] = true
	}
	// 화면(app.js 의 findingText)은 age_sec 로 {age} 를, internet_exposed 의 target 으로 {host} 를 만든다.
	has := func(f Finding, name string) bool {
		if _, ok := f.Args[name]; ok {
			return true
		}
		switch {
		case name == "age":
			_, ok := f.Args["age_sec"]
			return ok
		case name == "host" && f.Code == "internet_exposed":
			_, ok := f.Args["target"]
			return ok
		}
		return false
	}

	topo := sampleTopology(t)
	if len(topo.Findings) == 0 {
		t.Fatal("점검 항목이 하나도 만들어지지 않았습니다")
	}
	for _, f := range topo.Findings {
		if !known[f.Code] {
			t.Errorf("등록되지 않은 점검 코드 %q", f.Code)
			continue
		}
		for _, name := range strings.Split(placeholders(dict["en"]["f."+f.Code+".detail"]), ",") {
			if name == "" {
				continue
			}
			if !has(f, name) {
				t.Errorf("%s: 문장의 {%s} 에 넣을 인자가 없습니다 (args=%v)", f.Code, name, f.Args)
			}
		}
	}
}
