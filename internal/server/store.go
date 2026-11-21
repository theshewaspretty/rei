package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // CGO 없이 동작하는 SQLite 드라이버

	"infrascanner/internal/model"
)

// AgentRecord 는 등록된 에이전트 1대다. 한 번 만든 레코드는 고치지 않고 새 값으로 바꿔 끼운다.
type AgentRecord struct {
	ID         string
	MachineID  string
	Hostname   string
	FirstSeen  time.Time
	LastSeen   time.Time // 보고를 받은 적이 없으면 zero
	RemoteAddr string
	Report     *model.Report // 마지막 보고(흐름 제외). 보고 전이면 nil
	tokenHash  []byte
}

// FlowRow 는 누적된 통신 흐름 1건이다.
type FlowRow struct {
	AgentID     string
	Direction   string
	Proto       string
	PeerIP      string // IP, 또는 묶음 표시인 peerInternet / peerClients
	Port        int
	Process     string
	PeerProcess string
	FirstSeen   time.Time
	LastSeen    time.Time
	MaxConns    int
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS agents (
  id          TEXT PRIMARY KEY,
  machine_id  TEXT NOT NULL UNIQUE,
  token_hash  BLOB NOT NULL,
  hostname    TEXT NOT NULL DEFAULT '',
  first_seen  INTEGER NOT NULL,
  last_seen   INTEGER NOT NULL DEFAULT 0,
  remote_addr TEXT NOT NULL DEFAULT '',
  report      TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS flows (
  agent_id     TEXT NOT NULL,
  direction    TEXT NOT NULL,
  proto        TEXT NOT NULL,
  peer_ip      TEXT NOT NULL,
  port         INTEGER NOT NULL,
  process      TEXT NOT NULL,
  peer_process TEXT NOT NULL DEFAULT '',
  first_seen   INTEGER NOT NULL,
  last_seen    INTEGER NOT NULL,
  max_conns    INTEGER NOT NULL,
  PRIMARY KEY (agent_id, direction, proto, peer_ip, port, process)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS flows_last_seen ON flows(last_seen);
CREATE TABLE IF NOT EXISTS layout (
  node_id TEXT PRIMARY KEY,
  x REAL NOT NULL,
  y REAL NOT NULL
);
`

func OpenStore(path string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // Windows: file:///C:/...
	}
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(NORMAL)")
	dsn := (&url.URL{Scheme: "file", Path: p, RawQuery: q.Encode()}).String()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func unixOrZero(sec int64) time.Time {
	if sec == 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}

func (s *Store) LoadAgents() ([]*AgentRecord, error) {
	rows, err := s.db.Query(`SELECT id, machine_id, token_hash, hostname, first_seen, last_seen, remote_addr, report FROM agents`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AgentRecord
	for rows.Next() {
		var (
			a           AgentRecord
			first, last int64
			report      string
		)
		if err := rows.Scan(&a.ID, &a.MachineID, &a.tokenHash, &a.Hostname, &first, &last, &a.RemoteAddr, &report); err != nil {
			return nil, err
		}
		a.FirstSeen, a.LastSeen = time.Unix(first, 0), unixOrZero(last)
		if report != "" {
			var r model.Report
			if err := json.Unmarshal([]byte(report), &r); err != nil {
				return nil, fmt.Errorf("agent %s: 저장된 보고를 읽을 수 없습니다: %w", a.ID, err)
			}
			a.Report = &r
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

// EnrollAgent 는 machine_id 가 처음이면 새 에이전트를 만들고, 이미 있으면 토큰만 바꾼다.
// 같은 장비를 다시 설치해도 구성도에서 같은 노드로 이어지게 하기 위해서다.
func (s *Store) EnrollAgent(newID, machineID, hostname string, tokenHash []byte, now time.Time) (*AgentRecord, error) {
	_, err := s.db.Exec(`
		INSERT INTO agents (id, machine_id, token_hash, hostname, first_seen) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(machine_id) DO UPDATE SET token_hash = excluded.token_hash, hostname = excluded.hostname`,
		newID, machineID, tokenHash, hostname, now.Unix())
	if err != nil {
		return nil, err
	}
	var (
		a           AgentRecord
		first, last int64
		report      string
	)
	err = s.db.QueryRow(`SELECT id, machine_id, token_hash, hostname, first_seen, last_seen, remote_addr, report FROM agents WHERE machine_id = ?`, machineID).
		Scan(&a.ID, &a.MachineID, &a.tokenHash, &a.Hostname, &first, &last, &a.RemoteAddr, &report)
	if err != nil {
		return nil, err
	}
	a.FirstSeen, a.LastSeen = time.Unix(first, 0), unixOrZero(last)
	if report != "" {
		var r model.Report
		if err := json.Unmarshal([]byte(report), &r); err == nil {
			a.Report = &r
		}
	}
	return &a, nil
}

// SaveReport 는 마지막 보고를 덮어쓰고 흐름을 누적한다. rep 에는 흐름이 들어 있지 않다.
func (s *Store) SaveReport(agentID string, rep *model.Report, flows []model.Flow, remoteAddr string, now time.Time) error {
	body, err := json.Marshal(rep)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`UPDATE agents SET hostname = ?, last_seen = ?, remote_addr = ?, report = ? WHERE id = ?`,
		rep.Host.Hostname, now.Unix(), remoteAddr, string(body), agentID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("등록되지 않은 에이전트입니다")
	}

	stmt, err := tx.Prepare(`
		INSERT INTO flows (agent_id, direction, proto, peer_ip, port, process, peer_process, first_seen, last_seen, max_conns)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_id, direction, proto, peer_ip, port, process) DO UPDATE SET
		  last_seen = excluded.last_seen,
		  peer_process = excluded.peer_process,
		  max_conns = max(max_conns, excluded.max_conns)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	ts := now.Unix()
	for _, f := range flows {
		if _, err := stmt.Exec(agentID, f.Direction, f.Proto, f.PeerIP, f.Port, f.Process, f.PeerProcess, ts, ts, f.Count); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const flowCols = `agent_id, direction, proto, peer_ip, port, process, peer_process, first_seen, last_seen, max_conns`

func scanFlows(rows *sql.Rows) ([]FlowRow, error) {
	defer rows.Close()
	var out []FlowRow
	for rows.Next() {
		var (
			f           FlowRow
			first, last int64
		)
		if err := rows.Scan(&f.AgentID, &f.Direction, &f.Proto, &f.PeerIP, &f.Port, &f.Process, &f.PeerProcess, &first, &last, &f.MaxConns); err != nil {
			return nil, err
		}
		f.FirstSeen, f.LastSeen = time.Unix(first, 0), time.Unix(last, 0)
		out = append(out, f)
	}
	return out, rows.Err()
}

// Flows 는 since 이후에 한 번이라도 관측된 서버 간 흐름이다. 서버 내부(local) 흐름은 빼고 준다.
func (s *Store) Flows(since time.Time) ([]FlowRow, error) {
	rows, err := s.db.Query(`SELECT `+flowCols+` FROM flows WHERE last_seen >= ? AND direction != ?`, since.Unix(), model.DirLocal)
	if err != nil {
		return nil, err
	}
	return scanFlows(rows)
}

func (s *Store) AgentFlows(agentID string, since time.Time) ([]FlowRow, error) {
	rows, err := s.db.Query(`SELECT `+flowCols+` FROM flows WHERE agent_id = ? AND last_seen >= ? ORDER BY direction, port, peer_ip, process`, agentID, since.Unix())
	if err != nil {
		return nil, err
	}
	return scanFlows(rows)
}

func (s *Store) DeleteAgent(agentID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM flows WHERE agent_id = ?`, `DELETE FROM agents WHERE id = ?`} {
		if _, err := tx.Exec(q, agentID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM layout WHERE node_id = ?`, "a:"+agentID); err != nil {
		return err
	}
	return tx.Commit()
}

// PruneFlows 는 before 이전에 마지막으로 관측된 흐름을 지운다.
func (s *Store) PruneFlows(before time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM flows WHERE last_seen < ?`, before.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) Layout() (map[string]Point, error) {
	rows, err := s.db.Query(`SELECT node_id, x, y FROM layout`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Point{}
	for rows.Next() {
		var (
			id string
			p  Point
		)
		if err := rows.Scan(&id, &p.X, &p.Y); err != nil {
			return nil, err
		}
		out[id] = p
	}
	return out, rows.Err()
}

func (s *Store) SaveLayout(points map[string]Point) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for id, p := range points {
		if _, err := tx.Exec(`INSERT INTO layout (node_id, x, y) VALUES (?, ?, ?) ON CONFLICT(node_id) DO UPDATE SET x = excluded.x, y = excluded.y`, id, p.X, p.Y); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ResetLayout() error {
	_, err := s.db.Exec(`DELETE FROM layout`)
	return err
}
