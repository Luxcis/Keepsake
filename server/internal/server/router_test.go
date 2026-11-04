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
}

func newStack(t *testing.T) *deps {
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
	ts := httptest.NewServer(server.New(cfg, st, cachex.New(nil), cosutil.NoopBackend{}, svc, log))
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

	// 时光轴事件
	r = d.do(t, "POST", "/api/admin/timeline",
		`{"event_date":"2026-08-02","title":"一起去看海","description":"海风与日落"}`)
	r.expectStatus(t, http.StatusCreated)

	// 公开时间轴（带管理员 cookie）可见
	r = d.do(t, "GET", "/api/timeline", "")
	r.expectStatus(t, http.StatusOK)

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
