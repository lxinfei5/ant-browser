// Package netguard 提供 SSRF 防护：拦截指向私有/内网/链路本地/云元数据地址的服务端 fetch。
package netguard

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

// BypassEnv 为显式跳过 SSRF 防护的逃生阀（开发/调试用）。
const BypassEnv = "PROFILEPOOL_ALLOW_PRIVATE_FETCH"

// IsBlockedIP 判断 ip 是否属于被拦截的地址段：
//   - 回环 127.0.0.0/8 与 ::1
//   - 私有 10/8、172.16/12、192.168/16
//   - 链路本地 169.254.0.0/16（含云元数据 169.254.169.254）与 fe80::/10
//   - 唯一本地地址 fc00::/7
//   - 未指定 0.0.0.0 / ::
func IsBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() {
		return true
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.IsPrivate() {
		return true
	}
	if v6 := ip.To16(); v6 != nil {
		if v6[0]&0xfe == 0xfc { // fc00::/7
			return true
		}
	}
	return false
}

// AllowedBypass 返回是否设置了显式 SSRF 旁路（环境变量 PROFILEPOOL_ALLOW_PRIVATE_FETCH）。
func AllowedBypass() bool {
	return os.Getenv(BypassEnv) != ""
}

// GuardedDialContext 在连接前解析并校验目标地址，拒绝指向被拦截段的连接（防 DNS-rebinding）。
func GuardedDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	d := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	if AllowedBypass() {
		return d.DialContext(ctx, network, address)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if IsBlockedIP(ip.IP) {
			return nil, fmt.Errorf("SSRF 防护拦截：%s -> %s（私有/内网地址）", address, ip.IP.String())
		}
	}
	return d.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

// NewClient 返回带 SSRF 防护的 HTTP 客户端。
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{DialContext: GuardedDialContext},
	}
}