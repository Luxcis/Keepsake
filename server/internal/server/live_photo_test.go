package server_test

// 实况照片（Live Photo）上传链路：签名（图片 + 视频）→ 直传 → confirm 关联 →
// 列表签名 video_src → 删除补偿。使用本地存储后端走真实 HTTP 路径。

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"keepsake/internal/config"
	"keepsake/internal/localstore"
)

// liveDeps 准备一个带本地存储后端的完整服务，并登录管理员。
func liveDeps(t *testing.T) *deps {
	t.Helper()
	ls, err := localstore.New(localstore.Options{
		Root:     t.TempDir(),
		Secret:   "live-photo-test-secret",
		MaxBytes: config.MaxUploadBytes,
	})
	if err != nil {
		t.Fatalf("localstore: %v", err)
	}
	d := newStackWithBackend(t, ls)
	d.do(t, "POST", "/api/auth/login", `{"username":"admin","password":"secret123"}`).
		expectStatus(t, http.StatusOK)
	return d
}

func (d *deps) createTrip(t *testing.T) string {
	t.Helper()
	r := d.do(t, "POST", "/api/admin/trips",
		`{"title":"实况测试","summary":"","start_date":"2026-08-01","status":"draft"}`)
	r.expectStatus(t, http.StatusCreated)
	data, _ := r.decode(t)["data"].(map[string]any)
	id, _ := data["id"].(string)
	if id == "" {
		t.Fatalf("no trip id: %s", r.body)
	}
	return id
}

// uploadObject 走签名 + 直传，返回服务端生成的对象 Key。
func (d *deps) uploadObject(t *testing.T, tripID, contentType string, body []byte) string {
	t.Helper()
	r := d.do(t, "POST", "/api/admin/uploads/sign",
		fmt.Sprintf(`{"trip_id":%q,"content_type":%q,"size":%d}`, tripID, contentType, len(body)))
	r.expectStatus(t, http.StatusOK)
	data, _ := r.decode(t)["data"].(map[string]any)
	key, _ := data["cos_key"].(string)
	uploadURL, _ := data["upload_url"].(string)
	if key == "" || uploadURL == "" {
		t.Fatalf("sign response missing key/url: %s", r.body)
	}

	req, err := http.NewRequest(http.MethodPut, d.base+uploadURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build upload: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	res, err := d.http.Do(req)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("upload status = %d, body=%s", res.StatusCode, raw)
	}
	return key
}

func (d *deps) listAdminPhotos(t *testing.T, tripID string) []any {
	t.Helper()
	r := d.do(t, "GET", "/api/admin/trips/"+tripID+"/photos", "")
	r.expectStatus(t, http.StatusOK)
	data, _ := r.decode(t)["data"].(map[string]any)
	photos, _ := data["photos"].([]any)
	return photos
}

