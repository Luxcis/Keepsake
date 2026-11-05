package server_test

// 端到端集成流：门禁 → 认证 → 管理端 CRUD → 公开内容可见性 + CSRF。
// 使用内存会话库（无 Redis），覆盖正常语义；失败关闭语义依赖六容器验证或在 Redis 路径单测。

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"keepsake/internal/auth"
	"keepsake/internal/cachex"
	"keepsake/internal/config"
	"keepsake/internal/cosutil"
	"keepsake/internal/server"
	"keepsake/internal/store"
)

type deps struct {
	st   *store.Store
	http *http.Client
	base string
	// storageRoot 是本地存储对象根目录；仅使用 localstore 的集成测试会设置。
	storageRoot string
}

func newStack(t *testing.T) *deps {
	t.Helper()
	return newStackWithBackend(t, cosutil.NoopBackend{})
}

// newStackWithBackend 允许用真实本地存储后端跑完整直传链路（签名 → PUT → confirm）。
func newStackWithBackend(t *testing.T, backend cosutil.Backend) *deps {
	t.Helper()

	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir()+string(os.PathSeparator)+"test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := auth.NewService(st, auth.NewMemorySessionStore(), auth.NewLimiter(nil), log, false)
	if err := svc.CreateAdminUser(ctx, "admin", "secret123"); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	gateHash, gerr := auth.HashPassword("gatepass")
	if gerr != nil {
		t.Fatalf("hash gate: %v", gerr)
	}
	if err := st.SetSettings(ctx, map[string]string{"site_password": gateHash}); err != nil {
		t.Fatalf("set gate: %v", err)
	}

	cfg := config.Config{Env: "dev", Port: "0"}
	ts := httptest.NewServer(server.New(cfg, st, cachex.New(nil), backend, svc, log))
	t.Cleanup(ts.Close)

	jar, jerr := cookiejar.New(nil)
	if jerr != nil {
		t.Fatalf("cookiejar: %v", jerr)
	}
	return &deps{
		st:   st,
		http: &http.Client{Jar: jar},
		base: ts.URL,
	}
}

func (d *deps) do(t *testing.T, method, path, body string) apiResult {
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
	res, err := d.http.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return apiResult{status: res.StatusCode, body: raw}
}

type apiResult struct {
	status int
	body   []byte
}

func (r apiResult) expectStatus(t *testing.T, want int) {
	t.Helper()
	if r.status != want {
		t.Fatalf("status = %d, want %d, body=%s", r.status, want, r.body)
	}
}

// decode 展开为 map，便于按路径断言。
func (r apiResult) decode(t *testing.T) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(r.body, &v); err != nil {
		t.Fatalf("decode %s: %v", r.body, err)
	}
	return v
}

func (r apiResult) errorCode(t *testing.T) string {
	t.Helper()
	m := r.decode(t)
	e, ok := m["error"].(map[string]any)
	if !ok {
		return ""
	}
	c, _ := e["code"].(string)
	return c
}

func TestUnlockGateFlow(t *testing.T) {
	d := newStack(t)

	// 1) 未过门禁 → 401 gate_required
	r := d.do(t, "GET", "/api/trips", "")
	r.expectStatus(t, http.StatusUnauthorized)
	if code := r.errorCode(t); code != "gate_required" {
		t.Fatalf("code = %s", code)
	}

	// 2) 错误口令 → 401
	r = d.do(t, "POST", "/api/site/unlock", `{"password":"nope"}`)
	r.expectStatus(t, http.StatusUnauthorized)
	if code := r.errorCode(t); code != "gate_mismatch" {
		t.Fatalf("code = %s", code)
	}

	// 3) 正确口令 → 200 + gate Cookie（自动存入 jar）
	r = d.do(t, "POST", "/api/site/unlock", `{"password":"gatepass"}`)
	r.expectStatus(t, http.StatusOK)

	// 4) 门禁后可读旅行（暂为空列表）
	r = d.do(t, "GET", "/api/trips", "")
	r.expectStatus(t, http.StatusOK)
	if !strings.Contains(string(r.body), `"data":[]`) {
		t.Fatalf("expected empty trips, got %s", r.body)
	}

	// 5) 信件未配置 → 404
	r = d.do(t, "GET", "/api/letter", "")
	r.expectStatus(t, http.StatusNotFound)
}

