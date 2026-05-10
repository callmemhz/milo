package store

import (
	"context"
	"database/sql"

	_ "modernc.org/sqlite"

	"github.com/callmemhz/milo/internal/store/sqlcgen"
)

type Store struct {
	DB *sql.DB
	Q  *sqlcgen.Queries
}

func Open(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	if err := RunMigrations(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{DB: db, Q: sqlcgen.New(db)}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) WithTx(ctx context.Context, fn func(*sqlcgen.Queries) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(s.Q.WithTx(tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
