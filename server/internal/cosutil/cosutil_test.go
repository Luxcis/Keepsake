package cosutil

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, origin string) *Client {
	t.Helper()
	c, err := New(Options{
		SecretID:   "AKIDtest",
		SecretKey:  "secret",
		Region:     "ap-guangzhou",
		Bucket:     "keepsake-1250000000",
		Origin:     origin,
		URLExpires: 15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("New(origin=%q): %v", origin, err)
	}
	return c
}

// 预签名在本地完成（不发网络请求），用假密钥即可验证 URL 结构。
func TestSignProcessedURLs(t *testing.T) {
	c := newTestClient(t, "")

	key := "trips/t1/202608/a.jpg"
	thumb, err := c.SignGETThumb(key)
	if err != nil {
		t.Fatalf("SignGETThumb: %v", err)
	}
	preview, err := c.SignGETPreview(key)
	if err != nil {
		t.Fatalf("SignGETPreview: %v", err)
	}

	u, err := url.Parse(preview)
	if err != nil {
		t.Fatalf("parse preview url: %v", err)
	}
	// 处理参数必须作为签名 query 的一部分出现，且落在对象路径上。
	if u.Path != "/"+key {
		t.Fatalf("preview path = %q, want %q", u.Path, "/"+key)
	}
	if _, ok := u.Query()[previewOp]; !ok {
		t.Fatalf("preview url missing signed op %q: %s", previewOp, preview)
	}
	if thumb == preview {
		t.Fatal("thumb and preview must differ (different processing params)")
	}
	if !strings.Contains(previewOp, "2400x2400>") {
		t.Fatalf("previewOp should cap both sides and never upscale, got %q", previewOp)
	}

	other, err := c.SignGETPreview("trips/t1/202608/b.jpg")
	if err != nil {
		t.Fatalf("SignGETPreview other key: %v", err)
	}
	ou, _ := url.Parse(other)
	if ou.Path == u.Path {
		t.Fatal("different object keys must sign different paths")
	}
}

// 未配置源站时保持原有行为：读写都使用 COS 默认域名。
func TestDefaultBucketURL(t *testing.T) {
	c := newTestClient(t, "")

	for name, u := range map[string]*url.URL{"core": c.core.BaseURL.BucketURL, "read": c.read.BaseURL.BucketURL} {
		if u.Scheme != "https" {
			t.Fatalf("%s scheme = %q, want https", name, u.Scheme)
		}
		if want := "keepsake-1250000000.cos.ap-guangzhou.myqcloud.com"; u.Host != want {
			t.Fatalf("%s host = %q, want %q", name, u.Host, want)
		}
	}
}

// 自定义源站只接管读取：GET 预签名指向源站并签入 Host；
// 上传 PUT（以及 HEAD/DELETE 使用的 core）仍是 COS 默认域名。
func TestCustomOriginForReadsOnly(t *testing.T) {
	const origin = "photos.example.com"
	const defaultHost = "keepsake-1250000000.cos.ap-guangzhou.myqcloud.com"
	c := newTestClient(t, origin)

	if got := c.core.BaseURL.BucketURL.Host; got != defaultHost {
		t.Fatalf("core host = %q, want default %q", got, defaultHost)
	}
	if got := c.read.BaseURL.BucketURL.Host; got != origin {
		t.Fatalf("read host = %q, want %q", got, origin)
	}

	key := "trips/t1/202608/a.jpg"
	readURLs := map[string]string{}
	for name, sign := range map[string]func(string) (string, error){
		"get":     c.SignGET,
		"thumb":   c.SignGETThumb,
		"preview": c.SignGETPreview,
	} {
		signed, err := sign(key)
		if err != nil {
			t.Fatalf("%s sign: %v", name, err)
		}
		readURLs[name] = signed
		u, err := url.Parse(signed)
		if err != nil {
			t.Fatalf("%s parse: %v", name, err)
		}
		if u.Host != origin {
			t.Fatalf("%s host = %q, want %q", name, u.Host, origin)
		}
	}
	if got := mustQuery(t, readURLs["preview"]).Get("q-header-list"); got != "host" {
		t.Fatalf("q-header-list = %q, want host (custom origin must be signed)", got)
	}

	uploadURL, _, err := c.SignPUT(key, "image/jpeg")
	if err != nil {
		t.Fatalf("SignPUT: %v", err)
	}
	if u := mustParse(t, uploadURL); u.Host != defaultHost {
		t.Fatalf("upload host = %q, want default %q", u.Host, defaultHost)
	}

	// Host 是签名串的一部分：读取换源站后同一对象的签名必须与默认域名不同。
	def, err := newTestClient(t, "").SignGETPreview(key)
	if err != nil {
		t.Fatalf("default SignGETPreview: %v", err)
	}
	if mustQuery(t, def).Get("q-signature") == mustQuery(t, readURLs["preview"]).Get("q-signature") {
		t.Fatal("signature must differ when the read origin host differs")
	}
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}

func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	return mustParse(t, raw).Query()
}

func TestCustomOriginNormalization(t *testing.T) {
	tests := []struct {
		name     string
		origin   string
		wantHost string
	}{
		{"裸域名补 https", "photos.example.com", "photos.example.com"},
		{"完整 URL 去掉末尾斜杠", "https://photos.example.com/", "photos.example.com"},
		{"主机名统一小写", "HTTPS://Photos.Example.COM", "photos.example.com"},
		{"保留端口", "http://127.0.0.1:9000", "127.0.0.1:9000"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, tc.origin)
			u := c.read.BaseURL.BucketURL
			if u.Host != tc.wantHost {
				t.Fatalf("host = %q, want %q", u.Host, tc.wantHost)
			}
			if u.Path != "" {
				t.Fatalf("path = %q, want empty (origin has no path prefix)", u.Path)
			}
		})
	}
}

func TestInvalidCustomOrigin(t *testing.T) {
	for _, origin := range []string{
		"https://photos.example.com/prefix",
		"ftp://photos.example.com",
		"https://user:pass@photos.example.com",
		"https://photos.example.com?x=1",
		"https://",
	} {
		t.Run(origin, func(t *testing.T) {
			_, err := New(Options{
				SecretID:   "AKIDtest",
				SecretKey:  "secret",
				Region:     "ap-guangzhou",
				Bucket:     "keepsake-1250000000",
				Origin:     origin,
				URLExpires: 15 * time.Minute,
			})
			if err == nil {
				t.Fatalf("New(origin=%q) should fail", origin)
			}
		})
	}
}
