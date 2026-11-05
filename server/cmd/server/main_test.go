// bootstrapAdmin 单测：自动引导仅在“同时配置且该用户名不存在”时创建，其余情况跳过。
package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"keepsake/internal/auth"
	"keepsake/internal/config"
	"keepsake/internal/store"
)

func openTestStore(t *testing.T) (context.Context, *store.Store) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return ctx, st
}

// hasAdmin 复用 AdminPasswordHash 的 ErrNoUser 语义判断指定用户名是否已存在。
func hasAdmin(ctx context.Context, st *store.Store, username string) bool {
	_, err := st.AdminPasswordHash(ctx, username)
	return err == nil
}

func newSvc(st *store.Store, log *slog.Logger) *auth.Service {
	return auth.NewService(st, auth.NewMemorySessionStore(), auth.NewLimiter(nil), log, false)
}

func TestBootstrapAdmin(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	newCtx := func(t *testing.T) (context.Context, *store.Store, *auth.Service) {
		ctx, st := openTestStore(t)
		return ctx, st, newSvc(st, log)
	}

	t.Run("同时配置则创建首个管理员并按可选值写入门禁", func(t *testing.T) {
		ctx, st, svc := newCtx(t)
		bootstrapAdmin(ctx, config.Config{AdminUsername: "admin", AdminPassword: "admin12345", AdminSitePassword: "gate12345"}, st, svc, log)

		if !hasAdmin(ctx, st, "admin") {
			t.Fatal("expected admin to be created")
		}
		hash, err := st.GetSetting(ctx, "site_password")
		if err != nil || hash == "" {
			t.Fatalf("expected site_password set, err=%v hash=%q", err, hash)
		}
	})

	t.Run("同名已存在则跳过且不改密码", func(t *testing.T) {
		ctx, st, svc := newCtx(t)
		if err := svc.CreateAdminUser(ctx, "admin", "original-pass"); err != nil {
			t.Fatalf("seed admin: %v", err)
		}
		// .env 中的密码即使被改过也不生效：只跳过，不重置。
		bootstrapAdmin(ctx, config.Config{AdminUsername: "admin", AdminPassword: "changed-pass"}, st, svc, log)

		hash, err := st.AdminPasswordHash(ctx, "admin")
		if err != nil {
			t.Fatalf("lookup admin: %v", err)
		}
		if auth.VerifyPassword("original-pass", hash) != nil {
			t.Fatal("password was unexpectedly reset")
		}
		if auth.VerifyPassword("changed-pass", hash) == nil {
			t.Fatal("password should not match the new value")
		}
	})

	t.Run("只配其一时不会创建", func(t *testing.T) {
		ctx, st, svc := newCtx(t)
		bootstrapAdmin(ctx, config.Config{AdminUsername: "half"}, st, svc, log)
		if hasAdmin(ctx, st, "half") {
			t.Fatal("admin should not be created when only ADMIN_USERNAME is set")
		}

		bootstrapAdmin(ctx, config.Config{AdminPassword: "pass12345"}, st, svc, log)
		if hasAdmin(ctx, st, "half") {
			t.Fatal("admin should not be created when only ADMIN_PASSWORD is set")
		}
	})

	t.Run("全部未配置时静默跳过", func(t *testing.T) {
		ctx, st, svc := newCtx(t)
		bootstrapAdmin(ctx, config.Config{}, st, svc, log)
		if hasAdmin(ctx, st, "admin") {
			t.Fatal("admin should not be created with empty config")
		}
	})
}
