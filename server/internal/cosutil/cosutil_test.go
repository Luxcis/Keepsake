package cosutil

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

// 预签名在本地完成（不发网络请求），用假密钥即可验证 URL 结构。
func TestSignProcessedURLs(t *testing.T) {
	c := New("AKIDtest", "secret", "ap-guangzhou", "keepsake-1250000000", 15*time.Minute)

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
