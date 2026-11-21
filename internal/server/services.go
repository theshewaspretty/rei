package server

import (
	"fmt"
	"net/netip"
)

// 서비스 분류. 구성도에서 연결선 색과 서버 역할을 정하는 데 쓴다.
const (
	catWeb   = "web"
	catDB    = "db"
	catQueue = "queue"
	catMgmt  = "mgmt"
	catInfra = "infra"
	catOther = "other"
)

type svcInfo struct {
	Name  string
	Cat   string
	Plain bool // 포트 기준으로 암호화되지 않는 프로토콜일 가능성이 높다
}

// 포트 번호만으로 추정한 서비스다. 실제 프로토콜은 패킷을 보지 않으므로 알 수 없다.
var wellKnown = map[int]svcInfo{
	21:    {"FTP", catMgmt, true},
	22:    {"SSH", catMgmt, false},
	23:    {"Telnet", catMgmt, true},
	25:    {"SMTP", catInfra, true},
	53:    {"DNS", catInfra, false},
	80:    {"HTTP", catWeb, true},
	88:    {"Kerberos", catInfra, false},
	110:   {"POP3", catInfra, true},
	123:   {"NTP", catInfra, false},
	135:   {"MS-RPC", catInfra, false},
	139:   {"NetBIOS", catInfra, false},
	143:   {"IMAP", catInfra, true},
	161:   {"SNMP", catInfra, false},
	389:   {"LDAP", catInfra, true},
	443:   {"HTTPS", catWeb, false},
	445:   {"SMB", catInfra, false},
	514:   {"Syslog", catInfra, false},
	636:   {"LDAPS", catInfra, false},
	1433:  {"MSSQL", catDB, false},
	1521:  {"Oracle", catDB, false},
	2049:  {"NFS", catInfra, false},
	2181:  {"ZooKeeper", catQueue, false},
	2379:  {"etcd", catDB, false},
	3260:  {"iSCSI", catInfra, false},
	3306:  {"MySQL", catDB, false},
	3389:  {"RDP", catMgmt, false},
	5432:  {"PostgreSQL", catDB, false},
	5672:  {"AMQP", catQueue, false},
	5900:  {"VNC", catMgmt, false},
	5985:  {"WinRM", catMgmt, true},
	5986:  {"WinRM-TLS", catMgmt, false},
	6379:  {"Redis", catQueue, false},
	6443:  {"Kubernetes API", catInfra, false},
	8000:  {"HTTP", catWeb, true},
	8080:  {"HTTP", catWeb, true},
	8443:  {"HTTPS", catWeb, false},
	9092:  {"Kafka", catQueue, false},
	9090:  {"Prometheus", catInfra, false},
	9100:  {"node-exporter", catInfra, false},
	9200:  {"Elasticsearch", catDB, false},
	10250: {"kubelet", catInfra, false},
	11211: {"Memcached", catQueue, false},
	27017: {"MongoDB", catDB, false},
}

func lookupService(port int) svcInfo {
	if s, ok := wellKnown[port]; ok {
		return s
	}
	return svcInfo{Name: "TCP", Cat: catOther}
}

// serviceLabel 은 "PostgreSQL :5432" 같은 표시용 문자열이다.
func serviceLabel(port int) string {
	return fmt.Sprintf("%s :%d", lookupService(port).Name, port)
}

// 분류별 연결선 색. test.html 의 ACCENT_PALETTE 키와 같다.
var catColor = map[string]string{
	catWeb: "sky", catDB: "purple", catQueue: "amber", catMgmt: "rose", catInfra: "emerald", catOther: "slate",
}

type role struct {
	Label  string
	Icon   string
	Accent string
}

// 여러 분류의 포트를 함께 열고 있으면 앞에 있는 분류가 서버의 역할이 된다.
var rolePriority = []string{catDB, catQueue, catWeb}

var roles = map[string]role{
	catDB:    {"Database Server", "database", "purple"},
	catQueue: {"Queue / Cache Server", "layers", "amber"},
	catWeb:   {"Web / API Server", "globe", "sky"},
}

var defaultRole = role{"Server", "server", "slate"}

// roleServer 는 역할을 추정하지 못한 서버의 role 값이다. 추정한 서버는 분류 이름(db, queue, web)을 쓴다.
const roleServer = "server"

// hostRoles 는 TopoNode.Role 에 올 수 있는 값이다. 화면의 번역 키(role.<값>)와 맞는지 시험에서 확인한다.
var hostRoles = append([]string{roleServer}, rolePriority...)

// CGNAT 대역(100.64.0.0/10)도 내부망으로 본다.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

func isInternal(a netip.Addr) bool {
	return a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || cgnat.Contains(a)
}
