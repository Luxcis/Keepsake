// Package config 聚合服务端运行配置。
// 12-factor：全部来自环境变量，容器内外一致；开发默认值见 Load()。
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// MaxUploadBytes 是单张图片上传上限（30MB，与前端/后端校验一致）。
// 签名接口、本地存储写入与 COS 确认阶段都按媒体类型选择上限。
const MaxUploadBytes int64 = 30 << 20

// MaxVideoUploadBytes 是单个视频对象（独立视频或实况照片视频）的上传上限（400MB）。
const MaxVideoUploadBytes int64 = 400 << 20

// MaxMediaUploadBytes 是存储层单对象写入上限，取图片与视频两类的最大值；
// 具体媒体类型仍由签名与确认阶段分别校验。
const MaxMediaUploadBytes int64 = MaxVideoUploadBytes

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
	// RedisPassword: 可选；连接开启 requirepass 的 Redis（如服务器已有实例）时填写，
	// 为空表示无密码（compose 内置 redis 即无密码）。跟密钥类变量同风格，不落盘。
	RedisPassword string
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
	// COSOriginDomain: 可选；COS 控制台绑定的「自定义源站域名」（如 photos.example.com）。
	// 为空时使用默认 {bucket}.cos.{region}.myqcloud.com；预签名与 HEAD/DELETE 均走该域名。
	COSOriginDomain string
	// COSURLExpires: 预签名 URL 有效期秒数（计划 §5.1：短时效）。
	COSURLExpires time.Duration
	// StorageDriver: 存储后端选择：local / cos；留空自动选择
	// （COS 四项齐全优先用 COS，否则开发环境用本地磁盘，生产环境禁用上传）。
	StorageDriver string
	// StorageDir: 本地存储根目录（默认 {DataDir}/uploads）。
	StorageDir string
	// StorageSecret: 本地存储签名 URL 的 HMAC 密钥；留空时自动生成并持久化到
	// {StorageDir}/.secret，重启后已签发的未过期 URL 仍有效。
	StorageSecret string
	// StorageURLExpires: 本地存储签名 URL 有效期（与 COS 预签名同语义）。
	StorageURLExpires time.Duration
	// CORSOrigins: 开发期允许的跨域来源，逗号分隔；生产由 Nginx 同源提供，可留空。
	CORSOrigins []string
}

func Load() Config {
	dataDir := getEnv("DATA_DIR", "./data")
	return Config{
		Env:             getEnv("APP_ENV", "dev"),
		Port:            getEnv("API_PORT", "8080"),
		DataDir:         dataDir,
		DBFile:          getEnv("DB_FILE", "journey.db"),
		RedisAddr:       getEnv("REDIS_ADDR", ""),
		RedisPassword:   os.Getenv("REDIS_PASSWORD"),
		CookieSecure:    getCookieSecure(getEnv("COOKIE_SECURE", "")),
		COSSecretID:     os.Getenv("COS_SECRET_ID"),
		COSSecretKey:    os.Getenv("COS_SECRET_KEY"),
		COSRegion:       getEnv("COS_REGION", ""),
		COSBucket:       getEnv("COS_BUCKET", ""),
		COSOriginDomain: strings.TrimSpace(os.Getenv("COS_ORIGIN_DOMAIN")),
		COSURLExpires:   getURLExpires("COS_URL_EXPIRES", 900),
		// 存储后端：显式 local/cos；留空由启动逻辑按环境与 COS 配置选择。
		StorageDriver:     strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_DRIVER"))),
		StorageDir:        getEnv("STORAGE_DIR", filepath.Join(dataDir, "uploads")),
		StorageSecret:     os.Getenv("STORAGE_SECRET"),
		StorageURLExpires: getURLExpires("STORAGE_URL_EXPIRES", 900),
		CORSOrigins:       splitAndTrim(getEnv("CORS_ORIGINS", "http://localhost:5173")),
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

// COSConfigured 判断 COS 四项是否齐全（密钥、区域、桶名任意缺失即视为未配置）。
func (c Config) COSConfigured() bool {
	return c.COSSecretID != "" && c.COSSecretKey != "" && c.COSRegion != "" && c.COSBucket != ""
}

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
