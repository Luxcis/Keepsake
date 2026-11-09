package server_test

// 管理端一键备份集成测试：创建 → 轮询 → 下载 → 删除，以及鉴权、并发生成与失败清理。

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"keepsake/internal/backup"
	"keepsake/internal/cosutil"
	"keepsake/internal/localstore"
)

func loginAdmin(t *testing.T, d *deps) {
	t.Helper()
	r := d.do(t, "POST", "/api/auth/login", `{"username":"admin","password":"secret123"}`)
	r.expectStatus(t, http.StatusOK)
}

// waitBackupDone 轮询状态接口直到当前任务结束，返回 data 节点。
func waitBackupDone(t *testing.T, d *deps) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		r := d.do(t, "GET", "/api/admin/backups", "")
		r.expectStatus(t, http.StatusOK)
		data, _ := r.decode(t)["data"].(map[string]any)
		if data == nil {
			t.Fatalf("missing data: %s", r.body)
		}
		if data["running"] == nil {
			return data
		}
		if time.Now().After(deadline) {
			t.Fatal("backup did not finish in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAdminBackupCreateDownloadDelete(t *testing.T) {
	root := t.TempDir()
	ls, err := localstore.New(localstore.Options{Root: root})
	if err != nil {
		t.Fatalf("new local store: %v", err)
	}
	d := newStackWithBackend(t, ls)
	loginAdmin(t, d)

	// 备份前写入一条业务数据与一个上传对象，验证快照与文件都被收录。
	r := d.do(t, "POST", "/api/admin/trips",
		`{"title":"备份前旅行","summary":"s","start_date":"2026-08-01","status":"draft"}`)
	r.expectStatus(t, http.StatusCreated)

	objectKey := "trips/trip-1/202611/a.jpg"
	objectPath := filepath.Join(root, filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(objectPath), 0o755); err != nil {
		t.Fatalf("mkdir object: %v", err)
	}
	if err := os.WriteFile(objectPath, []byte("jpeg-bytes"), 0o644); err != nil {
		t.Fatalf("write object: %v", err)
	}

	r = d.do(t, "POST", "/api/admin/backups", "")
	r.expectStatus(t, http.StatusAccepted)
	started := r.decode(t)["data"].(map[string]any)
	name, _ := started["name"].(string)
	if name == "" || started["started"] != true {
		t.Fatalf("create response = %s", r.body)
	}

	data := waitBackupDone(t, d)
	last, _ := data["last"].(map[string]any)
	if last["status"] != "completed" {
		t.Fatalf("last = %v", last)
	}
	items, _ := data["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items = %v", items)
	}
	item, _ := items[0].(map[string]any)
	if item["name"] != name {
		t.Fatalf("item name = %v, want %s", item["name"], name)
	}
	if size, _ := item["size_bytes"].(float64); size <= 0 {
		t.Fatalf("size_bytes = %v", item["size_bytes"])
	}
	if _, err := os.Stat(filepath.Join(d.backupDir, name)); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}

	// 下载并核对包内容。
	req, err := http.NewRequest(http.MethodGet, d.base+"/api/admin/backups/"+url.PathEscape(name), nil)
	if err != nil {
		t.Fatalf("build download: %v", err)
	}
	res, err := d.http.Do(req)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("download status = %d body=%s", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("content-type = %q", ct)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, name) {
		t.Fatalf("content-disposition = %q", cd)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read download: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	entries := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry %s: %v", f.Name, err)
		}
		b, rerr := io.ReadAll(rc)
		_ = rc.Close()
		if rerr != nil {
			t.Fatalf("read entry %s: %v", f.Name, rerr)
		}
		entries[f.Name] = b
	}
	for _, want := range []string{"manifest.json", "test.db", "uploads/" + objectKey} {
		if _, ok := entries[want]; !ok {
			t.Fatalf("zip missing %s (got %v)", want, entryNames(entries))
		}
	}
	for name := range entries {
		if strings.Contains(name, ".env") || strings.Contains(name, "dump.rdb") ||
			strings.Contains(name, "appendonly") || strings.HasPrefix(name, "backups/") {
			t.Fatalf("zip must not contain env/redis/backup dir, got %q", name)
		}
	}
	if string(entries["uploads/"+objectKey]) != "jpeg-bytes" {
		t.Fatalf("object content = %q", entries["uploads/"+objectKey])
	}

	var manifest backup.Manifest
	if err := json.Unmarshal(entries["manifest.json"], &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.Schema != backup.ManifestSchema || manifest.StorageDriver != "local" {
		t.Fatalf("manifest = %+v", manifest)
	}
	if manifest.CreatedAt.IsZero() || manifest.AppEnv != "dev" {
		t.Fatalf("manifest meta = %+v", manifest)
	}
	// 本地存储根目录下的 .secret 也属于站点文件，会一并收录。
	if manifest.Files.Count != 2 {
		t.Fatalf("manifest files = %+v", manifest.Files)
	}
	foundObject := false
	for _, obj := range manifest.Objects {
		if obj.Key == objectKey {
			foundObject = true
		}
	}
	if !foundObject {
		t.Fatalf("manifest objects = %+v", manifest.Objects)
	}

	// 快照可独立打开并读到备份前创建的旅行。
	dbPath := filepath.Join(t.TempDir(), "snapshot.db")
	if err := os.WriteFile(dbPath, entries["test.db"], 0o600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	ro, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer func() { _ = ro.Close() }()
	var title string
	if err := ro.QueryRow(`SELECT title FROM trips LIMIT 1`).Scan(&title); err != nil {
		t.Fatalf("query snapshot: %v", err)
	}
	if title != "备份前旅行" {
		t.Fatalf("snapshot trip title = %q", title)
	}

	// 删除后列表与磁盘文件同时消失。
	r = d.do(t, "DELETE", "/api/admin/backups/"+url.PathEscape(name), "")
	r.expectStatus(t, http.StatusOK)
	if _, err := os.Stat(filepath.Join(d.backupDir, name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup file should be removed, err=%v", err)
	}
	r = d.do(t, "GET", "/api/admin/backups", "")
	r.expectStatus(t, http.StatusOK)
	items, _ = r.decode(t)["data"].(map[string]any)["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("items after delete = %v", items)
	}
}

func TestAdminBackupAuthAndInvalidNames(t *testing.T) {
	d := newStack(t)

	// 未登录：列表、创建、下载、删除都要求管理员会话。
	anon := &http.Client{}
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/admin/backups"},
		{http.MethodPost, "/api/admin/backups"},
		{http.MethodGet, "/api/admin/backups/keepsake-backup-20260101-000000.zip"},
		{http.MethodDelete, "/api/admin/backups/keepsake-backup-20260101-000000.zip"},
	} {
		req, err := http.NewRequest(tc.method, d.base+tc.path, nil)
		if err != nil {
			t.Fatalf("build %s %s: %v", tc.method, tc.path, err)
		}
		res, err := anon.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		raw, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized || !strings.Contains(string(raw), "auth_required") {
			t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, res.StatusCode, raw)
		}
	}

	loginAdmin(t, d)

	// 跨站 Origin 的创建/删除被 CSRF 拒绝。
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/admin/backups"},
		{http.MethodDelete, "/api/admin/backups/keepsake-backup-20260101-000000.zip"},
	} {
		req, err := http.NewRequest(tc.method, d.base+tc.path, nil)
		if err != nil {
			t.Fatalf("build %s: %v", tc.method, err)
		}
		req.Header.Set("Origin", "https://evil.example.com")
		res, err := d.http.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", tc.method, err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("%s cross-origin status = %d", tc.method, res.StatusCode)
		}
	}

	// 非法备份名被拒绝，且不会影响备份目录之外的路径。
	sentinel := filepath.Join(filepath.Dir(d.backupDir), "keep-me.txt")
	if err := os.WriteFile(sentinel, []byte("safe"), 0o600); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	for _, name := range []string{"bad.zip", "..%2F..%2Fetc%2Fpasswd"} {
		r := d.do(t, "GET", "/api/admin/backups/"+name, "")
		if r.status != http.StatusBadRequest && r.status != http.StatusNotFound {
			t.Fatalf("GET %s status = %d body=%s", name, r.status, r.body)
		}
		r = d.do(t, "DELETE", "/api/admin/backups/"+name, "")
		if r.status != http.StatusBadRequest && r.status != http.StatusNotFound {
			t.Fatalf("DELETE %s status = %d body=%s", name, r.status, r.body)
		}
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("file outside backup dir must be untouched: %v", err)
	}
}

