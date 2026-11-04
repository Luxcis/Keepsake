// Package server 组装 HTTP 路由与中间件（计划 §3.1：/api 由 Nginx 反向代理至此）。
package server

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"keepsake/internal/auth"
	"keepsake/internal/cachex"
	"keepsake/internal/config"
	"keepsake/internal/cosutil"
	"keepsake/internal/handlers"
	"keepsake/internal/store"
)

// New 返回装配好中间件与路由的 http.Handler。
func New(cfg config.Config, st *store.Store, cch *cachex.Cache, cos cosutil.Backend, svc *auth.Service, log *slog.Logger) http.Handler {
	r := chi.NewRouter()

	// 请求上下文：RequestID → RealIP → 结构化日志 → Panic 恢复。
	r.Use(middleware.CleanPath, middleware.RequestID, middleware.RealIP)
	r.Use(requestLogger(log), middleware.Recoverer)
	if len(cfg.CORSOrigins) > 0 {
		r.Use(cors(cfg.CORSOrigins))
	}

	h := handlers.New(st, cch, svc, cos, log)

	// 健康检查独立于 /api，便于容器/Nginx 直接探测。
	r.Get("/healthz", h.Health)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if isAPIPath(r.URL.Path) {
			handlers.NotFound(w)
			return
		}
		http.NotFound(w, r)
	})

	r.Route("/api", func(r chi.Router) {
		r.Use(csrfSameOrigin)
		h.Mount(r)
	})
	return r
}

func isAPIPath(path string) bool { return strings.HasPrefix(path, "/api/") || path == "/api" }

// requestLogger 用 slog 输出结构化访问日志。
// 计划 §5.1 日志要求：不记录敏感信息（签名 URL、密码、Cookie）。
func requestLogger(log *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := timeNow()
			defer func() {
				log.Info("http",
					"request_id", middleware.GetReqID(r.Context()),
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.Status(),
					"bytes", ww.BytesWritten(),
					"ms", timeSince(start).Milliseconds(),
				)
			}()
			next.ServeHTTP(ww, r)
		})
	}
}

// cors 一个只支持开发期的精简 CORS 中间件：生产走 Nginx 同源，可留空不启用。
func cors(allowList []string) func(next http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowList))
	for _, o := range allowList {
		allowed[strings.ToLower(strings.TrimSuffix(o, "/"))] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.ToLower(r.Header.Get("Origin"))
			if origin != "" && allowed[origin] {
				w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
