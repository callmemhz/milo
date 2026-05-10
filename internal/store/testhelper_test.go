package store

import (
	"database/sql"
	"testing"

	"github.com/callmemhz/milo/internal/store/sqlcgen"
	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	s := &Store{DB: db, Q: sqlcgen.New(db)}
	t.Cleanup(func() { s.Close() })
	return s
}
