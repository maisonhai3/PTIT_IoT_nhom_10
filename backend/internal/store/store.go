// Package store persists telemetry history and the event log in SQLite (pure Go driver, no cgo).
package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/model"
)

type Store interface {
	InsertTelemetry(ctx context.Context, t model.Telemetry) error
	// History returns rows with ts >= since in ascending order, evenly thinned to at most maxPoints.
	History(ctx context.Context, since time.Time, maxPoints int) ([]model.HistoryPoint, error)
	InsertEvent(ctx context.Context, e model.Event) error
	// Events returns the most recent events, newest first.
	Events(ctx context.Context, limit int) ([]model.Event, error)
	Prune(ctx context.Context, telemetryBefore, eventsBefore time.Time) error
	Close() error
}

type SQLite struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS telemetry (
	ts          INTEGER NOT NULL,           -- unix milliseconds
	temp        REAL,
	humidity    REAL,
	light       INTEGER NOT NULL,
	state       TEXT NOT NULL,
	mode        TEXT NOT NULL,
	rain        INTEGER NOT NULL,
	rain_source TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_telemetry_ts ON telemetry(ts);
CREATE TABLE IF NOT EXISTS events (
	id     INTEGER PRIMARY KEY AUTOINCREMENT,
	ts     INTEGER NOT NULL,
	kind   TEXT NOT NULL,
	detail TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_ts ON events(ts);
`

// Open opens (creating if needed) the database at path. Use ":memory:" for tests.
func Open(path string) (*SQLite, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	if path == ":memory:" {
		dsn = "file::memory:?_pragma=busy_timeout(5000)"
	} else {
		dsn += "&_pragma=journal_mode(WAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection: writes are tiny and infrequent, and it keeps :memory: databases consistent.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &SQLite{db: db}, nil
}

func (s *SQLite) Close() error { return s.db.Close() }

func nullFloat(p *float64) sql.NullFloat64 {
	if p == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *p, Valid: true}
}

func floatPtr(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}

func (s *SQLite) InsertTelemetry(ctx context.Context, t model.Telemetry) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO telemetry(ts,temp,humidity,light,state,mode,rain,rain_source) VALUES(?,?,?,?,?,?,?,?)`,
		t.TS.UnixMilli(), nullFloat(t.Temp), nullFloat(t.Humidity), t.Light,
		string(t.State), string(t.Mode), t.Rain, string(t.RainSource))
	return err
}

func (s *SQLite) History(ctx context.Context, since time.Time, maxPoints int) ([]model.HistoryPoint, error) {
	if maxPoints < 1 {
		maxPoints = 1
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM telemetry WHERE ts >= ?`, since.UnixMilli()).Scan(&n); err != nil {
		return nil, err
	}
	stride := 1
	if n > maxPoints {
		stride = (n + maxPoints - 1) / maxPoints
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT ts,temp,humidity,light,state,mode,rain,rain_source FROM (
			SELECT *, ROW_NUMBER() OVER (ORDER BY ts) AS rn FROM telemetry WHERE ts >= ?
		) WHERE (rn-1) % ? = 0 ORDER BY ts`, since.UnixMilli(), stride)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.HistoryPoint, 0, min(n, maxPoints))
	for rows.Next() {
		var (
			ms         int64
			temp, hum  sql.NullFloat64
			p          model.HistoryPoint
			state, mod string
			src        string
		)
		if err := rows.Scan(&ms, &temp, &hum, &p.Light, &state, &mod, &p.Rain, &src); err != nil {
			return nil, err
		}
		p.TS = time.UnixMilli(ms).UTC()
		p.Temp, p.Humidity = floatPtr(temp), floatPtr(hum)
		p.State, p.Mode, p.RainSource = model.AwningState(state), model.Mode(mod), model.RainSource(src)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *SQLite) InsertEvent(ctx context.Context, e model.Event) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO events(ts,kind,detail) VALUES(?,?,?)`,
		e.TS.UnixMilli(), string(e.Kind), e.Detail)
	return err
}

func (s *SQLite) Events(ctx context.Context, limit int) ([]model.Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ts,kind,detail FROM events ORDER BY ts DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Event{}
	for rows.Next() {
		var ms int64
		var kind, detail string
		if err := rows.Scan(&ms, &kind, &detail); err != nil {
			return nil, err
		}
		out = append(out, model.Event{TS: time.UnixMilli(ms).UTC(), Kind: model.EventKind(kind), Detail: detail})
	}
	return out, rows.Err()
}

func (s *SQLite) Prune(ctx context.Context, telemetryBefore, eventsBefore time.Time) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM telemetry WHERE ts < ?`, telemetryBefore.UnixMilli()); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE ts < ?`, eventsBefore.UnixMilli())
	return err
}

// Memory is a convenience for tests in other packages.
func Memory() (*SQLite, error) { return Open(strings.TrimSpace(":memory:")) }
