package backup

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"keepsake/internal/cosutil"
)

type fakeSource struct {
	mu       sync.Mutex
	objects  []cosutil.ObjectInfo
	contents map[string]string
	failKey  string
	listErr  error
	block    chan struct{}
}

func (f *fakeSource) ListObjects(context.Context) ([]cosutil.ObjectInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.objects, nil
}

func (f *fakeSource) OpenObject(_ context.Context, key string) (io.ReadCloser, error) {
	if f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	failKey := f.failKey
	f.mu.Unlock()
	if key == failKey {
		return nil, errors.New("read failed")
	}
	body, ok := f.contents[key]
	if !ok {
		return nil, cosutil.ErrObjectMissing
	}
	return io.NopCloser(strings.NewReader(body)), nil
}

func newService(t *testing.T, dir string, src cosutil.ObjectSource) *Service {
	t.Helper()
	svc, err := New(Options{
		Dir:      dir,
		DBName:   "journey.db",
		Env:      "dev",
		Driver:   "local",
		Snapshot: func(_ context.Context, dst string) error { return os.WriteFile(dst, []byte("sqlite-bytes"), 0o600) },
		Objects:  src,
		Now:      func() time.Time { return time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC) },
		Log:      slog.New(slog.NewJSONHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

func waitIdle(t *testing.T, svc *Service) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st, err := svc.Status()
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if st.Running == nil {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatal("backup did not finish in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func readZip(t *testing.T, p string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(p)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer func() { _ = zr.Close() }()
	out := make(map[string][]byte)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		out[f.Name] = b
	}
	return out
}

func TestCreateBackupContainsDatabaseFilesAndManifest(t *testing.T) {
	dir := t.TempDir()
	src := &fakeSource{
		objects: []cosutil.ObjectInfo{
			{Key: "trips/t1/a.jpg", Size: 4},
			{Key: "videos/t1/b.mp4", Size: 6},
		},
		contents: map[string]string{
			"trips/t1/a.jpg":  "jpeg",
			"videos/t1/b.mp4": "mpeg4!",
		},
	}
	svc := newService(t, dir, src)

	if started, err := svc.Start(); err != nil || started == "" {
		t.Fatalf("Start: name=%q err=%v", started, err)
	}
	st := waitIdle(t, svc)
	if st.Last == nil || st.Last.Status != "completed" {
		t.Fatalf("last = %+v", st.Last)
	}
	if len(st.Items) != 1 {
		t.Fatalf("items = %+v", st.Items)
	}
	item := st.Items[0]
	if !namePattern.MatchString(item.Name) {
		t.Fatalf("unexpected name %q", item.Name)
	}
	if item.SizeBytes <= 0 {
		t.Fatalf("size = %d", item.SizeBytes)
	}

	entries := readZip(t, filepath.Join(dir, item.Name))
	for _, want := range []string{"manifest.json", "journey.db", "uploads/trips/t1/a.jpg", "uploads/videos/t1/b.mp4"} {
		if _, ok := entries[want]; !ok {
			t.Fatalf("zip missing %s (got %v)", want, keys(entries))
		}
	}
	if string(entries["journey.db"]) != "sqlite-bytes" {
		t.Fatalf("db content = %q", entries["journey.db"])
	}

	var manifest Manifest
	if err := json.Unmarshal(entries["manifest.json"], &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.Schema != ManifestSchema || manifest.StorageDriver != "local" || manifest.AppEnv != "dev" {
		t.Fatalf("manifest meta = %+v", manifest)
	}
	if manifest.Database.File != "journey.db" || manifest.Database.Bytes != int64(len("sqlite-bytes")) {
		t.Fatalf("manifest database = %+v", manifest.Database)
	}
	if manifest.Files.Count != 2 || manifest.Files.Bytes != 10 || manifest.Files.Prefix != "uploads/" {
		t.Fatalf("manifest files = %+v", manifest.Files)
	}
	if len(manifest.Objects) != 2 || manifest.Objects[0].Key != "trips/t1/a.jpg" {
		t.Fatalf("manifest objects = %+v", manifest.Objects)
	}
	if len(manifest.Excludes) == 0 {
		t.Fatal("manifest should document excludes")
	}
}

func TestFailureLeavesNoPartialBackup(t *testing.T) {
	dir := t.TempDir()
	src := &fakeSource{
		objects:  []cosutil.ObjectInfo{{Key: "trips/t1/a.jpg", Size: 4}},
		contents: map[string]string{"trips/t1/a.jpg": "jpeg"},
		failKey:  "trips/t1/a.jpg",
	}
	svc := newService(t, dir, src)

	if _, err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	st := waitIdle(t, svc)
	if st.Last == nil || st.Last.Status != "failed" || st.Last.Error == "" {
		t.Fatalf("last = %+v", st.Last)
	}
	if len(st.Items) != 0 {
		t.Fatalf("failed backup must not appear in list: %+v", st.Items)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") || strings.HasSuffix(e.Name(), ".zip") {
			t.Fatalf("leftover file %q", e.Name())
		}
	}
}

func TestConcurrentStartRejected(t *testing.T) {
	dir := t.TempDir()
	block := make(chan struct{})
	src := &fakeSource{
		objects:  []cosutil.ObjectInfo{{Key: "a.jpg", Size: 1}},
		contents: map[string]string{"a.jpg": "x"},
		block:    block,
	}
	svc := newService(t, dir, src)

	if _, err := svc.Start(); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if _, err := svc.Start(); !errors.Is(err, ErrRunning) {
		t.Fatalf("second Start err = %v, want ErrRunning", err)
	}
	close(block)
	st := waitIdle(t, svc)
	if st.Last == nil || st.Last.Status != "completed" {
		t.Fatalf("last = %+v", st.Last)
	}
}

func TestListDeleteAndNameValidation(t *testing.T) {
	dir := t.TempDir()
	svc := newService(t, dir, &fakeSource{})

	if _, err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	st := waitIdle(t, svc)
	name := st.Items[0].Name

	for _, bad := range []string{"../journey.db", "bad.zip", "keepsake-backup-2026.zip", "/etc/passwd"} {
		if _, _, err := svc.Open(bad); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("Open(%q) err = %v, want ErrInvalidName", bad, err)
		}
		if err := svc.Delete(bad); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("Delete(%q) err = %v, want ErrInvalidName", bad, err)
		}
	}
	if err := svc.Delete(name); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := svc.Open(name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open after delete err = %v, want ErrNotFound", err)
	}
}

func TestNewCleansStaleTempFiles(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, ".tmp-keepsake-backup-20260101-000000.zip")
	if err := os.WriteFile(stale, []byte("partial"), 0o600); err != nil {
		t.Fatalf("seed stale: %v", err)
	}
	_ = newService(t, dir, &fakeSource{})
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale temp file should be removed, err=%v", err)
	}
}

