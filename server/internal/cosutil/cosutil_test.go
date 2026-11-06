package cosutil

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, cdn string) *Client {
	t.Helper()
	c, err := New(Options{
		SecretID:   "AKIDtest",
		SecretKey:  "secret",
		Region:     "ap-guangzhou",
		Bucket:     "keepsake-1250000000",
		CDNDomain:  cdn,
		URLExpires: 15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("New(cdn=%q): %v", cdn, err)
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

// 未配置 CDN 时保持私有桶行为：读取仍使用 COS 默认域名的短时效签名 URL。
func TestNoCDNKeepsSignedReads(t *testing.T) {
	c := newTestClient(t, "")

	u := c.core.BaseURL.BucketURL
	if u.Scheme != "https" {
		t.Fatalf("scheme = %q, want https", u.Scheme)
	}
	if want := "keepsake-1250000000.cos.ap-guangzhou.myqcloud.com"; u.Host != want {
		t.Fatalf("host = %q, want %q", u.Host, want)
	}
	if c.cdn != nil {
		t.Fatalf("cdn = %v, want nil", c.cdn)
	}

	signed, err := c.SignGET("trips/t1/202608/a.jpg")
	if err != nil {
		t.Fatalf("SignGET: %v", err)
	}
	q := mustQuery(t, signed)
	if q.Get("q-signature") == "" || q.Get("q-header-list") != "host" {
		t.Fatalf("read url should be signed against COS host: %s", signed)
	}
}

// 配置 CDN 后读取不再签名：原图/缩略图/灯箱直接指向 CDN；
// 上传 PUT（以及 HEAD/DELETE 使用的 core）仍是 COS 默认域名。
func TestCDNReadsAreUnsigned(t *testing.T) {
	const cdn = "photos.example.com"
	const defaultHost = "keepsake-1250000000.cos.ap-guangzhou.myqcloud.com"
	c := newTestClient(t, cdn)

	if got := c.core.BaseURL.BucketURL.Host; got != defaultHost {
		t.Fatalf("core host = %q, want default %q", got, defaultHost)
	}
	if c.cdn == nil || c.cdn.Host != cdn {
		t.Fatalf("cdn = %v, want host %q", c.cdn, cdn)
	}

	key := "trips/t1/202608/a.jpg"
	readURLs := map[string]string{}
	for name, read := range map[string]func(string) (string, error){
		"get":     c.SignGET,
		"thumb":   c.SignGETThumb,
		"preview": c.SignGETPreview,
	} {
		raw, err := read(key)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		readURLs[name] = raw
		u := mustParse(t, raw)
		if u.Host != cdn {
			t.Fatalf("%s host = %q, want %q", name, u.Host, cdn)
		}
		if u.Path != "/"+key {
			t.Fatalf("%s path = %q, want %q", name, u.Path, "/"+key)
		}
		for _, signedParam := range []string{
			"q-signature", "q-sign-time", "q-key-time", "q-header-list", "q-url-param-list",
		} {
			if got := u.Query().Get(signedParam); got != "" {
				t.Fatalf("%s url unexpectedly carries %s=%q: %s", name, signedParam, got, raw)
			}
		}
	}

	if want := "https://" + cdn + "/" + key; readURLs["get"] != want {
		t.Fatalf("get url = %q, want %q (no query)", readURLs["get"], want)
	}
	// 处理参数保持 COS/CDN 约定的字面量斜杠形式；`>` 按 URL 规范转义为 %3E。
	if want := "https://" + cdn + "/" + key + "?" + thumbOp; readURLs["thumb"] != want {
		t.Fatalf("thumb url = %q, want %q", readURLs["thumb"], want)
	}
	wantPreviewOp := strings.ReplaceAll(previewOp, ">", "%3E")
	if want := "https://" + cdn + "/" + key + "?" + wantPreviewOp; readURLs["preview"] != want {
		t.Fatalf("preview url = %q, want %q", readURLs["preview"], want)
	}
	if _, ok := mustQuery(t, readURLs["thumb"])[thumbOp]; !ok {
		t.Fatalf("thumb url missing op %q: %s", thumbOp, readURLs["thumb"])
	}
	if _, ok := mustQuery(t, readURLs["preview"])[previewOp]; !ok {
		t.Fatalf("preview url missing op %q: %s", previewOp, readURLs["preview"])
	}
	if readURLs["thumb"] == readURLs["preview"] {
		t.Fatal("thumb and preview must differ (different processing params)")
	}

	// 无签名 URL 不随时间变化，便于浏览器/CDN 缓存。
	again, err := c.SignGETPreview(key)
	if err != nil {
		t.Fatalf("SignGETPreview again: %v", err)
	}
	if again != readURLs["preview"] {
		t.Fatalf("CDN read urls should be deterministic: %q != %q", again, readURLs["preview"])
	}

	uploadURL, _, err := c.SignPUT(key, "image/jpeg")
	if err != nil {
		t.Fatalf("SignPUT: %v", err)
	}
	if u := mustParse(t, uploadURL); u.Host != defaultHost {
		t.Fatalf("upload host = %q, want default %q", u.Host, defaultHost)
	}
	if mustQuery(t, uploadURL).Get("q-signature") == "" {
		t.Fatalf("upload url must stay signed: %s", uploadURL)
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

func TestCDNDomainNormalization(t *testing.T) {
	tests := []struct {
		name     string
		cdn      string
		wantHost string
	}{
		{"裸域名补 https", "photos.example.com", "photos.example.com"},
		{"完整 URL 去掉末尾斜杠", "https://photos.example.com/", "photos.example.com"},
		{"主机名统一小写", "HTTPS://Photos.Example.COM", "photos.example.com"},
		{"保留端口", "http://127.0.0.1:9000", "127.0.0.1:9000"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, tc.cdn)
			u := c.cdn
			if u == nil {
				t.Fatal("cdn url is nil")
			}
			if u.Host != tc.wantHost {
				t.Fatalf("host = %q, want %q", u.Host, tc.wantHost)
			}
			if u.Path != "" {
				t.Fatalf("path = %q, want empty (cdn domain has no path prefix)", u.Path)
			}
		})
	}
}

func TestInvalidCDNDomain(t *testing.T) {
	for _, cdn := range []string{
		"https://photos.example.com/prefix",
		"ftp://photos.example.com",
		"https://user:pass@photos.example.com",
		"https://photos.example.com?x=1",
		"https://",
	} {
		t.Run(cdn, func(t *testing.T) {
			_, err := New(Options{
				SecretID:   "AKIDtest",
				SecretKey:  "secret",
				Region:     "ap-guangzhou",
				Bucket:     "keepsake-1250000000",
				CDNDomain:  cdn,
				URLExpires: 15 * time.Minute,
			})
			if err == nil {
				t.Fatalf("New(cdn=%q) should fail", cdn)
			}
		})
	}
}
