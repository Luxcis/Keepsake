package server

import (
	"net/http"
	"strings"

	"keepsake/internal/handlers"
)

// csrfSameOrigin：计划 §5.1（Cookie 认证场景使用 CSRF 防护）。
// 策略：不安全方法（POST/PUT/PATCH/DELETE）若携带 Origin / Referer，
// 必须与请求目标同源（Host + scheme，感知反代的 X-Forwarded-Proto）。
// 不携带时放行：能被跨站伪造的浏览器请求一定携带同源信息；
// 服务器间调用（curl/脚本）不带 Cookie 即无法登录态冒用。
// 与 SameSite=Lax Cookie 叠加，构成双层 CSRF 防线。
func csrfSameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isUnsafeMethod(r.Method) {
			origin := r.Header.Get("Origin")
			referer := r.Header.Get("Referer")
			switch {
			case origin != "":
				if !sameOrigin(origin, r) {
					handlers.Forbidden(w, "csrf_rejected", "跨站请求被拒绝")
					return
				}
			case referer != "":
				refOrigin, ok := parseOrigin(referer)
				if !ok || refOrigin != requestOrigin(r) {
					handlers.Forbidden(w, "csrf_rejected", "跨站请求被拒绝")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

const protoHeader = "X-Forwarded-Proto"

// requestOrigin 计算请求侧的 scheme://host。
func requestOrigin(r *http.Request) string {
	scheme := r.Header.Get(protoHeader)
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return scheme + "://" + r.Host
}

func sameOrigin(candidate string, r *http.Request) bool {
	return matchOrigin(candidate, r)
}

func matchOrigin(candidate string, r *http.Request) bool {
	origin, ok := parseOrigin(candidate)
	if !ok {
		return false
	}
	return strings.EqualFold(origin, requestOrigin(r))
}

// parseOrigin 从 Origin/Referer 中提取 scheme://host[:port]。
func parseOrigin(v string) (string, bool) {
	idx := strings.Index(v, "://")
	if idx <= 0 {
		return "", false
	}
	scheme := strings.ToLower(v[:idx])
	rest := v[idx+3:]
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	if rest == "" {
		return "", false
	}
	return scheme + "://" + rest, true
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}
