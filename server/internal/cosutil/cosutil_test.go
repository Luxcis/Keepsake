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

// 未配置源站时保持原有行为：使用 COS 默认域名。
func TestDefaultBucketURL(t *testing.T) {
	c := newTestClient(t, "")

	u := c.core.BaseURL.BucketURL
	if u.Scheme != "https" {
		t.Fatalf("scheme = %q, want https", u.Scheme)
	}
	if want := "keepsake-1250000000.cos.ap-guangzhou.myqcloud.com"; u.Host != want {
		t.Fatalf("host = %q, want %q", u.Host, want)
	}
}

// 自定义源站域名同时作用于 SDK 请求地址与预签名 URL，且 Host 参与签名。
func TestCustomOrigin(t *testing.T) {
	const origin = "photos.example.com"
	c := newTestClient(t, origin)

	bu := c.core.BaseURL.BucketURL
	if bu.Scheme != "https" || bu.Host != origin {
		t.Fatalf("bucket url = %s, want https://%s", bu, origin)
	}

	key := "trips/t1/202608/a.jpg"
	signed, err := c.SignGETPreview(key)
	if err != nil {
		t.Fatalf("SignGETPreview: %v", err)
	}
	u, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("parse preview url: %v", err)
	}
	if u.Host != origin {
		t.Fatalf("signed host = %q, want %q", u.Host, origin)
	}
	if got := u.Query().Get("q-header-list"); got != "host" {
		t.Fatalf("q-header-list = %q, want host (custom origin must be signed)", got)
	}

	// Host 是签名串的一部分：换源站后同一对象的签名必须不同。
	defSigned, err := newTestClient(t, "").SignGETPreview(key)
	if err != nil {
		t.Fatalf("default SignGETPreview: %v", err)
	}
	du, _ := url.Parse(defSigned)
	if u.Query().Get("q-signature") == du.Query().Get("q-signature") {
		t.Fatal("signature must differ when the origin host differs")
	}
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
			u := c.core.BaseURL.BucketURL
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
