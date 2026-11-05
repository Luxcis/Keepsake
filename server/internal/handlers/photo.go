package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"keepsake/internal/config"
	"keepsake/internal/cosutil"
	"keepsake/internal/store"
)

// ---------- 计划 §4.3 上传与图片处理 ----------

// allowedImageTypes：Content-Type 白名单 → 扩展名（对象 Key 生成用）。
var allowedImageTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
	"image/heic": "heic",
	"image/heif": "heif",
}

// allowedVideoTypes：实况照片视频对象白名单 → 扩展名。
// mp4 覆盖 Android 动态照片与内嵌 MP4；quicktime 覆盖 iOS 实况照片导出的 .MOV。
var allowedVideoTypes = map[string]string{
	"video/mp4":       "mp4",
	"video/quicktime": "mov",
}

// uploadExt 合并图片与视频白名单，返回 Key 生成用的扩展名。
func uploadExt(contentType string) (string, bool) {
	if ext, ok := allowedImageTypes[contentType]; ok {
		return ext, true
	}
	ext, ok := allowedVideoTypes[contentType]
	return ext, ok
}

const maxUploadBytes = config.MaxUploadBytes // 30MB：签名接口、确认阶段与本地存储写入共用

// cosErrToHTTP：存储层错误 → 统一 HTTP 语义（本地存储与 COS 共用）。
// cos_not_configured 单独成码：管理员需要一眼看出“是环境没配存储”，而非服务故障。
func cosErrToHTTP(w http.ResponseWriter, cerr error) {
	switch {
	case errors.Is(cerr, cosutil.ErrNotConfigured):
		err(w, http.StatusServiceUnavailable, "cos_not_configured",
			"文件存储尚未配置：请设置 STORAGE_DRIVER=local 或 COS_SECRET_ID/KEY/REGION/BUCKET")
	case errors.Is(cerr, cosutil.ErrObjectMissing):
		err(w, http.StatusBadRequest, "object_missing", "对象不存在：请重新上传")
	default:
		slogInternalError(cerr)
		err(w, http.StatusBadGateway, "cos_error", "对象存储访问失败，请稍后再试")
	}
}

