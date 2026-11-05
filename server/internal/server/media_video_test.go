package server_test

// 独立视频条目：签名上限按媒体类型区分（视频 400MB / 图片 30MB）→ 视频作为主对象确认入库
// → 列表输出 media_kind 与 video_src → 删除时同步清理视频对象。
// 视频对象超过图片上限的部分用稀疏文件直接落盘，避免测试搬运数百 MB。

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"keepsake/internal/config"
	"keepsake/internal/store"
)

// seedObject 在本地存储根目录创建一个指定大小的稀疏对象，模拟已直传完成的视频/图片。
func (d *deps) seedObject(t *testing.T, tripID, ext string, size int64) string {
	t.Helper()
	key := fmt.Sprintf("trips/%s/202611/%s.%s", tripID, store.NewUUID(), ext)
	p := filepath.Join(d.storageRoot, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir object dir: %v", err)
	}
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("create object: %v", err)
	}
	if err := f.Truncate(size); err != nil {
		f.Close()
		t.Fatalf("truncate object: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close object: %v", err)
	}
	return key
}

// TestVideoSignLimits：签名接口按内容类型选择上限，并在响应中回传对应 max_bytes。
func TestVideoSignLimits(t *testing.T) {
	d := liveDeps(t)
	tripID := d.createTrip(t)

	cases := []struct {
		name        string
		contentType string
		size        int64
		wantStatus  int
		wantMax     int64
	}{
		{"视频超过图片上限仍可签名", "video/mp4", 31 << 20, http.StatusOK, config.MaxVideoUploadBytes},
		{"视频恰好 400MB", "video/mp4", 400 << 20, http.StatusOK, config.MaxVideoUploadBytes},
		{"视频超过 400MB", "video/mp4", 400<<20 + 1, http.StatusBadRequest, 0},
		{"quicktime 400MB", "video/quicktime", 400 << 20, http.StatusOK, config.MaxVideoUploadBytes},
		{"图片恰好 30MB", "image/jpeg", 30 << 20, http.StatusOK, config.MaxUploadBytes},
		{"图片超过 30MB", "image/jpeg", 30<<20 + 1, http.StatusBadRequest, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := d.do(t, "POST", "/api/admin/uploads/sign",
				fmt.Sprintf(`{"trip_id":%q,"content_type":%q,"size":%d}`, tripID, tc.contentType, tc.size))
			r.expectStatus(t, tc.wantStatus)
			if tc.wantStatus != http.StatusOK {
				return
			}
			data, _ := r.decode(t)["data"].(map[string]any)
			if got, _ := data["max_bytes"].(float64); int64(got) != tc.wantMax {
				t.Fatalf("max_bytes = %v, want %d", data["max_bytes"], tc.wantMax)
			}
		})
	}
}

// TestStandaloneVideoUploadFlow：单独上传的视频作为主对象入库，列表输出 media_kind=video，
// video_src 可读取；删除时同步清理对象且不残留补偿任务。
func TestStandaloneVideoUploadFlow(t *testing.T) {
	d := liveDeps(t)
	tripID := d.createTrip(t)

	videoBytes := []byte("standalone-video-mp4")
	videoKey := d.uploadObject(t, tripID, "video/mp4", videoBytes)

	r := d.do(t, "POST", "/api/admin/photos/confirm",
		fmt.Sprintf(`{"trip_id":%q,"cos_key":%q,"title":"旅行短片"}`, tripID, videoKey))
	r.expectStatus(t, http.StatusCreated)
	mediaID, _ := r.decode(t)["data"].(map[string]any)["id"].(string)
	if mediaID == "" {
		t.Fatalf("confirm returned no id")
	}

	photos := d.listAdminPhotos(t, tripID)
	if len(photos) != 1 {
		t.Fatalf("photos = %d, want 1", len(photos))
	}
	item, _ := photos[0].(map[string]any)
	if kind, _ := item["media_kind"].(string); kind != "video" {
		t.Fatalf("media_kind = %q, want video", kind)
	}
	for _, field := range []string{"src", "preview_src", "thumb_src"} {
		if v, _ := item[field].(string); v != "" {
			t.Fatalf("%s = %q, want empty for video", field, v)
		}
	}
	videoSrc, _ := item["video_src"].(string)
	if videoSrc == "" {
		t.Fatalf("video_src empty for standalone video: %v", item)
	}

	res, err := d.http.Get(d.base + videoSrc)
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
	if string(got) != string(videoBytes) {
		t.Fatalf("video body mismatch: %q", got)
	}

	// 视频条目不能再携带 video_key（那是实况照片的关联字段）。
	imageKey := d.uploadObject(t, tripID, "image/jpeg", []byte("\xff\xd8\xff\xe0plain"))
	d.do(t, "POST", "/api/admin/photos/confirm",
		fmt.Sprintf(`{"trip_id":%q,"cos_key":%q,"video_key":%q,"title":"x"}`, tripID, videoKey, imageKey)).
		expectStatus(t, http.StatusBadRequest)

	// 视频不能用作旅行封面。
	d.do(t, "PUT", "/api/admin/trips/"+tripID,
		fmt.Sprintf(`{"title":"单独视频","start_date":"2026-08-01","cover_photo_id":%q}`, mediaID)).
		expectStatus(t, http.StatusBadRequest)

	d.do(t, "DELETE", "/api/admin/photos/"+mediaID, "").expectStatus(t, http.StatusOK)
	d.assertObjectGone(t, videoKey)
	var pending int
	if err := d.st.DB.QueryRow(
		`SELECT COUNT(*) FROM cos_delete_tasks WHERE done_at IS NULL`).Scan(&pending); err != nil {
		t.Fatalf("count delete tasks: %v", err)
	}
	if pending != 0 {
		t.Fatalf("pending delete tasks = %d, want 0 (deleted synchronously)", pending)
	}
}

