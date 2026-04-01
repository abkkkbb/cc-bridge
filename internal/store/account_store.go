package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"cc2api/internal/model"
)

// jsonScanner scans a JSON column. SQLite returns string, PostgreSQL returns []byte.
type jsonScanner struct {
	target *json.RawMessage
}

func (s *jsonScanner) Scan(src any) error {
	if src == nil {
		*s.target = json.RawMessage("{}")
		return nil
	}
	switch v := src.(type) {
	case []byte: // PostgreSQL
		*s.target = json.RawMessage(v)
	case string: // SQLite
		*s.target = json.RawMessage(v)
	default:
		return fmt.Errorf("unsupported json type: %T", src)
	}
	return nil
}

type AccountStore struct {
	db     *sql.DB
	driver string
}

func NewAccountStore(db *sql.DB, driver string) *AccountStore {
	return &AccountStore{db: db, driver: driver}
}

func (s *AccountStore) now() string {
	if s.driver == "sqlite" {
		// strftime outputs RFC3339 (with Z suffix) to match our timeVal format
		return "strftime('%Y-%m-%dT%H:%M:%SZ','now')"
	}
	return "NOW()"
}

// timeVal wraps time.Time for SQLite: writes as RFC3339 string, reads as RFC3339 string.
// For PostgreSQL the pq driver handles time.Time natively, so this type is only used with SQLite.
type timeVal time.Time

func (t timeVal) Value() (driver.Value, error) {
	return time.Time(t).Format(time.RFC3339), nil
}

// timeScanner scans a time column. SQLite: parses RFC3339 string. PostgreSQL: native time.Time.
type timeScanner struct {
	target *time.Time
}

func (s *timeScanner) Scan(src any) error {
	if src == nil {
		return nil
	}
	switch v := src.(type) {
	case time.Time: // PostgreSQL
		*s.target = v
		return nil
	case string: // SQLite — we always write RFC3339
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return fmt.Errorf("time parse %q: %w", v, err)
		}
		*s.target = t
		return nil
	default:
		return fmt.Errorf("unsupported time type: %T", src)
	}
}

// nullTimeScanner scans a nullable time column.
type nullTimeScanner struct {
	target **time.Time
}

func (s *nullTimeScanner) Scan(src any) error {
	if src == nil {
		*s.target = nil
		return nil
	}
	t := new(time.Time)
	ts := &timeScanner{target: t}
	if err := ts.Scan(src); err != nil {
		return err
	}
	*s.target = t
	return nil
}

// fmtTime converts time.Time to the appropriate SQL parameter.
// SQLite: RFC3339 string via timeVal. PostgreSQL: native time.Time.
func (s *AccountStore) fmtTime(t time.Time) any {
	if s.driver == "sqlite" {
		return timeVal(t)
	}
	return t
}

func (s *AccountStore) Create(ctx context.Context, a *model.Account) error {
	var count int
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts WHERE email=$1`, a.Email).Scan(&count)
	if count > 0 {
		return fmt.Errorf("邮箱 %s 已存在", a.Email)
	}

	var ca, ua timeScanner
	ca.target = &a.CreatedAt
	ua.target = &a.UpdatedAt
	return s.db.QueryRowContext(ctx, `
		INSERT INTO accounts (name, email, status, token, proxy_url,
			device_id, canonical_env, canonical_prompt_env, canonical_process,
			concurrency, priority)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, created_at, updated_at`,
		a.Name, a.Email, a.Status, a.Token, a.ProxyURL,
		a.DeviceID, a.CanonicalEnv, a.CanonicalPrompt, a.CanonicalProcess,
		a.Concurrency, a.Priority,
	).Scan(&a.ID, &ca, &ua)
}

func (s *AccountStore) Update(ctx context.Context, a *model.Account) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE accounts SET name=$1, email=$2, status=$3, token=$4,
			proxy_url=$5, concurrency=$6, priority=$7, updated_at=`+s.now()+`
		WHERE id=$8`,
		a.Name, a.Email, a.Status, a.Token,
		a.ProxyURL, a.Concurrency, a.Priority, a.ID)
	return err
}

func (s *AccountStore) UpdateStatus(ctx context.Context, id int64, status model.AccountStatus) error {
	_, err := s.db.ExecContext(ctx, `UPDATE accounts SET status=$1, updated_at=`+s.now()+` WHERE id=$2`, status, id)
	return err
}

func (s *AccountStore) SetRateLimit(ctx context.Context, id int64, resetAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE accounts SET rate_limited_at=$1, rate_limit_reset_at=$2, updated_at=`+s.now()+` WHERE id=$3`,
		s.fmtTime(time.Now()), s.fmtTime(resetAt), id)
	return err
}

func (s *AccountStore) ClearRateLimit(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE accounts SET rate_limited_at=NULL, rate_limit_reset_at=NULL, updated_at=`+s.now()+` WHERE id=$1`, id)
	return err
}

func (s *AccountStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM accounts WHERE id=$1`, id)
	return err
}

func (s *AccountStore) GetByID(ctx context.Context, id int64) (*model.Account, error) {
	a := &model.Account{}
	err := s.db.QueryRowContext(ctx, `SELECT `+accountCols+` FROM accounts WHERE id=$1`, id).Scan(accountDest(a)...)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *AccountStore) List(ctx context.Context) ([]*model.Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+accountCols+` FROM accounts ORDER BY priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []*model.Account
	for rows.Next() {
		a := &model.Account{}
		if err := rows.Scan(accountDest(a)...); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (s *AccountStore) ListSchedulable(ctx context.Context) ([]*model.Account, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+accountCols+` FROM accounts
		WHERE status='active'
		  AND (rate_limit_reset_at IS NULL OR rate_limit_reset_at < `+s.now()+`)
		ORDER BY priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []*model.Account
	for rows.Next() {
		a := &model.Account{}
		if err := rows.Scan(accountDest(a)...); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

const accountCols = `id,name,email,status,token,proxy_url,device_id,
	canonical_env,canonical_prompt_env,canonical_process,
	concurrency,priority,rate_limited_at,rate_limit_reset_at,created_at,updated_at`

func accountDest(a *model.Account) []any {
	return []any{
		&a.ID, &a.Name, &a.Email, &a.Status, &a.Token, &a.ProxyURL,
		&a.DeviceID, &jsonScanner{&a.CanonicalEnv}, &jsonScanner{&a.CanonicalPrompt}, &jsonScanner{&a.CanonicalProcess},
		&a.Concurrency, &a.Priority,
		&nullTimeScanner{&a.RateLimitedAt}, &nullTimeScanner{&a.RateLimitResetAt},
		&timeScanner{&a.CreatedAt}, &timeScanner{&a.UpdatedAt},
	}
}