// POST /api/admin/uploads/sign —— 校验 → 服务端生成 Key → 短时效 PUT 签名。
func (h *Handlers) signUpload(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TripID      string `json:"trip_id"`
		AlbumID     string `json:"album_id"`
		ContentType string `json:"content_type"`
		Size        int64  `json:"size"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if _, _, gerr := h.st.GetTripAny(r.Context(), in.TripID); gerr != nil {
		mapStoreErr(w, gerr)
		return
	}
	in.ContentType = strings.ToLower(strings.TrimSpace(in.ContentType))
	ext, ok := uploadExt(in.ContentType)
	if !ok {
		err(w, http.StatusBadRequest, "bad_request", "仅支持 jpeg/png/webp/heic/heif 与实况视频 mp4/mov")
		return
	}
	if in.Size <= 0 || in.Size > maxUploadBytes {
		err(w, http.StatusBadRequest, "bad_request",
			"文件大小需在 1 字节到 "+strconv.FormatInt(maxUploadBytes>>20, 10)+"MB 之间")
		return
	}

	key := uploadKey(in.TripID, ext)
	uploadURL, expireAt, serr := h.cos.SignPUT(key, in.ContentType)
	if serr != nil {
		cosErrToHTTP(w, serr)
		return
	}

	resp := map[string]any{
		"cos_key":      key,
		"upload_url":   uploadURL,
		"content_type": in.ContentType,
		"max_bytes":    maxUploadBytes,
		"expires_at":   expireAt.UTC().Format(time.RFC3339),
	}
	if in.AlbumID != "" {
		resp["album_id"] = in.AlbumID
	}
	Data(w, http.StatusOK, resp)
}

// uploadKey 生成对象 Key：trips/{tripID}/{yyyyMM}/{uuid}.{ext}（计划 §4.3 服务端生成）。
func uploadKey(tripID, ext string) string {
	month := time.Now().UTC().Format("200601")
	return "trips/" + tripID + "/" + month + "/" + store.NewUUID() + "." + ext
}

// POST /api/admin/photos/confirm —— HEAD 校验对象真实存在、大小与类型一致后才入库。
func (h *Handlers) confirmPhoto(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TripID      string  `json:"trip_id"`
		AlbumID     *string `json:"album_id"`
		COSKey      string  `json:"cos_key"`
		VideoKey    string  `json:"video_key"`
		Title       string  `json:"title"`
		Description string  `json:"description"`
		TakenAt     *string `json:"taken_at"`
		Width       *int64  `json:"width"`
		Height      *int64  `json:"height"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}

	// Key 归属校验：禁止把其它旅行的对象塞进当前旅行（prefix 绑 tripID）。
	if msg := validRequired("cos_key", in.COSKey, 255); msg != "" {
		err(w, http.StatusBadRequest, "bad_request", msg)
		return
	}
	if !strings.HasPrefix(in.COSKey, "trips/"+in.TripID+"/") {
		err(w, http.StatusBadRequest, "bad_request", "cos_key 与旅行不匹配")
		return
	}
	if in.TakenAt != nil && *in.TakenAt != "" && !validTakenAt(*in.TakenAt) {
		err(w, http.StatusBadRequest, "bad_request", "taken_at 需为 YYYY-MM-DD 或 RFC3339 时间")
		return
	}

	size, ctype, herr := h.cos.Head(in.COSKey)
	if herr != nil {
		cosErrToHTTP(w, herr)
		return
	}
	if size <= 0 || size > maxUploadBytes {
		err(w, http.StatusBadRequest, "bad_request", "对象大小超出限制")
		return
	}
	if baseCT(ctype) != "" && allowedImageTypes[strings.ToLower(baseCT(ctype))] == "" {
		err(w, http.StatusBadRequest, "bad_request", "对象类型不在白名单内: "+ctype)
		return
	}

	// 实况视频（可选）：与图片同属当前旅行前缀，且必须是白名单内的视频对象。
	var videoKey *string
	if vk := strings.TrimSpace(in.VideoKey); vk != "" {
		if len(vk) > 255 {
			err(w, http.StatusBadRequest, "bad_request", "video_key 过长")
			return
		}
		if !strings.HasPrefix(vk, "trips/"+in.TripID+"/") {
			err(w, http.StatusBadRequest, "bad_request", "video_key 与旅行不匹配")
			return
		}
		vsize, vctype, vherr := h.cos.Head(vk)
		if vherr != nil {
			cosErrToHTTP(w, vherr)
			return
		}
		if vsize <= 0 || vsize > maxUploadBytes {
			err(w, http.StatusBadRequest, "bad_request", "实况视频大小超出限制")
			return
		}
		if allowedVideoTypes[strings.ToLower(baseCT(vctype))] == "" {
			err(w, http.StatusBadRequest, "bad_request", "实况视频类型不在白名单内: "+vctype)
			return
		}
		videoKey = &vk
	}

	in.Title = strings.TrimSpace(in.Title)
	if len(in.Title) > 200 {
		err(w, http.StatusBadRequest, "bad_request", "title 过长")
		return
	}
	if len(in.Description) > 2000 {
		err(w, http.StatusBadRequest, "bad_request", "description 过长")
		return
	}

	id, cerr := h.st.CreatePhoto(r.Context(), store.PhotoInput{
		TripID: in.TripID, AlbumID: in.AlbumID, COSKey: in.COSKey, VideoKey: videoKey,
		Title: in.Title, Description: in.Description, TakenAt: in.TakenAt,
		Width: in.Width, Height: in.Height,
	})
	if cerr != nil {
		if strings.Contains(cerr.Error(), "constraint failed") {
			err(w, http.StatusConflict, "duplicate_key", "该照片对象已确认过")
			return
		}
		mapStoreErr(w, cerr)
		return
	}
	Data(w, http.StatusCreated, map[string]any{"id": id})
}

// baseCT 处理可能带参数的 Content-Type（如 'image/jpeg; charset=...' 场景）。
func baseCT(ct string) string {
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		return strings.TrimSpace(ct[:i])
	}
	return ct
}

func validTakenAt(s string) bool {
	if len(s) == 10 {
		return validDate(s)
	}
	_, err := time.Parse(time.RFC3339, s)
	return err == nil
}

