package tokenrw

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const probeCookieName = CookieNamePrefix + "Probe-8b54eed2688b"

// TransferMiddleware 根据请求中的 Cookie 和 Token-Transfer 头决定令牌传输方式，
// 并将结果存入 context。应该应用于所有用户可能直接访问的路由。
func TransferMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		transfer := InlineTransfer
		forbidInline := false

		// 检查已有 Cookie 中是否包含令牌前缀
		for _, cookie := range r.Cookies() {
			if strings.HasPrefix(cookie.Name, CookieNamePrefix) {
				transfer = CookieTransfer
				if cookie.Name != probeCookieName {
					// 传输了真实令牌
					forbidInline = true
					break
				}
			}
		}

		// 处理 Token-Transfer 请求头
		switch h := r.Header.Get("Token-Transfer"); h {
		case "inline":
			transfer = InlineTransfer
		case "cookie":
			transfer = CookieTransfer
		case "":
			if r.Method == http.MethodGet {
				// 设置探测 Cookie，用于判断客户端是否支持 Cookie 传输
				http.SetCookie(w, &http.Cookie{
					Name:     probeCookieName,
					Value:    "1",
					MaxAge:   31536000,
					Path:     "/",
					Secure:   true,
					HttpOnly: true,
					SameSite: http.SameSiteStrictMode,
				})
			}
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(struct {
				Err string `json:"error"`
			}{
				Err: fmt.Sprintf("Token-Transfer header only accepts 'inline' or 'cookie', got '%s'", h),
			})
			return
		}

		// 禁止在已存在令牌 Cookie 时使用 inline 传输
		if forbidInline && transfer == InlineTransfer {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(struct {
				Err string `json:"error"`
			}{
				Err: "inline Token-Transfer not allowed when cookie already contains token",
			})
			return
		}

		// 将传输方式存入 context 并继续处理
		ctx := WithTransfer(r.Context(), transfer)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// CookiesMiddleware 提供通过 context 读写 Cookie 的能力，适用于需要操作令牌的路由。
func CookiesMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		adapter := cookiesAdapter{
			w: w,
			r: r,
		}
		ctx := WithCookies(r.Context(), adapter)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// cookiesAdapter 实现 Cookies 接口，基于 http.ResponseWriter 和 *http.Request 进行 Cookie 操作。
type cookiesAdapter struct {
	w http.ResponseWriter
	r *http.Request
}

// Get 实现 Cookies.Get，从请求中读取 Cookie。
func (c cookiesAdapter) Get(name string) (string, error) {
	cookie, err := c.r.Cookie(name)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

// Set 实现 Cookies.Set，通过 http.SetCookie 写入响应。
func (c cookiesAdapter) Set(name string, value string, maxAge int, path string, domain string, secure bool, httpOnly bool, sameSite http.SameSite) {
	http.SetCookie(c.w, &http.Cookie{
		Name:     name,
		Value:    value,
		MaxAge:   maxAge,
		Path:     path,
		Domain:   domain,
		Secure:   secure,
		HttpOnly: httpOnly,
		SameSite: sameSite,
	})
}

// 确保 stdCookiesAdapter 实现了 Cookies 接口
var _ Cookies = cookiesAdapter{}
