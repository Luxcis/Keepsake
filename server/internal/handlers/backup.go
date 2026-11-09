package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"keepsake/internal/backup"
)

// 管理端全量备份接口（计划：一键备份数据库与上传文件）。
// 创建为异步任务：接口只启动任务并返回 202，页面轮询状态；下载与删除直接操作备份目录。

func (h *Handlers) listBackups(w http.ResponseWriter, _ *http.Request) {
	st, err := h.backup.Status()
	if err != nil {
		h.backupError(w, "list", err)
		return
	}
	Data(w, http.StatusOK, st)
}

func (h *Handlers) createBackup(w http.ResponseWriter, _ *http.Request) {
	name, startErr := h.backup.Start()
	switch {
	case startErr == nil:
		Data(w, http.StatusAccepted, map[string]any{"started": true, "name": name})
	case errors.Is(startErr, backup.ErrRunning):
		err(w, http.StatusConflict, "backup_running", "已有备份任务正在生成，请稍候")
	default:
		h.backupError(w, "start", startErr)
	}
}

// downloadBackup 以附件形式返回备份 zip；http.ServeContent 提供范围请求支持。
func (h *Handlers) downloadBackup(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	f, info, err := h.backup.Open(name)
	if err != nil {
		h.backupError(w, "download", err)
		return
	}
	defer func() { _ = f.Close() }()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")

	// 备份包可能远大于普通 JSON 响应；COS 模式下服务端默认写超时只有 30s，
	// 这里为本次下载单独放宽到 15 分钟（与 Nginx proxy_read_timeout 一致）。
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(15 * time.Minute))

	http.ServeContent(w, r, name, info.ModTime(), f)
}

func (h *Handlers) deleteBackup(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := h.backup.Delete(name); err != nil {
		h.backupError(w, "delete", err)
		return
	}
	Data(w, http.StatusOK, map[string]any{"deleted": true})
}

// backupError 把备份服务的错误映射为统一的 HTTP 语义。
func (h *Handlers) backupError(w http.ResponseWriter, op string, backupErr error) {
	switch {
	case errors.Is(backupErr, backup.ErrInvalidName):
		err(w, http.StatusBadRequest, "bad_request", "备份名称无效")
	case errors.Is(backupErr, backup.ErrNotFound):
		NotFound(w)
	default:
		h.log.Error("backup "+op+" failed", "err", backupErr)
		err(w, http.StatusInternalServerError, "internal_error", "备份操作失败")
	}
}