// GET /api/trips/{id}/photos —— 门禁后：真实元数据 + 短时效签名 URL。
// enabled=false（存储后端未配置）时返回空 URL 让前端给出提示（不影响旅行页面其它信息）。
// 仅已发布旅行（公开门禁语义）；管理端走 adminListTripPhotos。
func (h *Handlers) listTripPhotos(w http.ResponseWriter, r *http.Request) {
	if _, _, terr := h.st.GetTrip(r.Context(), chi.URLParam(r, "id")); terr != nil {
		mapStoreErr(w, terr)
		return
	}
	h.servePhotos(w, r, chi.URLParam(r, "id"))
}

// adminListTripPhotos 管理端：草稿同样可管理，输出与公开结构一致。
func (h *Handlers) adminListTripPhotos(w http.ResponseWriter, r *http.Request) {
	if _, _, gerr := h.st.GetTripAny(r.Context(), chi.URLParam(r, "id")); gerr != nil {
		mapStoreErr(w, gerr)
		return
	}
	h.servePhotos(w, r, chi.URLParam(r, "id"))
}

func (h *Handlers) servePhotos(w http.ResponseWriter, r *http.Request, tripID string) {
	photos, err := h.st.ListPhotosByTrip(r.Context(), tripID)
	if err != nil {
		mapStoreErr(w, err)
		return
	}

	enabled := h.cos.Enabled()
	out := []map[string]any{}
	for _, p := range photos {
		row := map[string]any{
			"id": p.ID, "album_id": p.AlbumID.String, "title": p.Title,
			"description": p.Description, "width": p.Width, "height": p.Height,
			"sort_order": p.SortOrder, "taken_at": nilOrValue(p.TakenAt.Valid, p.TakenAt.String),
		}
		if enabled {
			src, serr := h.cos.SignGET(p.COSKey)
			if serr != nil {
				slog.Warn("photo sign get failed", "key", p.COSKey, "err", serr)
				src = ""
			}
			thumbSrc := ""
			previewSrc := ""
			if src != "" {
				thumbSrc = h.signThumb(p.COSKey, p.ThumbnailKey)
				previewSrc = h.signPreview(p.COSKey)
			}
			row["src"] = src
			row["preview_src"] = previewSrc
			row["thumb_src"] = thumbSrc
			row["video_src"] = h.signVideo(p.VideoKey)
		} else {
			row["src"] = ""
			row["preview_src"] = ""
			row["thumb_src"] = ""
			row["video_src"] = ""
		}
		out = append(out, row)
	}
	Data(w, http.StatusOK, map[string]any{"enabled": enabled, "photos": out})
}

// signThumb 签名照片缩略图 URL：优先独立缩略图对象；否则把缩放处理参数
// 签入原图 GET URL —— 处理参数必须参与签名，签名后追加会被 COS 校验拒绝（403）。
// 失败时返回空串并记 warn（photo sign 语义：不阻断内容响应）。
func (h *Handlers) signThumb(cosKey string, thumbKey sql.NullString) string {
	if !h.cos.Enabled() {
		return ""
	}
	if thumbKey.Valid {
		u, gerr := h.cos.SignGET(thumbKey.String)
		if gerr == nil {
			return u
		}
		slog.Warn("photo sign thumb failed", "key", thumbKey.String, "err", gerr)
	}
	u, terr := h.cos.SignGETThumb(cosKey)
	if terr != nil {
		slog.Warn("photo sign thumb failed", "key", cosKey, "err", terr)
		return ""
	}
	return u
}

// signPreview 签名灯箱显示尺寸 URL；失败时返回空串并记 warn。
// 前端在为空时回退到原图 src，保证功能不因缩放签名失败而中断。
func (h *Handlers) signPreview(cosKey string) string {
	if !h.cos.Enabled() {
		return ""
	}
	u, err := h.cos.SignGETPreview(cosKey)
	if err != nil {
		slog.Warn("photo sign preview failed", "key", cosKey, "err", err)
		return ""
	}
	return u
}

