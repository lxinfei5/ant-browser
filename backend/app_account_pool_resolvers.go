package backend

import (
	"ant-chrome/backend/internal/accountpool"
	"ant-chrome/backend/internal/browser"
	"strconv"
	"strings"
	"sync/atomic"
)

// profileCounter 为批量导入生成的实例名提供单调递增序号，保证 {platform}-acc-{n} 唯一。
var profileCounter uint64

// CreateProfileForRow 实现 accountpool.ProfileFactory：为批量导入行创建一个绑定实例。
// 实例名遵循 {platform}-acc-{n} 约定。
func (a *App) CreateProfileForRow(row accountpool.AccountBatchRow) (string, error) {
	platform := strings.TrimSpace(row.Platform)
	if platform == "" {
		platform = "acc"
	}
	n := atomic.AddUint64(&profileCounter, 1)
	name := platform + "-acc-" + strconv.FormatUint(n, 10)
	profile, err := a.browserMgr.Create(browser.ProfileInput{
		ProfileName: name,
		Tags:        append([]string{}, row.Tags...),
	})
	if err != nil {
		return "", err
	}
	return profile.ProfileId, nil
}

// ProxyIDForName 实现 accountpool.ProxyResolver：按名称唯一匹配代理，返回其 ID。
// 未找到或名称不唯一时返回空串（跳过代理绑定，与上游导入行为一致）。
func (a *App) ProxyIDForName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	key := strings.ToLower(name)
	proxies := a.getLatestProxies()
	var hit string
	matched := 0
	for _, p := range proxies {
		if strings.ToLower(strings.TrimSpace(p.ProxyName)) == key {
			hit = strings.TrimSpace(p.ProxyId)
			matched++
			if matched > 1 {
				return "" // 歧义，跳过
			}
		}
	}
	return hit
}

// ProxyIDForProfile 实现 accountpool.ProxyResolver：解析绑定实例当前使用的代理 ID。
// 优先 profile.ProxyBindName -> 代理名匹配，回退 profile.ProxyId，再回退 account.ProxyID 由上层处理。
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