func TestLivePhotoUploadFlow(t *testing.T) {
	d := liveDeps(t)
	tripID := d.createTrip(t)

	imageBytes := []byte("\xff\xd8\xff\xe0live-photo-still")
	videoBytes := []byte("live-photo-motion-mp4")
	imageKey := d.uploadObject(t, tripID, "image/jpeg", imageBytes)
	videoKey := d.uploadObject(t, tripID, "video/mp4", videoBytes)
	if !strings.HasPrefix(videoKey, "trips/"+tripID+"/") || !strings.HasSuffix(videoKey, ".mp4") {
		t.Fatalf("video key = %s", videoKey)
	}

	r := d.do(t, "POST", "/api/admin/photos/confirm",
		fmt.Sprintf(`{"trip_id":%q,"cos_key":%q,"video_key":%q,"title":"实况照片"}`, tripID, imageKey, videoKey))
	r.expectStatus(t, http.StatusCreated)
	photoID, _ := r.decode(t)["data"].(map[string]any)["id"].(string)

	photos := d.listAdminPhotos(t, tripID)
	if len(photos) != 1 {
		t.Fatalf("photos = %d, want 1: %v", len(photos), photos)
	}
	photo, _ := photos[0].(map[string]any)
	videoSrc, _ := photo["video_src"].(string)
	if videoSrc == "" {
		t.Fatalf("video_src empty: %v", photo)
	}

	// 签名后的视频 URL 可以直接读取，类型与上传时一致。
	req, err := http.NewRequest(http.MethodGet, d.base+videoSrc, nil)
	if err != nil {
		t.Fatalf("build video get: %v", err)
	}
	res, err := d.http.Do(req)
	if err != nil {
		t.Fatalf("get video: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("video get status = %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "video/mp4" {
		t.Fatalf("video content-type = %s", ct)
	}
	got, _ := io.ReadAll(res.Body)
	if !bytes.Equal(got, videoBytes) {
		t.Fatalf("video body mismatch: %q", got)
	}

	// 删除后图片与视频各入队一条补偿任务。
	d.do(t, "DELETE", "/api/admin/photos/"+photoID, "").expectStatus(t, http.StatusOK)
	var pending int
	if err := d.st.DB.QueryRow(
		`SELECT COUNT(*) FROM cos_delete_tasks WHERE done_at IS NULL`).Scan(&pending); err != nil {
		t.Fatalf("count delete tasks: %v", err)
	}
	if pending != 2 {
		t.Fatalf("pending delete tasks = %d, want 2 (image + video)", pending)
	}
}

func TestPlainPhotoKeepsSingleObject(t *testing.T) {
	d := liveDeps(t)
	tripID := d.createTrip(t)

	imageKey := d.uploadObject(t, tripID, "image/jpeg", []byte("\xff\xd8\xff\xe0plain"))
	r := d.do(t, "POST", "/api/admin/photos/confirm",
		fmt.Sprintf(`{"trip_id":%q,"cos_key":%q,"title":"普通照片"}`, tripID, imageKey))
	r.expectStatus(t, http.StatusCreated)
	photoID, _ := r.decode(t)["data"].(map[string]any)["id"].(string)

	photos := d.listAdminPhotos(t, tripID)
	if len(photos) != 1 {
		t.Fatalf("photos = %d, want 1", len(photos))
	}
	photo, _ := photos[0].(map[string]any)
	if src, _ := photo["video_src"].(string); src != "" {
		t.Fatalf("plain photo video_src = %q, want empty", src)
	}

	d.do(t, "DELETE", "/api/admin/photos/"+photoID, "").expectStatus(t, http.StatusOK)
	var pending int
	if err := d.st.DB.QueryRow(
		`SELECT COUNT(*) FROM cos_delete_tasks WHERE done_at IS NULL`).Scan(&pending); err != nil {
		t.Fatalf("count delete tasks: %v", err)
	}
	if pending != 1 {
		t.Fatalf("pending delete tasks = %d, want 1 (image only)", pending)
	}
}

func TestVideoKeyValidation(t *testing.T) {
	d := liveDeps(t)
	tripID := d.createTrip(t)
	otherTripID := d.createTrip(t)

	imageKey := d.uploadObject(t, tripID, "image/jpeg", []byte("\xff\xd8\xff\xe0still"))
	otherVideoKey := d.uploadObject(t, otherTripID, "video/mp4", []byte("other-video"))

	cases := []struct {
		name     string
		videoKey string
	}{
		{"跨旅行前缀", otherVideoKey},
		{"图片对象冒充视频", imageKey},
		{"对象不存在", "trips/" + tripID + "/202611/not-uploaded.mp4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := d.do(t, "POST", "/api/admin/photos/confirm",
				fmt.Sprintf(`{"trip_id":%q,"cos_key":%q,"video_key":%q,"title":"x"}`, tripID, imageKey, tc.videoKey))
			r.expectStatus(t, http.StatusBadRequest)
		})
	}

	// 白名单外的类型不能签名；video/quicktime 与 mp4 一样可用。
	d.do(t, "POST", "/api/admin/uploads/sign",
		fmt.Sprintf(`{"trip_id":%q,"content_type":"text/plain","size":10}`, tripID)).
		expectStatus(t, http.StatusBadRequest)
	r := d.do(t, "POST", "/api/admin/uploads/sign",
		fmt.Sprintf(`{"trip_id":%q,"content_type":"video/quicktime","size":10}`, tripID))
	r.expectStatus(t, http.StatusOK)
	if key, _ := r.decode(t)["data"].(map[string]any)["cos_key"].(string); !strings.HasSuffix(key, ".mov") {
		t.Fatalf("quicktime key = %s", key)
	}

	// 上述失败请求都不应创建照片记录。
	if photos := d.listAdminPhotos(t, tripID); len(photos) != 0 {
		t.Fatalf("photos = %d, want 0 after rejected confirms", len(photos))
	}
}

func TestQuickTimeVideoConfirmed(t *testing.T) {
	d := liveDeps(t)
	tripID := d.createTrip(t)

	imageKey := d.uploadObject(t, tripID, "image/jpeg", []byte("\xff\xd8\xff\xe0still-mov"))
	videoKey := d.uploadObject(t, tripID, "video/quicktime", []byte("live-photo-motion-mov"))
	if !strings.HasSuffix(videoKey, ".mov") {
		t.Fatalf("quicktime key = %s", videoKey)
	}

	r := d.do(t, "POST", "/api/admin/photos/confirm",
		fmt.Sprintf(`{"trip_id":%q,"cos_key":%q,"video_key":%q,"title":"MOV 实况"}`, tripID, imageKey, videoKey))
	r.expectStatus(t, http.StatusCreated)

	photos := d.listAdminPhotos(t, tripID)
	if len(photos) != 1 {
		t.Fatalf("photos = %d, want 1", len(photos))
	}
	photo, _ := photos[0].(map[string]any)
	if src, _ := photo["video_src"].(string); src == "" {
		t.Fatalf("video_src empty for quicktime video: %v", photo)
	}
}