func TestEmptyStorageStillCreatesBackup(t *testing.T) {
	dir := t.TempDir()
	svc := newService(t, dir, nil)

	if _, err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	st := waitIdle(t, svc)
	if st.Last == nil || st.Last.Status != "completed" {
		t.Fatalf("last = %+v", st.Last)
	}
	entries := readZip(t, filepath.Join(dir, st.Items[0].Name))
	var manifest Manifest
	if err := json.Unmarshal(entries["manifest.json"], &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.Files.Count != 0 || manifest.Files.Bytes != 0 || len(manifest.Objects) != 0 {
		t.Fatalf("empty storage manifest = %+v", manifest)
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestListOrdersNewestFirst(t *testing.T) {
	dir := t.TempDir()
	svc := newService(t, dir, &fakeSource{})
	older := filepath.Join(dir, "keepsake-backup-20260101-000000.zip")
	newer := filepath.Join(dir, "keepsake-backup-20260102-000000.zip")
	for _, p := range []string{older, newer} {
		if err := os.WriteFile(p, []byte("zip"), 0o600); err != nil {
			t.Fatalf("seed %s: %v", p, err)
		}
	}
	base := time.Now()
	if err := os.Chtimes(older, base.Add(-time.Hour), base.Add(-time.Hour)); err != nil {
		t.Fatalf("chtimes older: %v", err)
	}
	if err := os.Chtimes(newer, base, base); err != nil {
		t.Fatalf("chtimes newer: %v", err)
	}
	items, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 || items[0].Name != filepath.Base(newer) {
		t.Fatalf("items = %+v", items)
	}
}

func TestManifestRecordsCosDriver(t *testing.T) {
	dir := t.TempDir()
	svc, err := New(Options{
		Dir:      dir,
		DBName:   "journey.db",
		Env:      "production",
		Driver:   "cos",
		Snapshot: func(_ context.Context, dst string) error { return os.WriteFile(dst, []byte("db"), 0o600) },
		Objects: &fakeSource{
			objects:  []cosutil.ObjectInfo{{Key: "trips/t1/a.jpg", Size: 1}},
			contents: map[string]string{"trips/t1/a.jpg": "x"},
		},
		Now: func() time.Time { return time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC) },
		Log: slog.New(slog.NewJSONHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	st := waitIdle(t, svc)
	if st.Last == nil || st.Last.Status != "completed" {
		t.Fatalf("last = %+v", st.Last)
	}
	entries := readZip(t, filepath.Join(dir, st.Items[0].Name))
	var manifest Manifest
	if err := json.Unmarshal(entries["manifest.json"], &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.StorageDriver != "cos" || manifest.AppEnv != "production" {
		t.Fatalf("manifest meta = %+v", manifest)
	}
}
