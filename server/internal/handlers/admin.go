package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"keepsake/internal/auth"
	"keepsake/internal/store"
)

// ---------- 旅行 CRUD（计划 §4.2：读取公开；写入管理员） ----------

func (h *Handlers) listAdminTrips(w http.ResponseWriter, r *http.Request) {
	trips, err := h.st.ListTripsAdmin(r.Context())
	if err != nil {
		mapStoreErr(w, err)
		return
	}
	h.fillCoverThumbs(r.Context(), trips)
	Data(w, http.StatusOK, trips)
}

// getAdminTrip：管理端读取（含草稿与地点），供后台编辑界面使用。
func (h *Handlers) getAdminTrip(w http.ResponseWriter, r *http.Request) {
	trip, locations, err := h.st.GetTripAny(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		mapStoreErr(w, err)
		return
	}
	h.fillCoverThumb(r.Context(), trip)
	Data(w, http.StatusOK, map[string]any{"trip": trip, "locations": locations})
}

func validTrip(in *store.TripInput) (string, bool) {
	if msg := validRequired("title", in.Title, 200); msg != "" {
		return msg, false
	}
	if len(in.Summary) > 2000 {
		return "summary 过长（上限 2000 字符）", false
	}
	if !validDate(in.StartDate) {
		return "start_date 需为 YYYY-MM-DD 有效日期", false
	}
	if in.EndDate != nil && *in.EndDate != "" && !validDate(*in.EndDate) {
		return "end_date 需为 YYYY-MM-DD 有效日期", false
	}
	if in.Status != "draft" && in.Status != "published" {
		return "status 只能是 draft 或 published", false
	}
	return "", true
}

