package handlers

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"keepsake/internal/auth"
	"keepsake/internal/cachex"
	"keepsake/internal/cosutil"
	"keepsake/internal/store"
)

type Handlers struct {
	st    *store.Store
	cache *cachex.Cache
	svc   *auth.Service
	cos   cosutil.Backend
	log   *slog.Logger
}

func New(st *store.Store, cch *cachex.Cache, svc *auth.Service, cos cosutil.Backend, log *slog.Logger) *Handlers {
	return &Handlers{st: st, cache: cch, svc: svc, cos: cos, log: log}
}

// Mount 挂载 /api 全部路由（计划 §4.2 API 分组）。
// 门禁策略（计划 §5.1/§6.1）：除站点设置与健康检查外，所有私人内容读接口
// 均要求门禁会话；管理端要求管理员会话 + 同源校验。
func (h *Handlers) Mount(r chi.Router) {
	// 站点/访问（公开）
	r.Get("/site/settings", h.getSiteSettings)
	r.Post("/site/unlock", h.unlock)

	// 认证（公开；会话由处理逻辑发放，/auth/me 需要现有管理员会话）
	r.Post("/auth/login", h.login)
	r.Post("/auth/logout", h.logout)
	r.With(h.withAdmin).Get("/auth/me", h.me)

	// 私人内容（需门禁会话）
	r.Group(func(r chi.Router) {
		r.Use(h.withGate)
		r.Get("/trips", h.listTrips)
		r.Get("/trips/{id}", h.getTrip)
		r.Get("/trips/{id}/photos", h.listTripPhotos)
		r.Get("/timeline", h.listTimeline)
		r.Get("/letter", h.getLetter)
	})

	// 管理端（需管理员会话 + 同源校验）
	r.Group(func(r chi.Router) {
		r.Use(h.withAdmin)
		r.Get("/admin/trips", h.listAdminTrips)
		r.Get("/admin/trips/{id}", h.getAdminTrip)
		r.Post("/admin/trips", h.createTrip)
		r.Put("/admin/trips/{id}", h.updateTrip)
		r.Delete("/admin/trips/{id}", h.deleteTrip)
		r.Put("/admin/trips/{id}/locations", h.putTripLocations)
		r.Get("/admin/trips/{id}/photos", h.adminListTripPhotos)
		r.Get("/admin/trips/{id}/albums", h.listAlbums)
		r.Post("/admin/trips/{id}/albums", h.createAlbum)
		r.Put("/admin/albums/{id}", h.updateAlbum)
		r.Delete("/admin/albums/{id}", h.deleteAlbum)

		r.Post("/admin/timeline", h.createTimelineEvent)
		r.Put("/admin/timeline/{id}", h.updateTimelineEvent)
		r.Delete("/admin/timeline/{id}", h.deleteTimelineEvent)

		r.Put("/admin/letter", h.putLetter)
		r.Put("/admin/site/settings", h.putSettings)

		r.Post("/admin/uploads/sign", h.signUpload)
		r.Post("/admin/photos/confirm", h.confirmPhoto)
		r.Put("/admin/photos/{id}", h.updatePhoto)
		r.Put("/admin/photos/reorder", h.reorderPhotos)
		r.Delete("/admin/photos/{id}", h.deletePhoto)
	})
}

// ---------- 私人内容（公开只读，但受门禁） ----------

func (h *Handlers) getSiteSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.st.GetSiteSettings(r.Context())
	if err != nil {
		mapStoreErr(w, err)
		return
	}
	gateEnabled, gerr := h.st.GetSetting(r.Context(), "site_password")
	if gerr != nil {
		mapStoreErr(w, gerr)
		return
	}
	Data(w, http.StatusOK, map[string]any{
		"settings":     settings,
		"gate_enabled": gateEnabled != "",
	})
}

func (h *Handlers) listTrips(w http.ResponseWriter, r *http.Request) {
	trips, err := h.st.ListTrips(r.Context())
	if err != nil {
		mapStoreErr(w, err)
		return
	}
	Data(w, http.StatusOK, trips)
}

func (h *Handlers) getTrip(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	trip, locations, err := h.st.GetTrip(r.Context(), id)
	if err != nil {
		mapStoreErr(w, err)
		return
	}
	Data(w, http.StatusOK, map[string]any{"trip": trip, "locations": locations})
}

func (h *Handlers) listTimeline(w http.ResponseWriter, r *http.Request) {
	events, err := h.st.ListTimeline(r.Context())
	if err != nil {
		mapStoreErr(w, err)
		return
	}
	Data(w, http.StatusOK, events)
}

// getLetter：门禁已解锁后可读（计划 §2 生日信页）。
func (h *Handlers) getLetter(w http.ResponseWriter, r *http.Request) {
	letter, err := h.st.GetLetter(r.Context())
	if err != nil {
		mapStoreErr(w, err)
		return
	}
	Data(w, http.StatusOK, letter)
}

// Health GET /healthz —— 部署健康检查（计划 §6.1 运维验收点）。
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	if err := h.st.Ping(r.Context()); err != nil {
		dbStatus = "unreachable"
	}
	status := http.StatusOK
	if dbStatus != "ok" {
		status = http.StatusServiceUnavailable
	}
	Data(w, status, map[string]string{
		"status": map[bool]string{true: "ok", false: "degraded"}[dbStatus == "ok"],
		"db":     dbStatus,
		"redis":  h.cache.Status(r.Context()),
	})
}
