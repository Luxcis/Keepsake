// Package handlers 实现 HTTP JSON 接口。
// 统一响应：成功 {"data": ...}；失败 {"error": {"code": ..., "message": ...}}。
package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"keepsake/internal/auth"
	"keepsake/internal/store"
)

type errorMessage struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload != nil {
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			slog.Error("encode json", "err", err)
		}
	}
}

type envelope struct {
	Data any `json:"data"`
}

func Data(w http.ResponseWriter, status int, data any) {
	JSON(w, status, envelope{Data: data})
}

type errorEnvelope struct {
	Error errorMessage `json:"error"`
}

func err(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, errorEnvelope{Error: errorMessage{Code: code, Message: message}})
}

func NotFound(w http.ResponseWriter) {
	err(w, http.StatusNotFound, "not_found", "资源不存在或不可见")
}

// Forbidden：403 状态导出形态（供 server 中间件的 CSRF 拒绝使用）。
func Forbidden(w http.ResponseWriter, code, message string) {
	err(w, http.StatusForbidden, code, message)
}

// gateRequired：门禁未通过 —— 所有私人内容读接口在 401 时使用该语义码。
func gateRequired(w http.ResponseWriter) {
	err(w, http.StatusUnauthorized, "gate_required", "请先输入访问口令解锁本站内容")
}

// authRequired：管理员会话缺失/失效。
func authRequired(w http.ResponseWriter) {
	err(w, http.StatusUnauthorized, "auth_required", "管理员未登录或会话已过期")
}

// backendUnavailable：失败关闭（计划 §3.2：Redis 不可用时认证与管理写入拒绝）。
func backendUnavailable(w http.ResponseWriter) {
	err(w, http.StatusServiceUnavailable, "backend_unavailable",
		"认证/会话服务暂时不可用，请稍后再试")
}

// NotImplemented：为尚未实现阶段的接口提供明确的 501 语义，便于联调时一眼分辨。
func NotImplemented(w http.ResponseWriter, message string) {
	err(w, http.StatusNotImplemented, "not_implemented", message)
}

// mapStoreErr 把 store 返回的错误统一映射为 HTTP 语义。
func mapStoreErr(w http.ResponseWriter, storeErr error) {
	switch {
	case storeErr == nil:
	case errors.Is(storeErr, store.ErrNotFound):
		NotFound(w)
	default:
		slog.Error("store error", "err", storeErr)
		err(w, http.StatusInternalServerError, "internal_error", "服务内部错误")
	}
}

const (
	maxJSONBody = 1 << 19 // 512KiB：仅承载 JSON 内容；照片走 COS 直传
)

// decodeJSON 严格绑定 JSON 请求体（限制大小 + 语法 + 空体拒绝）。
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	dec := json.NewDecoder(r.Body)
	if derr := dec.Decode(v); derr != nil {
		switch {
		case errors.Is(derr, io.EOF):
			err(w, http.StatusBadRequest, "bad_request", "请求体为空，期望 JSON 内容")
		default:
			err(w, http.StatusBadRequest, "bad_request", "JSON 解析失败: "+derr.Error())
		}
		return false
	}
	// 拒绝尾部多余数据（如两个 JSON 对象拼接）。
	if dec.More() {
		err(w, http.StatusBadRequest, "bad_request", "请求体包含多余内容")
		return false
	}
	return true
}

// sessionErrToHTTP：会话层错误 → 统一 HTTP 语义（供中间件复用）。
// noSession 回调类型故意避免 http.HandlerFunc（它签名带 *http.Request，语义冗余）。
func sessionErrToHTTP(w http.ResponseWriter, sessionErr error, noSession func(http.ResponseWriter)) {
	switch {
	case sessionErr == nil:
	case errors.Is(sessionErr, auth.ErrNoSession):
		noSession(w)
	default:
		// Redis 已配置但不可用等后端故障 → 失败关闭。
		backendUnavailable(w)
	}
}