func (h *Handlers) createTrip(w http.ResponseWriter, r *http.Request) {
	var in store.TripInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg, ok := validTrip(&in); !ok {
		err(w, http.StatusBadRequest, "bad_request", msg)
		return
	}
	id, terr := h.st.CreateTrip(r.Context(), in)
	if terr != nil {
		mapStoreErr(w, terr)
		return
	}
	Data(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *Handlers) updateTrip(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var in store.TripInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg, ok := validTrip(&in); !ok {
		err(w, http.StatusBadRequest, "bad_request", msg)
		return
	}
	// 封面校验：非空指针且非空串才需要落地校验（"" 是清除，缺失是保持）。
	if in.CoverPhotoID != nil && *in.CoverPhotoID != "" {
		tripID, _, _, kind, perr := h.st.GetPhotoKeys(r.Context(), *in.CoverPhotoID)
		if perr != nil && perr != store.ErrNotFound {
			mapStoreErr(w, perr)
			return
		}
		if perr != nil || tripID != id {
			err(w, http.StatusBadRequest, "bad_request", "封面照片不存在或不属于该旅行")
			return
		}
		if kind == store.MediaVideo {
			err(w, http.StatusBadRequest, "bad_request", "视频不能用作旅行封面")
			return
		}
	}
	if uerr := h.st.UpdateTrip(r.Context(), id, in); uerr != nil {
		mapStoreErr(w, uerr)
		return
	}
	Data(w, http.StatusOK, map[string]any{"updated": true})
}

func (h *Handlers) deleteTrip(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if derr := h.st.DeleteTrip(r.Context(), id); derr != nil {
		mapStoreErr(w, derr)
		return
	}
	// 注意：数据库关联已级联删除；COS 原图对象的补偿清理将在 COS 阶段任务化（计划 §4.1）。
	Data(w, http.StatusOK, map[string]any{"deleted": true})
}

func (h *Handlers) putTripLocations(w http.ResponseWriter, r *http.Request) {
	tripID := chi.URLParam(r, "id")
	if _, _, gerr := h.st.GetTripAny(r.Context(), tripID); gerr != nil {
		mapStoreErr(w, gerr)
		return
	}
	var in struct {
		Locations []store.TripLocationInput `json:"locations"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	for _, l := range in.Locations {
		if msg := validRequired("name", l.Name, 200); msg != "" {
			err(w, http.StatusBadRequest, "bad_request", msg)
			return
		}
	}
	if rerr := h.st.ReplaceTripLocations(r.Context(), tripID, in.Locations); rerr != nil {
		mapStoreErr(w, rerr)
		return
	}
	Data(w, http.StatusOK, map[string]any{"updated": true})
}

// ---------- 相册 ----------

func validAlbum(in *store.AlbumInput) (string, bool) {
	if msg := validRequired("title", in.Title, 120); msg != "" {
		return msg, false
	}
	if len(in.Description) > 2000 {
		return "description 过长（上限 2000 字符）", false
	}
	return "", true
}

func (h *Handlers) listAlbums(w http.ResponseWriter, r *http.Request) {
	albums, err := h.st.ListAlbums(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		mapStoreErr(w, err)
		return
	}
	Data(w, http.StatusOK, albums)
}

func (h *Handlers) createAlbum(w http.ResponseWriter, r *http.Request) {
	tripID := chi.URLParam(r, "id")
	if _, _, gerr := h.st.GetTripAny(r.Context(), tripID); gerr != nil {
		mapStoreErr(w, gerr)
		return
	}
	var in store.AlbumInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg, ok := validAlbum(&in); !ok {
		err(w, http.StatusBadRequest, "bad_request", msg)
		return
	}
	id, cerr := h.st.CreateAlbum(r.Context(), tripID, in)
	if cerr != nil {
		mapStoreErr(w, cerr)
		return
	}
	Data(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *Handlers) updateAlbum(w http.ResponseWriter, r *http.Request) {
	var in store.AlbumInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg, ok := validAlbum(&in); !ok {
		err(w, http.StatusBadRequest, "bad_request", msg)
		return
	}
	if uerr := h.st.UpdateAlbum(r.Context(), chi.URLParam(r, "id"), in); uerr != nil {
		mapStoreErr(w, uerr)
		return
	}
	Data(w, http.StatusOK, map[string]any{"updated": true})
}

func (h *Handlers) deleteAlbum(w http.ResponseWriter, r *http.Request) {
	if derr := h.st.DeleteAlbum(r.Context(), chi.URLParam(r, "id")); derr != nil {
		mapStoreErr(w, derr)
		return
	}
	Data(w, http.StatusOK, map[string]any{"deleted": true})
}

// ---------- 生日信（单实例 upsert） ----------

func (h *Handlers) putLetter(w http.ResponseWriter, r *http.Request) {
	var in store.Letter
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg := validRequired("title", in.Title, 200); msg != "" {
		err(w, http.StatusBadRequest, "bad_request", msg)
		return
	}
	if len(in.Body) > 64*1024 {
		err(w, http.StatusBadRequest, "bad_request", "信件正文过长（上限 64KB）")
		return
	}
	in.AudioKey = strings.TrimSpace(in.AudioKey)
	if uerr := h.st.UpsertLetter(r.Context(), in); uerr != nil {
		mapStoreErr(w, uerr)
		return
	}
	Data(w, http.StatusOK, map[string]any{"updated": true})
}

// ---------- 站点设置（白名单 + 口令哈希化存储） ----------

// adminWritableSettings：管理端可写字段；site_password、open_date 与
// not_open_message 在校验分支中单独处理。
var adminWritableSettings = map[string]bool{
	"site_title": true, "hero_kicker": true, "hero_title": true,
	"hero_subtitle": true, "hero_cta": true, "site_motto": true,
	"open_date": true,
}

func (h *Handlers) putSettings(w http.ResponseWriter, r *http.Request) {
	var in map[string]any
	if !decodeJSON(w, r, &in) {
		return
	}
	kv := make(map[string]string, len(in))
	for key, raw := range in {
		val, ok := raw.(string)
		if !ok {
			err(w, http.StatusBadRequest, "bad_request", key+" 的值必须是字符串")
			return
		}
		if key == "site_password" {
			if msg, ok := settingPasswordOK(val); !ok {
				err(w, http.StatusBadRequest, "bad_request", msg)
				return
			}
			hash, herr := auth.HashPassword(val)
			if herr != nil {
				slogInternalError(herr)
				err(w, http.StatusInternalServerError, "internal_error", "服务内部错误")
				return
			}
			kv["site_password"] = hash
			continue
		}
		if key == "open_date" {
			// 空串表示取消开放日期限制；其余必须是合法的自然日。
			val = strings.TrimSpace(val)
			if val != "" && !validDate(val) {
				err(w, http.StatusBadRequest, "bad_request", "open_date 需为 YYYY-MM-DD 有效日期（留空表示不限制）")
				return
			}
			kv[key] = val
			continue
		}
		if key == "not_open_message" {
			// 空串表示使用默认提示语；{date} 会在返回时替换为开放日期。
			val = strings.TrimSpace(val)
			if utf8.RuneCountInString(val) > 200 {
				err(w, http.StatusBadRequest, "bad_request", "not_open_message 过长（上限 200 字）")
				return
			}
			kv[key] = val
			continue
		}
		if !adminWritableSettings[key] {
			err(w, http.StatusBadRequest, "bad_request", "字段 "+key+" 不可写或不存在")
			return
		}
		if len(val) > 2000 {
			err(w, http.StatusBadRequest, "bad_request", key+" 过长（上限 2000 字符）")
			return
		}
		kv[key] = strings.TrimSpace(val)
	}
	if len(kv) == 0 {
		err(w, http.StatusBadRequest, "bad_request", "没有可写入的字段")
		return
	}
	if serr := h.st.SetSettings(r.Context(), kv); serr != nil {
		mapStoreErr(w, serr)
		return
	}
	Data(w, http.StatusOK, map[string]any{"updated": true})
}

func settingPasswordOK(password string) (string, bool) {
	if len(password) < 6 || len(password) > 128 {
		return "site_password 长度需为 6..128", false
	}
	return "", true
}

// ---------- 共享校验工具 ----------

func validRequired(field, value string, max int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return field + " 不能为空"
	}
	if len(value) > max {
		return field + " 过长（上限 " + strconv.Itoa(max) + " 字符）"
	}
	return ""
}

func validDate(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
