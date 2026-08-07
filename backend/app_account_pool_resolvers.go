package backend

import (
	"strings"
)

// ProxyIDForProfile 实现 accountpool.ProxyResolver：解析绑定实例当前使用的代理 ID。
// 优先 profile.ProxyBindName -> 代理名匹配，回退 profile.ProxyId，再回退 account.ProxyID 由上层处理。
// 供代理失败冷却（CooldownAccountsByProxy）按实例解析代理。
func (a *App) ProxyIDForProfile(profileID string) string {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" || a.browserMgr == nil {
		return ""
	}
	for _, p := range a.browserMgr.List() {
		if p.ProfileId != profileID {
			continue
		}
		if id := strings.TrimSpace(p.ProxyId); id != "" {
			return id
		}
		return ""
	}
	return ""
}
