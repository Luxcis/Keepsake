// Package localstore 提供本地文件系统存储后端：开发与单机部署不依赖 COS
// 也能完成照片上传、读取与删除。
//
// 它实现 cosutil.Backend 的最小集合，语义与 COS 私有桶一致：
//   - SignPUT/SignGET 返回指向本服务 /api/storage/* 的短时效 HMAC 签名 URL，
//     浏览器继续走原有的 XHR 直传链路（PUT + Content-Type）；
//   - Head/Delete 直接落到磁盘，供确认入库与删除补偿任务使用。
//
// 签名 URL 是能力凭证（capability URL），不依赖 Cookie，因此这些端点不经过
// 门禁/CSRF 中间件；有效期由调用方配置（默认 15 分钟）。
//
// 对象布局：{root}/{key}，key 仍由服务端生成（trips/{tripID}/{yyyyMM}/{uuid}.{ext}）。
// 读取 key 前只接受无 ..、无绝对路径、无 URL 保留字符的相对路径，防止目录穿越。
package localstore

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"keepsake/internal/cosutil"
)

// URLPrefix 是签名 URL 的固定前缀；路由挂载点必须与之保持一致。
// 前端通过 Vite proxy / Nginx 同源访问，因此返回相对路径即可。
const URLPrefix = "/api/storage"

const defaultURLExpires = 15 * time.Minute

// Options 是 Store 的构造参数。
type Options struct {
	// Root 是对象根目录（通常为 {DATA_DIR}/uploads）。
	Root string
	// URLExpires 是预签名 URL 有效期，<=0 时使用 15 分钟。
	URLExpires time.Duration
	// Secret 是 HMAC 密钥；为空时自动生成并持久化到 {Root}/.secret，
	// 保证进程重启后已签发但未过期的 URL 仍然有效。
	Secret string
	// MaxBytes 是单对象大小上限，<=0 表示不限制（调用方仍应传入业务上限）。
	MaxBytes int64
}

// Store 是基于本地文件系统的 Backend 实现。
type Store struct {
	root     string
	secret   []byte
	expires  time.Duration
	maxBytes int64
}

var _ cosutil.Backend = (*Store)(nil)