func TestAdminFullFlow(t *testing.T) {
	d := newStack(t)

	// 错误凭据 → 401
	r := d.do(t, "POST", "/api/auth/login", `{"username":"admin","password":"bad"}`)
	r.expectStatus(t, http.StatusUnauthorized)

	// 正确登录
	r = d.do(t, "POST", "/api/auth/login", `{"username":"admin","password":"secret123"}`)
	r.expectStatus(t, http.StatusOK)

	// me
	r = d.do(t, "GET", "/api/auth/me", "")
	r.expectStatus(t, http.StatusOK)
	if m := r.decode(t); m["data"].(map[string]any)["username"] != "admin" {
		t.Fatalf("me.username = %s", r.body)
	}

	// 管理员会话可直接读私人内容（gate 语义）
	r = d.do(t, "GET", "/api/trips", "")
	r.expectStatus(t, http.StatusOK)

	// 创建旅行（草稿）
	r = d.do(t, "POST", "/api/admin/trips",
		`{"title":"海边的两天","summary":"风吹过海面","start_date":"2026-08-01","status":"draft"}`)
	r.expectStatus(t, http.StatusCreated)
	data, _ := r.decode(t)["data"].(map[string]any)
	tripID, _ := data["id"].(string)
	if tripID == "" {
		t.Fatalf("no trip id: %s", r.body)
	}

	// 草稿不出现在公开列表
	r = d.do(t, "GET", "/api/trips", "")
	if strings.Contains(string(r.body), tripID) {
		t.Fatalf("draft must be hidden, got %s", r.body)
	}

	// 发布
	r = d.do(t, "PUT", "/api/admin/trips/"+tripID,
		`{"title":"海边的两天","summary":"风吹过海面","start_date":"2026-08-01","status":"published"}`)
	r.expectStatus(t, http.StatusOK)

	// 地点整组替换
	r = d.do(t, "PUT", "/api/admin/trips/"+tripID+"/locations",
		`{"locations":[{"name":"月牙湾","note":"夕阳非常好"}]}`)
	r.expectStatus(t, http.StatusOK)

	// 时光轴接口已移除：不再接受手工事件
	r = d.do(t, "GET", "/api/timeline", "")
	r.expectStatus(t, http.StatusNotFound)
	r = d.do(t, "POST", "/api/admin/timeline",
		`{"event_date":"2026-08-02","title":"一起去看海","description":"海风与日落"}`)
	r.expectStatus(t, http.StatusNotFound)

	// 信件 upsert 与读取
	r = d.do(t, "PUT", "/api/admin/letter", `{"title":"For You","body":"生日快乐。","reveal_enabled":true}`)
	r.expectStatus(t, http.StatusOK)
	r = d.do(t, "GET", "/api/letter", "")
	r.expectStatus(t, http.StatusOK)
	if m := r.decode(t); m["data"].(map[string]any)["body"] != "生日快乐。" {
		t.Fatalf("letter body = %s", r.body)
	}

	// 站点设置写入（白名单机制生效：未知键拒绝）
	r = d.do(t, "PUT", "/api/admin/site/settings", `{"site_title":"我们的纪念册"}`)
	r.expectStatus(t, http.StatusOK)
	r = d.do(t, "PUT", "/api/admin/site/settings", `{"hacker_key":"x"}`)
	r.expectStatus(t, http.StatusBadRequest)
	r = d.do(t, "GET", "/api/site/settings", "")
	r.expectStatus(t, http.StatusOK)

	// 管理端列表含草稿与已发布
	r = d.do(t, "GET", "/api/admin/trips", "")
	r.expectStatus(t, http.StatusOK)

	// 删除旅行（级联）
	r = d.do(t, "DELETE", "/api/admin/trips/"+tripID, "")
	r.expectStatus(t, http.StatusOK)
	r = d.do(t, "GET", "/api/admin/trips", "")
	if strings.Contains(string(r.body), tripID) {
		t.Fatalf("trip should be deleted")
	}
}

