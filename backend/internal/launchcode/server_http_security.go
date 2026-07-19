package launchcode

import (
	"net/http"
	"net/url"
	"strings"
)

// isStateChangingMethod 判断是否为状态变更方法（需 CSRF / Content-Type 防护）。
func isStateChangingMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// hasBody 判断请求是否带 body（已知 Content-Length > 0；chunked/未知长度不强制，留作兼容）。
func hasBody(r *http.Request) bool {
	return r.ContentLength > 0
}

// isJSONContentType 判断 Content-Type 是否为 application/json（兼容 charset 等参数）。
func isJSONContentType(r *http.Request) bool {
	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if contentType == "" {
		return false
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	mediaType = strings.TrimPrefix(mediaType, "application/")
	return mediaType == "json"
}

// isSameOriginFetch 基于 Sec-Fetch-Site 判断是否为同源/同站请求。
// 仅 same-origin / same-site 视为同源；cross-site 或缺少该头（no-site）一律视为非同源。
func isSameOriginFetch(r *http.Request) bool {
	site := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
	switch site {
	case "same-origin", "same-site":
		return true
	}
	return false
}

// isLocalhostOrigin 判断 Origin/Host 头对应的主机是否为本机（用于 CDP 反向代理的来源白名单）。
func isLocalhostHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.Trim(host, "[]")
	switch host {
	case "", "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// isAllowedCDPOrigin 判断 CDP 反向代理请求的来源是否允许：
//   - 无 Origin 头（本地非浏览器客户端，如 puppeteer/curl）-> 允许；
//   - Origin 主机为本机（localhost/127.0.0.1/::1）-> 允许；
//   - 其它跨源（浏览器驻留 drive-by / DNS-rebinding）-> 拒绝。
func isAllowedCDPOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return isLocalhostHost(parsed.Hostname())
}