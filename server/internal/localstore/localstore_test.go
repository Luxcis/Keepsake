package localstore

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"keepsake/internal/cosutil"
)

const testKey = "trips/trip-1/202611/photo.jpg"

func newTestStore(t *testing.T, opts Options) (*Store, *httptest.Server) {
	t.Helper()
	if opts.Root == "" {
		opts.Root = t.TempDir()
	}
	s, err := New(opts)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	r := chi.NewRouter()
	r.Mount(URLPrefix, s.Router())
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return s, srv
}

func doPUT(t *testing.T, srv *httptest.Server, rawURL, contentType string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, srv.URL+rawURL, strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("new put request: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func doGET(t *testing.T, srv *httptest.Server, rawURL string) *http.Response {
	t.Helper()
	resp, err := http.Get(srv.URL + rawURL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestUploadReadHeadDelete(t *testing.T) {
	s, srv := newTestStore(t, Options{URLExpires: time.Minute})
	body := []byte("jpeg-bytes")

	uploadURL, expireAt, err := s.SignPUT(testKey, "image/jpeg")
	if err != nil {
		t.Fatalf("sign put: %v", err)
	}
	if !strings.HasPrefix(uploadURL, URLPrefix+"/upload/"+testKey+"?") {
		t.Fatalf("unexpected upload url: %s", uploadURL)
	}
	if time.Until(expireAt) <= 0 {
		t.Fatalf("expireAt not in future: %v", expireAt)
	}
	if resp := doPUT(t, srv, uploadURL, "image/jpeg", body); resp.StatusCode != http.StatusOK {
		t.Fatalf("put status = %d", resp.StatusCode)
	}

	size, contentType, err := s.Head(testKey)
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	if size != int64(len(body)) || contentType != "image/jpeg" {
		t.Fatalf("head = (%d, %q)", size, contentType)
	}

	getURL, err := s.SignGET(testKey)
	if err != nil {
		t.Fatalf("sign get: %v", err)
	}
	resp := doGET(t, srv, getURL)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("get content type = %q", ct)
	}
	got, _ := io.ReadAll(resp.Body)
	if string(got) != string(body) {
		t.Fatalf("get body = %q", got)
	}

	if err := s.Delete(testKey); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, _, err := s.Head(testKey); !errors.Is(err, cosutil.ErrObjectMissing) {
		t.Fatalf("head after delete = %v, want ErrObjectMissing", err)
	}
	if err := s.Delete(testKey); err != nil {
		t.Fatalf("second delete should be idempotent: %v", err)
	}
}

func TestThumbAndPreviewUseOriginalURL(t *testing.T) {
	s, _ := newTestStore(t, Options{URLExpires: time.Minute})
	src, err := s.SignGET(testKey)
	if err != nil {
		t.Fatalf("sign get: %v", err)
	}
	thumb, err := s.SignGETThumb(testKey)
	if err != nil || thumb != src {
		t.Fatalf("thumb = %q, %v; want same as src", thumb, err)
	}
	preview, err := s.SignGETPreview(testKey)
	if err != nil || preview != src {
		t.Fatalf("preview = %q, %v; want same as src", preview, err)
	}
}

func TestUploadRejectsMismatchedContentType(t *testing.T) {
	s, srv := newTestStore(t, Options{URLExpires: time.Minute})
	uploadURL, _, err := s.SignPUT(testKey, "image/jpeg")
	if err != nil {
		t.Fatalf("sign put: %v", err)
	}
	if resp := doPUT(t, srv, uploadURL, "image/png", []byte("x")); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestExpiredAndTamperedURLsRejected(t *testing.T) {
	s, srv := newTestStore(t, Options{URLExpires: time.Minute})
	store := []byte("data")

	// 用过期时间重新签一个 GET URL：签名有效但已过期。
	exp := time.Now().Add(-time.Minute).Unix()
	q := url.Values{}
	q.Set("exp", strconv.FormatInt(exp, 10))
	q.Set("sig", s.sign(http.MethodGet, testKey, "", exp))
	if resp := doGET(t, srv, URLPrefix+"/files/"+testKey+"?"+q.Encode()); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expired status = %d, want 403", resp.StatusCode)
	}

	getURL, err := s.SignGET(testKey)
	if err != nil {
		t.Fatalf("sign get: %v", err)
	}
	tampered := strings.Replace(getURL, "sig=", "sig=x", 1)
	if resp := doGET(t, srv, tampered); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("tampered status = %d, want 403", resp.StatusCode)
	}

	uploadURL, _, err := s.SignPUT(testKey, "image/jpeg")
	if err != nil {
		t.Fatalf("sign put: %v", err)
	}
	if resp := doPUT(t, srv, strings.Replace(uploadURL, "sig=", "sig=x", 1), "image/jpeg", store); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("tampered put status = %d, want 403", resp.StatusCode)
	}
}