// TestConfirmObjectSizeByMediaKind：确认阶段同样按类型校验大小——
// 视频可用超过 30MB 的对象，图片仍卡在 30MB，两者都不能超过 400MB。
func TestConfirmObjectSizeByMediaKind(t *testing.T) {
	d := liveDeps(t)
	tripID := d.createTrip(t)

	bigVideo := d.seedObject(t, tripID, "mp4", 31<<20)
	d.do(t, "POST", "/api/admin/photos/confirm",
		fmt.Sprintf(`{"trip_id":%q,"cos_key":%q,"title":"31MB 视频"}`, tripID, bigVideo)).
		expectStatus(t, http.StatusCreated)

	bigImage := d.seedObject(t, tripID, "jpg", 31<<20)
	d.do(t, "POST", "/api/admin/photos/confirm",
		fmt.Sprintf(`{"trip_id":%q,"cos_key":%q,"title":"31MB 图片"}`, tripID, bigImage)).
		expectStatus(t, http.StatusBadRequest)

	overVideo := d.seedObject(t, tripID, "mp4", 400<<20+1)
	d.do(t, "POST", "/api/admin/photos/confirm",
		fmt.Sprintf(`{"trip_id":%q,"cos_key":%q,"title":"超限视频"}`, tripID, overVideo)).
		expectStatus(t, http.StatusBadRequest)

	// 只有第一条 31MB 视频入库。
	photos := d.listAdminPhotos(t, tripID)
	if len(photos) != 1 {
		t.Fatalf("photos = %d, want 1: %v", len(photos), photos)
	}
	if kind, _ := photos[0].(map[string]any)["media_kind"].(string); kind != "video" {
		t.Fatalf("media_kind = %q, want video", kind)
	}
}

// TestLivePhotoVideoSizeLimit：实况照片的 video_key 同样放宽到 400MB，超过 400MB 仍拒绝。
func TestLivePhotoVideoSizeLimit(t *testing.T) {
	d := liveDeps(t)
	tripID := d.createTrip(t)

	imageKey := d.uploadObject(t, tripID, "image/jpeg", []byte("\xff\xd8\xff\xe0still"))
	bigVideo := d.seedObject(t, tripID, "mov", 31<<20)
	d.do(t, "POST", "/api/admin/photos/confirm",
		fmt.Sprintf(`{"trip_id":%q,"cos_key":%q,"video_key":%q,"title":"大实况"}`, tripID, imageKey, bigVideo)).
		expectStatus(t, http.StatusCreated)

	photos := d.listAdminPhotos(t, tripID)
	if len(photos) != 1 {
		t.Fatalf("photos = %d, want 1", len(photos))
	}
	if src, _ := photos[0].(map[string]any)["video_src"].(string); src == "" {
		t.Fatalf("video_src empty for 31MB live video")
	}

	overVideo := d.seedObject(t, tripID, "mp4", 400<<20+1)
	imageKey2 := d.uploadObject(t, tripID, "image/jpeg", []byte("\xff\xd8\xff\xe0still2"))
	d.do(t, "POST", "/api/admin/photos/confirm",
		fmt.Sprintf(`{"trip_id":%q,"cos_key":%q,"video_key":%q,"title":"超限实况"}`, tripID, imageKey2, overVideo)).
		expectStatus(t, http.StatusBadRequest)

	if photos := d.listAdminPhotos(t, tripID); len(photos) != 1 {
		t.Fatalf("photos = %d, want 1 after rejected live video", len(photos))
	}
}

func TestVideoLimitsAreConfigured(t *testing.T) {
	if config.MaxVideoUploadBytes>>20 != 400 {
		t.Fatalf("MaxVideoUploadBytes = %d bytes, want 400MB", config.MaxVideoUploadBytes)
	}
	if config.MaxUploadBytes>>20 != 30 {
		t.Fatalf("MaxUploadBytes = %d bytes, want 30MB", config.MaxUploadBytes)
	}
	if config.MaxMediaUploadBytes != config.MaxVideoUploadBytes {
		t.Fatalf("MaxMediaUploadBytes = %d, want video limit %d",
			config.MaxMediaUploadBytes, config.MaxVideoUploadBytes)
	}
}