// TestPublicTripsOrderedByDate：公开旅行列表按出发日期倒序（最新在前），
// 同一天出发按 sort_order 升序；草稿不出现。
func TestPublicTripsOrderedByDate(t *testing.T) {
	d := newStack(t)

	r := d.do(t, "POST", "/api/auth/login", `{"username":"admin","password":"secret123"}`)
	r.expectStatus(t, http.StatusOK)

	fixtures := []struct {
		title     string
		startDate string
		sortOrder int
		status    string
	}{
		{"较早的旅行", "2025-05-01", 0, "published"},
		{"最新的旅行", "2026-09-10", 5, "published"},
		{"同日后写", "2026-09-10", 2, "published"},
		{"未发布的旅行", "2027-01-01", 0, "draft"},
	}
	for _, f := range fixtures {
		body, err := json.Marshal(map[string]any{
			"title":      f.title,
			"start_date": f.startDate,
			"sort_order": f.sortOrder,
			"status":     f.status,
		})
		if err != nil {
			t.Fatalf("marshal trip: %v", err)
		}
		r = d.do(t, "POST", "/api/admin/trips", string(body))
		r.expectStatus(t, http.StatusCreated)
	}

	r = d.do(t, "GET", "/api/trips", "")
	r.expectStatus(t, http.StatusOK)
	var payload struct {
		Data []struct {
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := json.Unmarshal(r.body, &payload); err != nil {
		t.Fatalf("decode trips: %v", err)
	}
	got := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		got = append(got, item.Title)
	}
	want := []string{"同日后写", "最新的旅行", "较早的旅行"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("trip order = %v, want %v", got, want)
	}
}

// TestTimelineEventsTableRemoved：迁移后手工时光轴事件表不再存在。
func TestTimelineEventsTableRemoved(t *testing.T) {
	d := newStack(t)

	var count int
	if err := d.st.DB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'timeline_events'`).Scan(&count); err != nil {
		t.Fatalf("inspect sqlite_master: %v", err)
	}
	if count != 0 {
		t.Fatalf("timeline_events table still exists")
	}
}

// TestTripCoverFlow：封面设置/保持/清除/校验/删除归空（COS 为 Noop，URL 一律空串降级）。
func TestTripCoverFlow(t *testing.T) {
	d := newStack(t)
	ctx := context.Background()

	// 登录（管理员会话即门禁豁免）
	r := d.do(t, "POST", "/api/auth/login", `{"username":"admin","password":"secret123"}`)
	r.expectStatus(t, http.StatusOK)

	// 两个旅行 + 各一张照片（直接落库绕过 Noop COS，open 拿不到对象）
	for _, title := range []string{"海边", "山里"} {
		r := d.do(t, "POST", "/api/admin/trips",
			`{"title":"`+title+`","start_date":"2026-08-01","status":"published"}`)
		r.expectStatus(t, http.StatusCreated)
	}
	r = d.do(t, "GET", "/api/admin/trips", "")
	admin, ok := r.decode(t)["data"].([]any)
	if !ok || len(admin) != 2 {
		t.Fatalf("admin trips = %s", r.body)
	}
	tripA, _ := admin[0].(map[string]any)["id"].(string)
	tripB, _ := admin[1].(map[string]any)["id"].(string)
	photoA, perr := d.st.CreatePhoto(ctx, store.PhotoInput{
		TripID: tripA, COSKey: "trips/" + tripA + "/202608/pa.jpg", Title: "海"})
	if perr != nil {
		t.Fatalf("create photoA: %v", perr)
	}
	photoB, perr := d.st.CreatePhoto(ctx, store.PhotoInput{
		TripID: tripB, COSKey: "trips/" + tripB + "/202608/pb.jpg", Title: "山"})
	if perr != nil {
		t.Fatalf("create photoB: %v", perr)
	}

	putCover := func(trip, photo string) apiResult {
		return d.do(t, "PUT", "/api/admin/trips/"+trip,
			`{"title":"海边","start_date":"2026-08-01","status":"published","cover_photo_id":"`+photo+`"}`)
	}
	adminCover := func(trip string) string {
		res := d.do(t, "GET", "/api/admin/trips/"+trip, "")
		res.expectStatus(t, http.StatusOK)
		data, _ := res.decode(t)["data"].(map[string]any)["trip"].(map[string]any)
		id, _ := data["cover_photo_id"].(string)
		if thumb, _ := data["cover_thumb_src"].(string); thumb != "" {
			t.Fatalf("noop COS must keep thumb empty, got %q", thumb)
		}
		return id
	}

	// 1) 设置封面：admin 详情可见，公开列表同样可见（签名 URL 空串降级）
	r = putCover(tripA, photoA)
	r.expectStatus(t, http.StatusOK)
	if got := adminCover(tripA); got != photoA {
		t.Fatalf("admin cover = %q, want %q", got, photoA)
	}
	r = d.do(t, "GET", "/api/trips", "")
	r.expectStatus(t, http.StatusOK)
	list, _ := r.decode(t)["data"].([]any)
	for _, item := range list {
		m, _ := item.(map[string]any)
		if m["id"].(string) != tripA {
			continue
		}
		if cid, _ := m["cover_photo_id"].(string); cid != photoA {
			t.Fatalf("public cover = %q, want %q", cid, photoA)
		}
	}

	// 2) 不带 cover_photo_id 的全量 PUT（toggleStatus 语义）→ 保持现有封面
	r = d.do(t, "PUT", "/api/admin/trips/"+tripA,
		`{"title":"海边","start_date":"2026-08-01","status":"draft"}`)
	r.expectStatus(t, http.StatusOK)
	if got := adminCover(tripA); got != photoA {
		t.Fatalf("cover should persist without field, got %q", got)
	}

	// 3) "" → 清除（null）
	r = putCover(tripA, "")
	r.expectStatus(t, http.StatusOK)
	if got := adminCover(tripA); got != "" {
		t.Fatalf("cover should be cleared, got %q", got)
	}

	// 4) 校验：跨旅行照片与不存在的照片 → 400
	r = putCover(tripA, photoB)
	r.expectStatus(t, http.StatusBadRequest)
	if code := r.errorCode(t); code != "bad_request" {
		t.Fatalf("cross-trip code = %s", code)
	}
	r = putCover(tripA, "no-such-photo")
	r.expectStatus(t, http.StatusBadRequest)

	// 5) 删除封面照片 → cover_photo_id 归空（无外键，应用层补偿）
	r = putCover(tripA, photoA)
	r.expectStatus(t, http.StatusOK)
	r = d.do(t, "DELETE", "/api/admin/photos/"+photoA, "")
	r.expectStatus(t, http.StatusOK)
	if got := adminCover(tripA); got != "" {
		t.Fatalf("cover should be nulled after photo delete, got %q", got)
	}
}

// 照片列表必须同时下发 src / preview_src / thumb_src 三个签名位：
// 灯箱用 preview_src（原图可能 30MB），缩略图用 thumb_src，原图保留给下载/兜底。
// Noop COS 下三者均为空串（前端据此降级），但字段必须存在。
func TestTripPhotosExposeSignedFields(t *testing.T) {
	d := newStack(t)

	r := d.do(t, "POST", "/api/auth/login", `{"username":"admin","password":"secret123"}`)
	r.expectStatus(t, http.StatusOK)
	r = d.do(t, "POST", "/api/admin/trips",
		`{"title":"海边","start_date":"2026-08-01","status":"published"}`)
	r.expectStatus(t, http.StatusCreated)

	r = d.do(t, "GET", "/api/trips", "")
	r.expectStatus(t, http.StatusOK)
	trips, _ := r.decode(t)["data"].([]any)
	if len(trips) != 1 {
		t.Fatalf("trips = %s", r.body)
	}
	tripID, _ := trips[0].(map[string]any)["id"].(string)

	if _, err := d.st.CreatePhoto(context.Background(), store.PhotoInput{
		TripID: tripID, COSKey: "trips/" + tripID + "/202608/a.jpg", Title: "海"}); err != nil {
		t.Fatalf("create photo: %v", err)
	}

	r = d.do(t, "GET", "/api/trips/"+tripID+"/photos", "")
	r.expectStatus(t, http.StatusOK)
	data, _ := r.decode(t)["data"].(map[string]any)
	photos, _ := data["photos"].([]any)
	if len(photos) != 1 {
		t.Fatalf("photos = %s", r.body)
	}
	photo, _ := photos[0].(map[string]any)
	for _, key := range []string{"src", "preview_src", "thumb_src"} {
		if _, ok := photo[key]; !ok {
			t.Fatalf("photo missing %q: %s", key, r.body)
		}
		if v, _ := photo[key].(string); v != "" {
			t.Fatalf("noop COS must keep %s empty, got %q", key, v)
		}
	}
}

func TestCSRFOriginRejected(t *testing.T) {
	d := newStack(t)

	// 登录拿 cookie
	r := d.do(t, "POST", "/api/auth/login", `{"username":"admin","password":"secret123"}`)
	r.expectStatus(t, http.StatusOK)

	postTrip := func(origin string) apiResult {
		req, berr := http.NewRequest("POST", d.base+"/api/admin/trips",
			strings.NewReader(`{"title":"海边","start_date":"2026-08-01","status":"draft"}`))
		if berr != nil {
			t.Fatalf("build: %v", berr)
		}
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		res, derr := d.http.Do(req)
		if derr != nil {
			t.Fatalf("req: %v", derr)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		return apiResult{status: res.StatusCode, body: raw}
	}

	// 恶意 Origin → 403
	r = postTrip("https://evil.example.com")
	r.expectStatus(t, http.StatusForbidden)

	// 同源 Origin → 201
	u, _ := url.Parse(d.base)
	r = postTrip("http://" + u.Host)
	r.expectStatus(t, http.StatusCreated)

	// 不带 Origin（脚本调用）→ 201
	r = postTrip("")
	r.expectStatus(t, http.StatusCreated)
}