func TestMaxBytesEnforced(t *testing.T) {
	s, srv := newTestStore(t, Options{URLExpires: time.Minute, MaxBytes: 8})
	uploadURL, _, err := s.SignPUT(testKey, "image/jpeg")
	if err != nil {
		t.Fatalf("sign put: %v", err)
	}
	if resp := doPUT(t, srv, uploadURL, "image/jpeg", []byte("123456789")); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", resp.StatusCode)
	}
}

func TestInvalidKeysRejected(t *testing.T) {
	s, _ := newTestStore(t, Options{})
	for _, key := range []string{
		"", "/etc/passwd", "../secret", "trips/../../x", "trips//x", "trips/./x",
		"trips/x/202611/a b.jpg", "trips/x/202611/a%2ejpg", `trips\x\y.jpg`,
	} {
		if _, _, err := s.SignPUT(key, "image/jpeg"); err == nil {
			t.Errorf("SignPUT(%q) should fail", key)
		}
		if _, err := s.SignGET(key); err == nil {
			t.Errorf("SignGET(%q) should fail", key)
		}
		if _, _, err := s.Head(key); !errors.Is(err, cosutil.ErrObjectMissing) {
			t.Errorf("Head(%q) = %v, want ErrObjectMissing", key, err)
		}
	}
	if !validKey(testKey) {
		t.Fatalf("validKey(%q) = false", testKey)
	}
}

func TestSecretPersistsAcrossRestart(t *testing.T) {
	root := t.TempDir()
	first, err := New(Options{Root: root})
	if err != nil {
		t.Fatalf("new first: %v", err)
	}
	second, err := New(Options{Root: root})
	if err != nil {
		t.Fatalf("new second: %v", err)
	}
	if string(first.secret) != string(second.secret) {
		t.Fatal("secret should persist across restarts")
	}
}

// ListObjects 只收录普通对象文件，跳过上传临时文件与系统文件；OpenObject 可读回内容。
func TestListAndOpenObjects(t *testing.T) {
	root := t.TempDir()
	s, err := New(Options{Root: root})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	writeFile := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	writeFile("trips/t1/a.jpg", "jpeg")
	writeFile(".upload-tmp", "partial")
	writeFile(".DS_Store", "junk")
	writeFile("trips/t1/.tmp-keep", "temp")
	writeFile("keepsake-backup-20260101-000000.zip", "old-backup")

	objects, err := s.ListObjects(context.Background())
	if err != nil {
		t.Fatalf("ListObjects: %v", err)
	}
	got := make(map[string]int64, len(objects))
	for _, o := range objects {
		got[o.Key] = o.Size
	}
	if _, ok := got["trips/t1/a.jpg"]; !ok {
		t.Fatalf("missing uploaded object: %v", got)
	}
	if _, ok := got[".secret"]; !ok {
		t.Fatalf("root .secret should be included: %v", got)
	}
	for _, unwanted := range []string{".upload-tmp", ".DS_Store", "trips/t1/.tmp-keep", "keepsake-backup-20260101-000000.zip"} {
		if _, ok := got[unwanted]; ok {
			t.Fatalf("unexpected object %q in %v", unwanted, got)
		}
	}

	rc, err := s.OpenObject(context.Background(), "trips/t1/a.jpg")
	if err != nil {
		t.Fatalf("OpenObject: %v", err)
	}
	defer func() { _ = rc.Close() }()
	body, err := io.ReadAll(rc)
	if err != nil || string(body) != "jpeg" {
		t.Fatalf("body = %q err=%v", body, err)
	}
	if _, err := s.OpenObject(context.Background(), "../secret"); err == nil {
		t.Fatal("invalid key should be rejected")
	}
}
