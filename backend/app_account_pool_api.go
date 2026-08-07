package backend

import (
	"ant-chrome/backend/internal/accountpool"
)

// 类型别名，供 Wails 前端绑定使用
type Account = accountpool.Account
type AccountInput = accountpool.AccountInput

// AccountPoolList 获取账号列表，status 可为空表示不过滤。
// platform 参数已废弃（平台归属并入 tags，服务即标签），仅为保持绑定签名兼容而保留，传入即忽略。
func (a *App) AccountPoolList(platform string, status string) ([]Account, error) {
	_ = platform // 平台过滤已废弃：服务即标签
	if a.accountPool == nil {
		return nil, errAccountPoolUnavailable
	}
	items, err := a.accountPool.List(accountpool.AccountFilter{Status: status})
	if err != nil {
		return nil, err
	}
	out := make([]Account, 0, len(items))
	for _, item := range items {
		out = append(out, *item)
	}
	return out, nil
}

// AccountPoolGet 查询单个账号
func (a *App) AccountPoolGet(accountId string) (*Account, error) {
	if a.accountPool == nil {
		return nil, errAccountPoolUnavailable
	}
	return a.accountPool.Get(accountId)
}

// AccountPoolCreate 创建账号；input.BoundProfileId 非空时绑定到对应实例。
// 不再写入 platform（incoming platform 由 JSON 解码忽略）：平台归属请使用 tags。
func (a *App) AccountPoolCreate(input AccountInput) (*Account, error) {
	if a.accountPool == nil {
		return nil, errAccountPoolUnavailable
	}
	return a.accountPool.Create(input)
}

// AccountPoolUpdate 更新账号。同样不再写入 platform。
func (a *App) AccountPoolUpdate(accountId string, input AccountInput) (*Account, error) {
	if a.accountPool == nil {
		return nil, errAccountPoolUnavailable
	}
	return a.accountPool.Update(accountId, input)
}

// AccountPoolDelete 软删除账号
func (a *App) AccountPoolDelete(accountId string) error {
	if a.accountPool == nil {
		return errAccountPoolUnavailable
	}
	return a.accountPool.Delete(accountId)
}

// AccountPoolCooldownByProxy 将绑定到指定代理的所有账号置为冷却（cooldownSec<=0 默认 3600）。
// 返回受影响的账号 ID 列表。供代理失败自动冷却流程与 GUI 手动触发使用。
func (a *App) AccountPoolCooldownByProxy(proxyId string, cooldownSec int) ([]string, error) {
	if a.accountPool == nil {
		return nil, errAccountPoolUnavailable
	}
	return a.accountPool.CooldownAccountsByProxy(proxyId, cooldownSec)
}

var errAccountPoolUnavailable = &accountPoolUnavailableError{}

type accountPoolUnavailableError struct{}

func (e *accountPoolUnavailableError) Error() string { return "account pool is not available" }