// New 创建本地存储。目录不存在时自动创建。
func New(o Options) (*Store, error) {
	if o.Root == "" {
		return nil, errors.New("localstore: root is required")
	}
	root, err := filepath.Abs(o.Root)
	if err != nil {
		return nil, fmt.Errorf("localstore: resolve root: %w", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("localstore: create root: %w", err)
	}
	secret, err := loadSecret(root, o.Secret)
	if err != nil {
		return nil, err
	}
	expires := o.URLExpires
	if expires <= 0 {
		expires = defaultURLExpires
	}
	return &Store{root: root, secret: secret, expires: expires, maxBytes: o.MaxBytes}, nil
}

// loadSecret：显式配置优先；否则读取持久化密钥，缺失时生成 32 字节随机密钥。
func loadSecret(root, configured string) ([]byte, error) {
	if configured != "" {
		return []byte(configured), nil
	}
	secretPath := filepath.Join(root, ".secret")
	b, err := os.ReadFile(secretPath)
	if err == nil {
		if s := strings.TrimSpace(string(b)); len(s) >= 32 {
			return []byte(s), nil
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("localstore: read secret: %w", err)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("localstore: generate secret: %w", err)
	}
	secret := []byte(hex.EncodeToString(raw))
	if err := os.WriteFile(secretPath, secret, 0o600); err != nil {
		return nil, fmt.Errorf("localstore: persist secret: %w", err)
	}
	return secret, nil
}

func (s *Store) Enabled() bool { return true }

// SignPUT 生成直传 URL；Content-Type 参与签名，PUT 时必须携带同一值。
func (s *Store) SignPUT(key, contentType string) (string, time.Time, error) {
	if !validKey(key) {
		return "", time.Time{}, errors.New("localstore: invalid key")
	}
	ct := normalizeContentType(contentType)
	if ct == "" {
		return "", time.Time{}, errors.New("localstore: content type required")
	}
	expireAt := time.Now().Add(s.expires)
	q := url.Values{}
	q.Set("exp", strconv.FormatInt(expireAt.Unix(), 10))
	q.Set("ct", ct)
	q.Set("sig", s.sign(http.MethodPut, key, ct, expireAt.Unix()))
	return URLPrefix + "/upload/" + key + "?" + q.Encode(), expireAt, nil
}

// SignGET 生成读取 URL。
func (s *Store) SignGET(key string) (string, error) {
	if !validKey(key) {
		return "", errors.New("localstore: invalid key")
	}
	exp := time.Now().Add(s.expires).Unix()
	q := url.Values{}
	q.Set("exp", strconv.FormatInt(exp, 10))
	q.Set("sig", s.sign(http.MethodGet, key, "", exp))
	return URLPrefix + "/files/" + key + "?" + q.Encode(), nil
}

// SignGETThumb 本地模式直接返回原图 URL。开发环境不引入图片处理依赖：
// 浏览器按 CSS 尺寸缩放，且同一 URL 可与 src 共用缓存，避免重复下载。
func (s *Store) SignGETThumb(key string) (string, error) { return s.SignGET(key) }

// SignGETPreview 同 SignGETThumb：灯箱也读取原图（本地磁盘无网络开销）。
func (s *Store) SignGETPreview(key string) (string, error) { return s.SignGET(key) }

// Head 返回对象大小与 Content-Type；对象不存在返回 cosutil.ErrObjectMissing。
func (s *Store) Head(key string) (int64, string, error) {
	p, ok := s.pathFor(key)
	if !ok {
		return 0, "", cosutil.ErrObjectMissing
	}
	info, err := os.Stat(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, "", cosutil.ErrObjectMissing
		}
		return 0, "", fmt.Errorf("localstore: stat %s: %w", key, err)
	}
	if info.IsDir() {
		return 0, "", cosutil.ErrObjectMissing
	}
	return info.Size(), contentTypeForKey(key), nil
}

// Delete 删除对象；不存在视为成功（幂等，供 janitor 重试）。
func (s *Store) Delete(key string) error {
	p, ok := s.pathFor(key)
	if !ok {
		return nil // 非法 key 不可能由服务端生成，视为目标状态已达成
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("localstore: delete %s: %w", key, err)
	}
	pruneEmptyDirs(s.root, filepath.Dir(p))
	return nil
}

// ListObjects 遍历存储根目录，返回备份需要的对象 key 与大小。
// 跳过上传中的临时文件、系统文件与非普通文件；.secret 等根级普通文件正常收录。
func (s *Store) ListObjects(ctx context.Context) ([]cosutil.ObjectInfo, error) {
	var out []cosutil.ObjectInfo
	err := filepath.WalkDir(s.root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		base := d.Name()
		// .upload-* 是正在写入的临时对象；其余是本机系统文件或历史备份包，
		// 不应进入新的备份（避免备份目录被误配到存储根时自我嵌套）。
		if strings.HasPrefix(base, ".upload-") || strings.HasPrefix(base, ".tmp-") ||
			base == ".DS_Store" || strings.HasPrefix(base, "keepsake-backup-") {
			return nil
		}
		rel, err := filepath.Rel(s.root, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, cosutil.ObjectInfo{Key: filepath.ToSlash(rel), Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("localstore: list objects: %w", err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// OpenObject 按 key 打开存储对象，供备份逐对象读取。
func (s *Store) OpenObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, ok := s.pathFor(key)
	if !ok {
		return nil, fmt.Errorf("localstore: invalid key %q", key)
	}
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, cosutil.ErrObjectMissing
		}
		return nil, fmt.Errorf("localstore: open %s: %w", key, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("localstore: stat %s: %w", key, err)
	}
	if info.IsDir() {
		_ = f.Close()
		return nil, cosutil.ErrObjectMissing
	}
	return f, nil
}

// Router 返回相对 URLPrefix 挂载的路由：/upload/*（PUT）与 /files/*（GET/HEAD）。
func (s *Store) Router() http.Handler {
	r := chi.NewRouter()
	r.Put("/upload/*", s.handleUpload)
	r.MethodFunc(http.MethodGet, "/files/*", s.handleGet)
	r.MethodFunc(http.MethodHead, "/files/*", s.handleGet)
	return r
}

func (s *Store) handleUpload(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "*")
	p, ok := s.pathFor(key)
	if !ok {
		deny(w)
		return
	}
	exp, ok := parseExpiry(r)
	contentType := normalizeContentType(r.URL.Query().Get("ct"))
	if !ok || contentType == "" ||
		!s.verify(http.MethodPut, key, contentType, r.URL.Query().Get("sig"), exp) {
		deny(w)
		return
	}
	// 直传方必须携带签名时的 Content-Type，与 COS 预签名行为一致。
	if normalizeContentType(r.Header.Get("Content-Type")) != contentType {
		http.Error(w, "content type mismatch", http.StatusForbidden)
		return
	}
	if r.ContentLength > 0 && s.maxBytes > 0 && r.ContentLength > s.maxBytes {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}

	body := r.Body
	if s.maxBytes > 0 {
		body = http.MaxBytesReader(w, r.Body, s.maxBytes)
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slogError("create dir", key, err)
		http.Error(w, "storage unavailable", http.StatusInternalServerError)
		return
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		slogError("create temp", key, err)
		http.Error(w, "storage unavailable", http.StatusInternalServerError)
		return
	}
	tmpName := tmp.Name()
	written, copyErr := io.Copy(tmp, body)
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tmpName)
		var maxErr *http.MaxBytesError
		if errors.As(copyErr, &maxErr) {
			http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
			return
		}
		slogError("write temp", key, errors.Join(copyErr, closeErr))
		http.Error(w, "storage unavailable", http.StatusInternalServerError)
		return
	}
	if written == 0 {
		_ = os.Remove(tmpName)
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		_ = os.Remove(tmpName)
		slogError("chmod temp", key, err)
		http.Error(w, "storage unavailable", http.StatusInternalServerError)
		return
	}
	if err := os.Rename(tmpName, p); err != nil {
		_ = os.Remove(tmpName)
		slogError("rename temp", key, err)
		http.Error(w, "storage unavailable", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Store) handleGet(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "*")
	p, ok := s.pathFor(key)
	if !ok {
		deny(w)
		return
	}
	exp, ok := parseExpiry(r)
	if !ok || !s.verify(http.MethodGet, key, "", r.URL.Query().Get("sig"), exp) {
		deny(w)
		return
	}

	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		slogError("open", key, err)
		http.Error(w, "storage unavailable", http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", contentTypeForKey(key))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if maxAge := exp - time.Now().Unix(); maxAge > 0 {
		w.Header().Set("Cache-Control", "private, max-age="+strconv.FormatInt(maxAge, 10))
	} else {
		w.Header().Set("Cache-Control", "private, max-age=0")
	}
	http.ServeContent(w, r, filepath.Base(p), info.ModTime(), f)
}

// ---------- 签名 ----------

func (s *Store) sign(method, key, extra string, exp int64) string {
	mac := hmac.New(sha256.New, s.secret)
	// 域分隔：版本、方法与附加参数都参与摘要，避免 GET/PUT 签名互换。
	fmt.Fprintf(mac, "v1\n%s\n%s\n%s\n%d", method, key, extra, exp)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Store) verify(method, key, extra, sig string, exp int64) bool {
	if sig == "" || time.Now().Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(s.sign(method, key, extra, exp)), []byte(sig))
}

// ---------- 路径与类型 ----------

// pathFor 把对象 key 解析为磁盘路径，并再次确认结果位于 root 内。
func (s *Store) pathFor(key string) (string, bool) {
	if !validKey(key) {
		return "", false
	}
	p := filepath.Join(s.root, filepath.FromSlash(key))
	rel, err := filepath.Rel(s.root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return p, true
}

// validKey 只接受服务端生成的相对 key 字符集；拒绝 ..、空段与 URL 保留字符。
func validKey(key string) bool {
	if key == "" || len(key) > 512 || strings.HasPrefix(key, "/") {
		return false
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '/', r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	if clean := path.Clean(key); clean != key || clean == "." {
		return false
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

func normalizeContentType(ct string) string {
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	return strings.ToLower(strings.TrimSpace(ct))
}

var extensionTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
	".heic": "image/heic",
	".heif": "image/heif",
	// 实况照片视频对象；与 handlers 的 allowedVideoTypes 保持一致。
	".mp4": "video/mp4",
	".mov": "video/quicktime",
}

// contentTypeForKey 按扩展名返回类型（与 handlers 的上传白名单一致）。
func contentTypeForKey(key string) string {
	ext := strings.ToLower(path.Ext(key))
	if ct, ok := extensionTypes[ext]; ok {
		return ct
	}
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

func parseExpiry(r *http.Request) (int64, bool) {
	exp, err := strconv.ParseInt(r.URL.Query().Get("exp"), 10, 64)
	return exp, err == nil && exp > 0
}

func deny(w http.ResponseWriter) {
	http.Error(w, "invalid or expired url", http.StatusForbidden)
}

// pruneEmptyDirs 从删除对象的目录开始向上清理空目录，止于 root。
func pruneEmptyDirs(root, dir string) {
	for {
		if dir == root || !strings.HasPrefix(dir, root+string(os.PathSeparator)) {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// slogError 保持错误日志统一格式；本地存储错误不包含签名 URL 等敏感信息。
func slogError(op, key string, err error) {
	slog.Error("localstore", "op", op, "key", key, "err", err)
}
