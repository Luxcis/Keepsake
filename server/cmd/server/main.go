// server 启动入口：加载配置 → 打开 SQLite 并迁移 → 组装auth → 装配路由 → 优雅退出。
// 引导子命令：go run ./cmd/server -create-admin
//
//	读取 ADMIN_USERNAME / ADMIN_PASSWORD（必需）、ADMIN_SITE_PASSWORD（可选）创建管理员，
//	并可选地把站点访问口令（Argon2id 哈希）写入 site_settings 打开门禁。
//
// 常驻启动自动引导：两者同时配置且用户不存在时，run() 内 bootstrapAdmin 会执行同样的引导（幂等，
// 可选 ADMIN_SITE_PASSWORD 一并生效），便于 `docker compose up` 直接用 .env 完成首次建号。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"keepsake/internal/auth"
	"keepsake/internal/backup"
	"keepsake/internal/cachex"
	"keepsake/internal/config"
	"keepsake/internal/cosutil"
	"keepsake/internal/janitor"
	"keepsake/internal/localstore"
	"keepsake/internal/server"
	"keepsake/internal/store"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-create-admin" {
		if err := runCreateAdmin(); err != nil {
			fmt.Fprintln(os.Stderr, "create-admin:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func newLogger(cfg config.Config) *slog.Logger {
	level := slog.LevelInfo
	if !cfg.IsProd() {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

// ---------- -create-admin 引导 ----------

func runCreateAdmin() error {
	cfg := config.Load()
	log := newLogger(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st, err := store.Open(ctx, cfg.DBPath())
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = st.Close() }()
	if _, err := st.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	svc := auth.NewService(st, auth.NewMemorySessionStore(), auth.NewLimiter(nil), log, false)
	username := os.Getenv("ADMIN_USERNAME")
	password := os.Getenv("ADMIN_PASSWORD")
	if username == "" || password == "" {
		return errors.New("environment ADMIN_USERNAME and ADMIN_PASSWORD are required")
	}
	if err := svc.CreateAdminUser(ctx, username, password); err != nil {
		return err
	}
	log.Info("admin created", "username", username)

	if gate := os.Getenv("ADMIN_SITE_PASSWORD"); gate != "" {
		hash, herr := auth.HashPassword(gate)
		if herr != nil {
			return fmt.Errorf("hash site password: %w", herr)
		}
		if serr := st.SetSettings(ctx, map[string]string{"site_password": hash}); serr != nil {
			return fmt.Errorf("store site password: %w", serr)
		}
		log.Info("site access password enabled")
	}
	return nil
}

// ---------- 常驻启动自动引导 ----------

// bootstrapAdmin：常驻启动时的管理员引导，等价于一次性 -create-admin（幂等）。
// 仅当 ADMIN_USERNAME/ADMIN_PASSWORD 同时配置且该用户名不存在时才创建；
// 已存在（不重置密码）、校验失败、写入失败等情况只记日志，不阻断服务启动。
// 配置了 ADMIN_SITE_PASSWORD 且本次创建成功时，顺带写入站点门禁（不覆盖已设口令）。
func bootstrapAdmin(ctx context.Context, cfg config.Config, st *store.Store, svc *auth.Service, log *slog.Logger) {
	switch {
	case cfg.AdminUsername == "" && cfg.AdminPassword == "":
		return
	case cfg.AdminUsername == "" || cfg.AdminPassword == "":
		log.Warn("admin auto-bootstrap skipped: ADMIN_USERNAME and ADMIN_PASSWORD must be set together")
		return
	}

	_, err := st.AdminPasswordHash(ctx, cfg.AdminUsername)
	switch {
	case err == nil:
		log.Info("admin already exists, auto-bootstrap skipped", "username", cfg.AdminUsername)
		return
	case !errors.Is(err, store.ErrNoUser):
		log.Error("admin auto-bootstrap lookup failed", "err", err)
		return
	}

	if err := svc.CreateAdminUser(ctx, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		log.Error("admin auto-bootstrap failed", "err", err)
		return
	}
	log.Info("admin auto-bootstrap created", "username", cfg.AdminUsername)

	if cfg.AdminSitePassword == "" {
		return
	}
	hash, err := auth.HashPassword(cfg.AdminSitePassword)
	if err != nil {
		log.Error("auto-bootstrap hash site password failed", "err", err)
		return
	}
	if err := st.SetSettings(ctx, map[string]string{"site_password": hash}); err != nil {
		log.Error("auto-bootstrap store site password failed", "err", err)
		return
	}
	log.Info("site access password enabled")
}

// ---------- 常驻服务 ----------

// newStorageBackend 按 STORAGE_DRIVER 选择存储后端，并返回实际驱动名供备份清单记录。
// 留空自动选择：COS 四项齐全 → COS；否则开发环境 → 本地磁盘；生产环境 → 禁用上传。
// 显式 local 也可用于单机生产部署（STORAGE_DIR 必须位于持久卷上）。
func newStorageBackend(cfg config.Config, log *slog.Logger) (cosutil.Backend, string, error) {
	driver := cfg.StorageDriver
	if driver == "" {
		switch {
		case cfg.COSConfigured():
			driver = "cos"
		case !cfg.IsProd():
			driver = "local"
		default:
			driver = "none"
		}
	}

	switch driver {
	case "local":
		ls, err := localstore.New(localstore.Options{
			Root:       cfg.StorageDir,
			URLExpires: cfg.StorageURLExpires,
			Secret:     cfg.StorageSecret,
			// 存储层放行到视频上限；图片的 30MB 由签名与确认阶段按类型校验。
			MaxBytes: config.MaxMediaUploadBytes,
		})
		if err != nil {
			return nil, "", fmt.Errorf("init local storage: %w", err)
		}
		log.Info("storage enabled", "driver", "local", "root", cfg.StorageDir,
			"url_expires", cfg.StorageURLExpires)
		return ls, "local", nil
	case "cos":
		if !cfg.COSConfigured() {
			return nil, "", errors.New("STORAGE_DRIVER=cos requires COS_SECRET_ID/COS_SECRET_KEY/COS_REGION/COS_BUCKET")
		}
		client, err := cosutil.New(cosutil.Options{
			SecretID:   cfg.COSSecretID,
			SecretKey:  cfg.COSSecretKey,
			Region:     cfg.COSRegion,
			Bucket:     cfg.COSBucket,
			CDNDomain:  cfg.COSCDNDomain,
			URLExpires: cfg.COSURLExpires,
		})
		if err != nil {
			return nil, "", fmt.Errorf("init cos storage: %w", err)
		}
		cdn := cfg.COSCDNDomain
		if cdn == "" {
			cdn = "disabled"
		}
		log.Info("storage enabled", "driver", "cos", "bucket", cfg.COSBucket,
			"region", cfg.COSRegion, "read_cdn", cdn, "url_expires", cfg.COSURLExpires)
		return client, "cos", nil
	case "none":
		log.Warn("storage not configured: photo upload disabled (503); set STORAGE_DRIVER=local to use local disk")
		return cosutil.NoopBackend{}, "none", nil
	default:
		return nil, "", fmt.Errorf("unknown STORAGE_DRIVER %q (want local or cos)", driver)
	}
}

func run() error {
	cfg := config.Load()

	log := newLogger(cfg)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- SQLite（唯一业务事实来源），启动即迁移到最新版本 ---
	st, err := store.Open(ctx, cfg.DBPath())
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = st.Close() }()

	applied, err := st.Migrate(ctx)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	log.Info("sqlite ready", "path", cfg.DBPath(), "applied", applied)

	// --- Redis（可选依赖；未配置 → 内存会话/限流降级，属合法部署形态，见启动日志） ---
	var rdb *redis.Client
	if cfg.RedisAddr != "" {
		rdb = redis.NewClient(&redis.Options{
			Addr:        cfg.RedisAddr,
			Password:    cfg.RedisPassword, // REDIS_PASSWORD；为空即无密码（go-redis 跳过 AUTH）
			DialTimeout: 2 * time.Second,
		})
	}
	cch := cachex.New(rdb)
	defer func() { _ = cch.Close() }()
	if perr := cch.Ping(ctx); perr != nil {
		log.Warn("redis unavailable, run degraded", "addr", cfg.RedisAddr, "err", perr)
		if cfg.IsProd() && perr != nil && !errors.Is(perr, cachex.ErrDisabled) && cfg.RedisAddr != "" {
			// 失败关闭生效：认证与管理写入在 Redis 恢复前返回 503。
			log.Error("fail-closed active for auth/admin writes until redis reaches")
		}
	}

	// --- 认证服务（计划 §5.1） ---
	var sessions auth.SessionStore
	if rdb != nil {
		sessions = auth.NewRedisSessionStore(rdb)
	} else {
		sessions = auth.NewMemorySessionStore()
		if cfg.IsProd() {
			// Redis 已是可选依赖（单实例部署可走内存会话）：记录告警即可，非错误。
			log.Warn("redis not configured: sessions/rate-limit are in-process (lost on restart)")
		}
	}
	limiter := auth.NewLimiter(rdb)
	svc := auth.NewService(st, sessions, limiter, log, cfg.CookieSecure)

	// --- 管理员自动引导：.env/compose 配置 ADMIN_* 即随启动创建首个管理员（幂等，见 bootstrapAdmin） ---
	bootstrapAdmin(ctx, cfg, st, svc, log)

	// --- 存储后端（local / COS；未配置 → NoopBackend，上传接口返回明确的 503） ---
	storageBackend, storageDriver, err := newStorageBackend(cfg, log)
	if err != nil {
		return err
	}

	// --- 备份服务（管理端一键备份：数据库快照 + 存储对象 + 清单） ---
	objects, _ := storageBackend.(cosutil.ObjectSource)
	backupSvc, err := backup.New(backup.Options{
		Dir:      cfg.BackupDir,
		DBName:   cfg.DBFile,
		Env:      cfg.Env,
		Driver:   storageDriver,
		Snapshot: st.SnapshotTo,
		Objects:  objects,
		Log:      log,
	})
	if err != nil {
		return fmt.Errorf("init backup service: %w", err)
	}
	log.Info("backup enabled", "dir", backupSvc.Dir(), "storage_driver", storageDriver)

	// --- 删除补偿后台（计划 §4.1：对象清理失败可重试） ---
	go func() { janitor.New(st, storageBackend).Run(ctx) }()

	handler := server.New(cfg, st, cch, storageBackend, backupSvc, svc, log)
	readTimeout, writeTimeout := 15*time.Second, 30*time.Second
	if _, isLocal := storageBackend.(*localstore.Store); isLocal {
		// 本地存储模式下媒体经 /api/storage 直传：400MB 视频在慢速上行下可能持续十余分钟，
		// 读写超时必须覆盖整个请求；ReadHeaderTimeout 仍限制慢速头攻击。
		readTimeout, writeTimeout = 15*time.Minute, 15*time.Minute
	}
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       90 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", srv.Addr, "env", cfg.Env, "cookieSecure", cfg.CookieSecure)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("graceful shutdown failed", "err", err)
		}
		return nil
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("listen: %w", err)
	}
}
