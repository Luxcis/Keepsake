package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// 快照必须包含全部表与数据，并且可以脱离原库/WAL 单独打开。
func TestSnapshotToCreatesIndependentCopy(t *testing.T) {
	ctx := context.Background()
	src := filepath.Join(t.TempDir(), "journey.db")
	st, err := Open(ctx, src)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := st.SetSettings(ctx, map[string]string{"site_title": "拾光集"}); err != nil {
		t.Fatalf("set settings: %v", err)
	}

	dst := filepath.Join(t.TempDir(), "snapshot.db")
	if err := st.SnapshotTo(ctx, dst); err != nil {
		t.Fatalf("SnapshotTo: %v", err)
	}
	if info, err := os.Stat(dst); err != nil || info.Size() == 0 {
		t.Fatalf("snapshot missing or empty: info=%v err=%v", info, err)
	}

	// 只读打开快照：不产生 WAL/SHM，也不依赖原库。
	ro, err := sql.Open("sqlite", "file:"+dst+"?mode=ro")
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer func() { _ = ro.Close() }()

	var title string
	if err := ro.QueryRow(`SELECT value FROM site_settings WHERE key = 'site_title'`).Scan(&title); err != nil {
		t.Fatalf("query snapshot: %v", err)
	}
	if title != "拾光集" {
		t.Fatalf("site_title = %q", title)
	}

	var migrations int
	if err := ro.QueryRow(`SELECT COUNT(*) FROM _migrations`).Scan(&migrations); err != nil {
		t.Fatalf("query migrations: %v", err)
	}
	if migrations == 0 {
		t.Fatal("snapshot should include _migrations")
	}
}

func TestSnapshotToRejectsExistingDestination(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "journey.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	dst := filepath.Join(t.TempDir(), "snapshot.db")
	if err := os.WriteFile(dst, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("seed destination: %v", err)
	}
	if err := st.SnapshotTo(ctx, dst); err == nil {
		t.Fatal("expected error for existing destination")
	}
}
