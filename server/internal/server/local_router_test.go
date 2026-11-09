package server_test

// 本地存储端点集成测试：签名 URL 直传/读取经过 /api/storage 路由，
// 且作为能力 URL 不受 Cookie CSRF 校验影响（与 COS 预签名同语义）。

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"keepsake/internal/auth"
	"keepsake/internal/backup"
	"keepsake/internal/cachex"
	"keepsake/internal/config"
	"keepsake/internal/localstore"
	"keepsake/internal/server"
	"keepsake/internal/store"
)

func TestLocalStorageUploadAndRead(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := auth.NewService(st, auth.NewMemorySessionStore(), auth.NewLimiter(nil), log, false)
	ls, err := localstore.New(localstore.Options{Root: t.TempDir(), URLExpires: time.Minute})
	if err != nil {
		t.Fatalf("new local store: %v", err)
	}
	bk, err := backup.New(backup.Options{
		Dir:      filepath.Join(t.TempDir(), "backups"),
		DBName:   "test.db",
		Env:      "dev",
		Driver:   "local",
		Snapshot: st.SnapshotTo,
		Objects:  ls,
		Log:      log,
	})
	if err != nil {
		t.Fatalf("new backup service: %v", err)
	}
	ts := httptest.NewServer(server.New(config.Config{Env: "dev"}, st, cachex.New(nil), ls, bk, svc, log))
	t.Cleanup(ts.Close)

	const key = "trips/trip-1/202611/photo.jpg"
	uploadURL, _, err := ls.SignPUT(key, "image/jpeg")
	if err != nil {
		t.Fatalf("sign put: %v", err)
	}

	// 模拟跨站 Origin：能力签名 URL 不依赖 Cookie，CSRF 中间件不应拦截。
	req, err := http.NewRequest(http.MethodPut, ts.URL+uploadURL, strings.NewReader("jpeg"))
	if err != nil {
		t.Fatalf("build put: %v", err)
	}
	req.Header.Set("Content-Type", "image/jpeg")
	req.Header.Set("Origin", "https://evil.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put status = %d", resp.StatusCode)
	}

	getURL, err := ls.SignGET(key)
	if err != nil {
		t.Fatalf("sign get: %v", err)
	}
	resp, err = http.Get(ts.URL + getURL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "jpeg" {
		t.Fatalf("body = %q", body)
	}
}
