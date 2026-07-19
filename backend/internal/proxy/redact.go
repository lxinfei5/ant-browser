package proxy

import (
	"net/url"
	"regexp"
	"strings"
)

// userInfoRedactPattern 用于在无法被 net/url 解析的代理字符串中按模式脱敏：
// 形如 scheme://user:pass@host 的凭据段会被替换为 ***。
var userInfoRedactPattern = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)([^:/@]+):[^@]*@`)

// RedactProxyConfig 在日志/记录中脱敏代理配置字符串里的 userinfo（user:pass@ -> ***@）。
// 直接返回输入副本，绝不透传原始凭据。无法解析的字符串则按正则兜底脱敏。
func RedactProxyConfig(proxyConfig string) string {
	proxyConfig = strings.TrimSpace(proxyConfig)
	if proxyConfig == "" || strings.EqualFold(proxyConfig, "direct://") {
		return proxyConfig
	}

	if u, err := url.Parse(proxyConfig); err == nil && u.Scheme != "" && u.User != nil {
		redacted := *u
		redacted.User = url.User("***")
		return redacted.String()
	}

	// 兜底：按正则将 scheme://user:pass@ 替换为 scheme://***@
	return userInfoRedactPattern.ReplaceAllString(proxyConfig, "${1}***@")
}