// blockingBackend 在读取对象时阻塞，用于稳定复现并发创建与失败清理。
type blockingBackend struct {
	cosutil.NoopBackend
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	fail    bool
}

func (b *blockingBackend) ListObjects(context.Context) ([]cosutil.ObjectInfo, error) {
	return []cosutil.ObjectInfo{{Key: "a.jpg", Size: 1}}, nil
}

func (b *blockingBackend) OpenObject(context.Context, string) (io.ReadCloser, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.release
	if b.fail {
		return nil, errors.New("read failed")
	}
	return io.NopCloser(strings.NewReader("x")), nil
}

func TestAdminBackupConflictAndFailure(t *testing.T) {
	t.Run("concurrent start returns 409", func(t *testing.T) {
		b := &blockingBackend{entered: make(chan struct{}), release: make(chan struct{})}
		d := newStackWithBackend(t, b)
		loginAdmin(t, d)

		r := d.do(t, "POST", "/api/admin/backups", "")
		r.expectStatus(t, http.StatusAccepted)
		select {
		case <-b.entered:
		case <-time.After(3 * time.Second):
			t.Fatal("backup did not reach object read")
		}

		// 备份生成期间站点仍可写入。
		r = d.do(t, "POST", "/api/admin/trips",
			`{"title":"备份期间旅行","summary":"s","start_date":"2026-08-02","status":"draft"}`)
		r.expectStatus(t, http.StatusCreated)

		r = d.do(t, "POST", "/api/admin/backups", "")
		r.expectStatus(t, http.StatusConflict)
		if code := r.errorCode(t); code != "backup_running" {
			t.Fatalf("code = %s", code)
		}

		close(b.release)
		data := waitBackupDone(t, d)
		last, _ := data["last"].(map[string]any)
		if last["status"] != "completed" {
			t.Fatalf("last = %v", last)
		}
	})

	t.Run("object read failure leaves no backup", func(t *testing.T) {
		b := &blockingBackend{entered: make(chan struct{}), release: make(chan struct{}), fail: true}
		close(b.release) // 立即失败，不需要外部放行
		d := newStackWithBackend(t, b)
		loginAdmin(t, d)

		r := d.do(t, "POST", "/api/admin/backups", "")
		r.expectStatus(t, http.StatusAccepted)

		data := waitBackupDone(t, d)
		last, _ := data["last"].(map[string]any)
		if last["status"] != "failed" || last["error"] == "" {
			t.Fatalf("last = %v", last)
		}
		if items, _ := data["items"].([]any); len(items) != 0 {
			t.Fatalf("failed backup must not be listed: %v", items)
		}
		entries, err := os.ReadDir(d.backupDir)
		if err != nil {
			t.Fatalf("read backup dir: %v", err)
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".zip") || strings.HasPrefix(e.Name(), ".tmp-") {
				t.Fatalf("leftover file %q", e.Name())
			}
		}
	})
}

func entryNames(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