// signVideo 签名实况视频 URL；无视频或签名失败时返回空串。
// 前端以 video_src 是否存在判定实况照片，空串即普通静态照片。
func (h *Handlers) signVideo(videoKey sql.NullString) string {
	if !h.cos.Enabled() || !videoKey.Valid || videoKey.String == "" {
		return ""
	}
	u, err := h.cos.SignGET(videoKey.String)
	if err != nil {
		slog.Warn("photo sign video failed", "key", videoKey.String, "err", err)
		return ""
	}
	return u
}

// fillCoverThumb 为单个旅行按其封面照片签名 cover_thumb_src；
// 悬挂引用（照片已删或该封面属于其它旅行）与 COS 未配置时静默保持为空。
func (h *Handlers) fillCoverThumb(ctx context.Context, t *store.Trip) {
	if !h.cos.Enabled() || !t.CoverPhotoID.Valid {
		return
	}
	tripID, cosKey, thumbKey, gerr := h.st.GetPhotoKeys(ctx, t.CoverPhotoID.String)
	if gerr != nil || tripID != t.ID {
		return
	}
	t.CoverThumbSrc = h.signThumb(cosKey, thumbKey)
}

func (h *Handlers) fillCoverThumbs(ctx context.Context, trips []store.Trip) {
	for i := range trips {
		h.fillCoverThumb(ctx, &trips[i])
	}
}

func nilOrValue(valid bool, v string) any {
	if !valid {
		return nil
	}
	return v
}

// PUT /api/admin/photos/{id} —— 说明类字段维护（title/description/taken_at/album_id）。
// album_id 传空串表示把照片挪回“旅行层级”（album_id = NULL）。
func (h *Handlers) updatePhoto(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		TakenAt     *string `json:"taken_at"`
		AlbumID     string  `json:"album_id"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if msg := validRequired("title", in.Title, 200); msg != "" {
		err(w, http.StatusBadRequest, "bad_request", msg)
		return
	}
	if len(in.Description) > 2000 {
		err(w, http.StatusBadRequest, "bad_request", "description 过长（上限 2000 字符）")
		return
	}
	if in.TakenAt != nil && *in.TakenAt != "" && !validTakenAt(*in.TakenAt) {
		err(w, http.StatusBadRequest, "bad_request", "taken_at 需为 YYYY-MM-DD 或 RFC3339 时间")
		return
	}
	if uerr := h.st.UpdatePhotoMeta(r.Context(), chi.URLParam(r, "id"), store.PhotoInput{
		Title: in.Title, Description: in.Description, TakenAt: in.TakenAt,
	}, store.AlbumRef{Set: &in.AlbumID}); uerr != nil {
		mapStoreErr(w, uerr)
		return
	}
	Data(w, http.StatusOK, map[string]any{"updated": true})
}

// PUT /api/admin/photos/reorder —— 传入端序即 sort_order。
func (h *Handlers) reorderPhotos(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if len(in.IDs) == 0 {
		err(w, http.StatusBadRequest, "bad_request", "ids 不能为空")
		return
	}
	if rerr := h.st.ReorderPhotos(r.Context(), in.IDs); rerr != nil {
		mapStoreErr(w, rerr)
		return
	}
	Data(w, http.StatusOK, map[string]any{"reordered": true})
}

// DELETE /api/admin/photos/{id} —— 先删数据库关联，再同步删除视频/缩略图/原图对象；
// 只有同步删除失败的对象才入队，由 janitor 重试（计划 §4.1）。
func (h *Handlers) deletePhoto(w http.ResponseWriter, r *http.Request) {
	cosKey, thumbKey, videoKey, derr := h.st.DeletePhoto(r.Context(), chi.URLParam(r, "id"))
	if derr != nil {
		mapStoreErr(w, derr)
		return
	}
	seen := make(map[string]bool, 3)
	for _, key := range []string{cosKey, thumbKey, videoKey} {
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if delErr := h.cos.Delete(key); delErr != nil {
			slog.Error("delete storage object failed, enqueue retry", "key", key, "err", delErr)
			if _, qerr := h.st.EnqueueDeleteTask(r.Context(), key); qerr != nil {
				// 任务队列写入失败：主链路仍然成功；孤儿对象的兜底依赖备份恢复与人工清理
				slog.Error("enqueue delete task failed", "err", qerr)
			}
		}
	}
	Data(w, http.StatusOK, map[string]any{"deleted": true})
}
