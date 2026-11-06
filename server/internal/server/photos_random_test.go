package server_test

// 首页随机照片：门禁鉴权、随机池范围（仅已发布旅行的照片，排除草稿与视频）与 limit 边界。
// 使用本地存储后端走真实签名链路，断言返回地址可直接读取。

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"keepsake/internal/store"
)

// seedPhoto 走签名 + 直传 + confirm，返回新建照片 ID。
func (d *deps) seedPhoto(t *testing.T, tripID, title string) string {
	t.Helper()
	key := d.uploadObject(t, tripID, "image/jpeg", []byte("\xff\xd8\xff\xe0random-photo-"+title))
	r := d.do(t, "POST", "/api/admin/photos/confirm",
		fmt.Sprintf(`{"trip_id":%q,"cos_key":%q,"title":%q}`, tripID, key, title))
	r.expectStatus(t, http.StatusCreated)
	id, _ := r.decode(t)["data"].(map[string]any)["id"].(string)
	if id == "" {
		t.Fatalf("confirm photo %s: no id in %s", title, r.body)
	}
	return id
}

// seedVideo 走签名 + 直传 + confirm 创建独立视频条目（媒体类型由 HEAD 判定）。
func (d *deps) seedVideo(t *testing.T, tripID string) string {
	t.Helper()
	key := d.uploadObject(t, tripID, "video/mp4", []byte("random-photo-standalone-video"))
	r := d.do(t, "POST", "/api/admin/photos/confirm",
		fmt.Sprintf(`{"trip_id":%q,"cos_key":%q,"title":"视频"}`, tripID, key))
	r.expectStatus(t, http.StatusCreated)
	id, _ := r.decode(t)["data"].(map[string]any)["id"].(string)
	if id == "" {
		t.Fatalf("confirm video: no id in %s", r.body)
	}
	return id
}

// publishTrip 把草稿旅行改为已发布。
func (d *deps) publishTrip(t *testing.T, tripID, title string) {
	t.Helper()
	r := d.do(t, "PUT", "/api/admin/trips/"+tripID,
		fmt.Sprintf(`{"title":%q,"summary":"","start_date":"2026-08-01","status":"published"}`, title))
	r.expectStatus(t, http.StatusOK)
}

// randomPhotos 请求随机照片接口并返回照片数组与 enabled 标记。
func randomPhotos(t *testing.T, d *deps, query string) ([]any, bool) {
	t.Helper()
	r := d.do(t, "GET", "/api/photos/random"+query, "")
	r.expectStatus(t, http.StatusOK)
	data, _ := r.decode(t)["data"].(map[string]any)
	photos, _ := data["photos"].([]any)
	enabled, _ := data["enabled"].(bool)
	return photos, enabled
}

// visitorClient 返回没有管理员会话的独立客户端，用于验证门禁语义
// （管理员会话按设计可绕过门禁，不能用来断言未解锁状态）。
func visitorClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return &http.Client{Jar: jar}
}

// doWith 用指定客户端发起请求，便于区分管理员与访客会话。
func (d *deps) doWith(t *testing.T, client *http.Client, method, path, body string) apiResult {
	t.Helper()
	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, d.base+path, reqBody)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return apiResult{status: res.StatusCode, body: raw}
}

func TestRandomPhotosGateRequired(t *testing.T) {
	d := liveDeps(t)
	tripID := d.createTrip(t)
	d.publishTrip(t, tripID, "已发布旅行")
	d.seedPhoto(t, tripID, "锁定照片")

	visitor := visitorClient(t)

	// 未解锁门禁的访客不可读随机照片。
	r := d.doWith(t, visitor, "GET", "/api/photos/random", "")
	r.expectStatus(t, http.StatusUnauthorized)
	errBody, _ := r.decode(t)["error"].(map[string]any)
	if code, _ := errBody["code"].(string); code != "gate_required" {
		t.Fatalf("error code = %q, want gate_required (%s)", code, r.body)
	}
	if strings.Contains(string(r.body), "/api/storage/") {
		t.Fatalf("locked response leaked photo url: %s", r.body)
	}

	d.doWith(t, visitor, "POST", "/api/site/unlock", `{"password":"gatepass"}`).expectStatus(t, http.StatusOK)
	r = d.doWith(t, visitor, "GET", "/api/photos/random", "")
	r.expectStatus(t, http.StatusOK)
	data, _ := r.decode(t)["data"].(map[string]any)
	if enabled, _ := data["enabled"].(bool); !enabled {
		t.Fatalf("enabled = false, want true (local storage configured)")
	}
	photos, _ := data["photos"].([]any)
	if len(photos) != 1 {
		t.Fatalf("photos = %d, want 1: %v", len(photos), photos)
	}
	photo, _ := photos[0].(map[string]any)
	for _, field := range []string{"id", "trip_id", "trip_title", "thumb_src", "preview_src", "src"} {
		if v, _ := photo[field].(string); v == "" {
			t.Fatalf("random photo %s empty: %v", field, photo)
		}
	}
	if title, _ := photo["trip_title"].(string); title != "已发布旅行" {
		t.Fatalf("trip_title = %q, want 已发布旅行", title)
	}
}

