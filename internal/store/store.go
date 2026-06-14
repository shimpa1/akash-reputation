// Package store is the PostgreSQL persistence layer for the reputation service.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrDuplicate is returned when a (author, deployment) feedback already exists.
var ErrDuplicate = errors.New("feedback already exists for this deployment")

// schema is applied idempotently at startup. A deployment is (owner, dseq); a
// lease is (owner, dseq, gseq, oseq, provider). One feedback per author per
// deployment is enforced by the unique index on (author_addr, owner, dseq).
const schema = `
CREATE TABLE IF NOT EXISTS leases (
    owner      TEXT        NOT NULL,
    dseq       TEXT        NOT NULL,
    gseq       TEXT        NOT NULL,
    oseq       TEXT        NOT NULL,
    provider   TEXT        NOT NULL,
    state      TEXT        NOT NULL,
    first_seen TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (owner, dseq, gseq, oseq, provider)
);
CREATE INDEX IF NOT EXISTS leases_owner_idx ON leases (owner);
CREATE INDEX IF NOT EXISTS leases_provider_idx ON leases (provider);

CREATE TABLE IF NOT EXISTS feedback (
    id           BIGSERIAL   PRIMARY KEY,
    role         TEXT        NOT NULL,
    author_addr  TEXT        NOT NULL,
    subject_addr TEXT        NOT NULL,
    provider     TEXT        NOT NULL,
    owner        TEXT        NOT NULL,
    dseq         TEXT        NOT NULL,
    gseq         TEXT        NOT NULL,
    oseq         TEXT        NOT NULL,
    score        SMALLINT    NOT NULL,
    comment      TEXT        NOT NULL DEFAULT '',
    issued_at    TIMESTAMPTZ NOT NULL,
    pubkey       TEXT        NOT NULL,
    signature    TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS feedback_author_deployment_uniq
    ON feedback (author_addr, owner, dseq);
CREATE INDEX IF NOT EXISTS feedback_subject_idx ON feedback (subject_addr);
`

// Store wraps a pgx connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// Open connects to the database, applies the schema and returns a Store.
func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases the pool.
func (s *Store) Close() { s.pool.Close() }

// Ping verifies database connectivity.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Lease is the on-chain lease identity the poller persists.
type Lease struct {
	Owner    string
	DSeq     string
	GSeq     string
	OSeq     string
	Provider string
	State    string
}

// UpsertLease records (or refreshes) a known lease.
func (s *Store) UpsertLease(ctx context.Context, l Lease) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO leases (owner, dseq, gseq, oseq, provider, state)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (owner, dseq, gseq, oseq, provider)
		DO UPDATE SET state = EXCLUDED.state, last_seen = now()
	`, l.Owner, l.DSeq, l.GSeq, l.OSeq, l.Provider, l.State)
	return err
}

// LeaseExists reports whether a lease with the exact identity is known. This is
// the anti-abuse check: feedback is only accepted between parties that leased.
func (s *Store) LeaseExists(ctx context.Context, owner, dseq, gseq, oseq, provider string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM leases
			WHERE owner=$1 AND dseq=$2 AND gseq=$3 AND oseq=$4 AND provider=$5
		)`, owner, dseq, gseq, oseq, provider).Scan(&exists)
	return exists, err
}

// ListLeases returns known leases, optionally filtered by provider and/or owner.
func (s *Store) ListLeases(ctx context.Context, provider, owner string) ([]Lease, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT owner, dseq, gseq, oseq, provider, state FROM leases
		WHERE ($1 = '' OR provider = $1) AND ($2 = '' OR owner = $2)
		ORDER BY last_seen DESC LIMIT 500`, provider, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Lease
	for rows.Next() {
		var l Lease
		if err := rows.Scan(&l.Owner, &l.DSeq, &l.GSeq, &l.OSeq, &l.Provider, &l.State); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// FeedbackRecord is a stored, signed reputation statement.
type FeedbackRecord struct {
	ID          int64     `json:"id"`
	Role        string    `json:"role"`
	AuthorAddr  string    `json:"author"`
	SubjectAddr string    `json:"subject"`
	Provider    string    `json:"provider"`
	Owner       string    `json:"deployer"`
	DSeq        string    `json:"dseq"`
	GSeq        string    `json:"gseq"`
	OSeq        string    `json:"oseq"`
	Score       int       `json:"score"`
	Comment     string    `json:"comment"`
	IssuedAt    time.Time `json:"issued_at"`
	PubKey      string    `json:"pubkey"`
	Signature   string    `json:"signature"`
	CreatedAt   time.Time `json:"created_at"`
}

// InsertFeedback stores a verified feedback record. It returns ErrDuplicate if
// the author has already rated this deployment.
func (s *Store) InsertFeedback(ctx context.Context, r FeedbackRecord) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO feedback
			(role, author_addr, subject_addr, provider, owner, dseq, gseq, oseq,
			 score, comment, issued_at, pubkey, signature)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id`,
		r.Role, r.AuthorAddr, r.SubjectAddr, r.Provider, r.Owner, r.DSeq, r.GSeq,
		r.OSeq, r.Score, r.Comment, r.IssuedAt, r.PubKey, r.Signature).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return 0, ErrDuplicate
		}
		return 0, err
	}
	return id, nil
}

// Reputation is the aggregate score for an address acting as the subject of feedback.
type Reputation struct {
	Address          string `json:"address"`
	Positive         int    `json:"positive"`
	Negative         int    `json:"negative"`
	Net              int    `json:"net"`
	DeploymentsRated int    `json:"deployments_rated"`
}

// Reputation aggregates all feedback where addr is the subject (the rated party).
func (s *Store) Reputation(ctx context.Context, addr string) (Reputation, error) {
	rep := Reputation{Address: addr}
	err := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN score > 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN score < 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(score), 0),
			COUNT(DISTINCT (owner, dseq))
		FROM feedback WHERE subject_addr = $1`, addr).
		Scan(&rep.Positive, &rep.Negative, &rep.Net, &rep.DeploymentsRated)
	return rep, err
}

// ListFeedback returns raw feedback rows, optionally filtered. Each row carries
// its signature/pubkey so a caller can independently re-verify it.
func (s *Store) ListFeedback(ctx context.Context, subject, author, role string) ([]FeedbackRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, role, author_addr, subject_addr, provider, owner, dseq, gseq, oseq,
		       score, comment, issued_at, pubkey, signature, created_at
		FROM feedback
		WHERE ($1 = '' OR subject_addr = $1)
		  AND ($2 = '' OR author_addr = $2)
		  AND ($3 = '' OR role = $3)
		ORDER BY created_at DESC LIMIT 500`, subject, author, role)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FeedbackRecord
	for rows.Next() {
		var r FeedbackRecord
		if err := rows.Scan(&r.ID, &r.Role, &r.AuthorAddr, &r.SubjectAddr, &r.Provider,
			&r.Owner, &r.DSeq, &r.GSeq, &r.OSeq, &r.Score, &r.Comment, &r.IssuedAt,
			&r.PubKey, &r.Signature, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
