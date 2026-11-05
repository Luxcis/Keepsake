// Package config 聚合服务端运行配置。
// 12-factor：全部来自环境变量，容器内外一致；开发默认值见 Load()。
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// Env: dev / production。production 下日志为纯 JSON，便于容器采集。
	Env string
	// Port: HTTP 监听端口（默认 8080，与 docker-compose / Nginx 代理一致）。
	Port string
	// DataDir: 持久化目录（SQLite 数据库文件所在目录）。容器内挂载 /data。
	DataDir string
	// DBFile: SQLite 数据库文件名。
	DBFile string
	// RedisAddr: 为空表示不启用 Redis（合法部署形态，会话/限流退化为进程内存；
	// 多实例部署需启用）。计划 §3.2：已配置但不可用时认证与管理写入应失败关闭。
	RedisAddr string
	// CookieSecure: 会话 Cookie 的 Secure 位。默认不启用；生产 HTTPS 网关下显式开启。
	// 本地 HTTP 联调（如 compose 无 TLS）保持 false。
	CookieSecure bool
	// 管理员自动引导：ADMIN_USERNAME/ADMIN_PASSWORD 同时配置时，
	// 常驻启动会自动创建首个管理员（同名已存在则跳过，不重置密码）；
	// 重置密码或追加账号仍走 -create-admin CLI。三者留空即完全关闭自动引导。
	AdminUsername string
	AdminPassword string
	// AdminSitePassword: 可选；仅在本进程成功创建管理员时顺带写入站点访问门禁，
	// 不会覆盖已存在的门禁口令。
	AdminSitePassword string
	// COS：腾讯 COS 私有桶访问参数（计划 §3.1：密钥仅保存在服务端）。
	// ID/Key/Region/Bucket 四项均配置才视作启用。
	COSSecretID  string
	COSSecretKey string
	COSRegion    string
	COSBucket    string
	// COSURLExpires: 预签名 URL 有效期秒数（计划 §5.1：短时效）。
	COSURLExpires time.Duration
	// CORSOrigins: 开发期允许的跨域来源，逗号分隔；生产由 Nginx 同源提供，可留空。
	CORSOrigins []string
}

func Load() Config {
	return Config{
		Env:           getEnv("APP_ENV", "dev"),
		Port:          getEnv("API_PORT", "8080"),
		DataDir:       getEnv("DATA_DIR", "./data"),
		DBFile:        getEnv("DB_FILE", "journey.db"),
		RedisAddr:     getEnv("REDIS_ADDR", ""),
		CookieSecure:  getCookieSecure(getEnv("COOKIE_SECURE", "")),
		COSSecretID:   os.Getenv("COS_SECRET_ID"),
		COSSecretKey:  os.Getenv("COS_SECRET_KEY"),
		COSRegion:     getEnv("COS_REGION", ""),
		COSBucket:     getEnv("COS_BUCKET", ""),
		COSURLExpires: getURLExpires("COS_URL_EXPIRES", 900),
		CORSOrigins:   splitAndTrim(getEnv("CORS_ORIGINS", "http://localhost:5173")),
		// 引导类变量无默认值，空即未启用（与密钥类同风格）。
		AdminUsername:     os.Getenv("ADMIN_USERNAME"),
		AdminPassword:     os.Getenv("ADMIN_PASSWORD"),
		AdminSitePassword: os.Getenv("ADMIN_SITE_PASSWORD"),
	}
}

func getURLExpires(key string, def int) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 60 && n <= 3600 {
			return time.Duration(n) * time.Second
		}
	}
	return time.Duration(def) * time.Second
}

// getCookieSecure：显式 true/false 优先；未配置时默认 false（HTTPS 就绪后再开）。
func getCookieSecure(v string) bool {
	switch strings.ToLower(v) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	}
	return false
}

// DBPath 返回 SQLite 数据库文件的完整路径。
// 注意（计划 §5.3）：不要把该文件放到 NFS/共享文件系统上；备份请走 Online Backup API。
func (c Config) DBPath() string {
	return c.DataDir + string(os.PathSeparator) + c.DBFile
}

func (c Config) IsProd() bool { return c.Env == "production" }

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitAndTrim(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