func TestRandomPhotosPoolFiltersAndLimit(t *testing.T) {
	d := liveDeps(t)

	published := d.createTrip(t)
	d.publishTrip(t, published, "已发布旅行")
	draft := d.createTrip(t)

	publishedIDs := map[string]bool{}
	for i := 0; i < 5; i++ {
		publishedIDs[d.seedPhoto(t, published, fmt.Sprintf("照片%d", i))] = true
	}
	videoID := d.seedVideo(t, published)
	for i := 0; i < 2; i++ {
		d.seedPhoto(t, draft, fmt.Sprintf("草稿照片%d", i))
	}

	d.do(t, "POST", "/api/site/unlock", `{"password":"gatepass"}`).expectStatus(t, http.StatusOK)

	// limit 生效，且只含已发布旅行的照片（视频与草稿照片都不出现）。
	photos, _ := randomPhotos(t, d, "?limit=2")
	if len(photos) != 2 {
		t.Fatalf("limit=2 returned %d photos: %v", len(photos), photos)
	}
	for _, raw := range photos {
		photo, _ := raw.(map[string]any)
		id, _ := photo["id"].(string)
		if id == videoID {
			t.Fatalf("standalone video leaked into random pool: %v", photo)
		}
		if !publishedIDs[id] {
			t.Fatalf("unexpected photo id %q (draft or video row): %v", id, photo)
		}
		if tripID, _ := photo["trip_id"].(string); tripID != published {
			t.Fatalf("photo %q trip_id = %q, want %q", id, tripID, published)
		}
	}

	// 覆盖全池：5 张已发布照片全部可被选中，视频始终缺席。
	all, _ := randomPhotos(t, d, "?limit=12")
	if len(all) != 5 {
		t.Fatalf("limit=12 returned %d photos, want 5: %v", len(all), all)
	}
	for _, raw := range all {
		photo, _ := raw.(map[string]any)
		if id, _ := photo["id"].(string); !publishedIDs[id] {
			t.Fatalf("unexpected id in full pool: %v", photo)
		}
	}

	// 非法值与越界值回落默认 3 张。
	for _, query := range []string{"", "?limit=abc", "?limit=0", "?limit=99"} {
		photos, _ := randomPhotos(t, d, query)
		if len(photos) != 3 {
			t.Fatalf("query %q returned %d photos, want default 3", query, len(photos))
		}
	}
}

func TestRandomPhotosStorageDisabled(t *testing.T) {
	// 未配置存储后端（NoopBackend）：enabled=false，地址字段留空，前端据此回退渐变占位。
	d := newStack(t)
	ctx := context.Background()
	tripID, err := d.st.CreateTrip(ctx, store.TripInput{
		Title: "无存储旅行", StartDate: "2026-08-01", Status: "published",
	})
	if err != nil {
		t.Fatalf("create trip: %v", err)
	}
	if _, err := d.st.CreatePhoto(ctx, store.PhotoInput{
		TripID: tripID, COSKey: "trips/" + tripID + "/x.jpg", Title: "无存储照片",
	}); err != nil {
		t.Fatalf("create photo: %v", err)
	}
	d.do(t, "POST", "/api/auth/login", `{"username":"admin","password":"secret123"}`).
		expectStatus(t, http.StatusOK)

	photos, enabled := randomPhotos(t, d, "")
	if enabled {
		t.Fatalf("enabled = true, want false without storage backend")
	}
	if len(photos) != 1 {
		t.Fatalf("photos = %d, want 1: %v", len(photos), photos)
	}
	photo, _ := photos[0].(map[string]any)
	for _, field := range []string{"src", "preview_src", "thumb_src"} {
		if v, _ := photo[field].(string); v != "" {
			t.Fatalf("%s = %q, want empty without storage backend", field, v)
		}
	}
}
