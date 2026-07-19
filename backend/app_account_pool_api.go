package backend

import (
	"ant-chrome/backend/internal/accountpool"
)

// 类型别名，供 Wails 前端绑定使用
type Account = accountpool.Account
type AccountInput = accountpool.AccountInput

// AccountPoolList 获取账号池列表，platform/status 可为空表示不过滤
func (a *App) AccountPoolList(platform string, status string) ([]Account, error) {
	if a.accountPool == nil {
		return nil, errAccountPoolUnavailable
	}
	items, err := a.accountPool.List(accountpool.AccountFilter{Platform: platform, Status: status})
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

// AccountPoolCreate 创建账号；input.BoundProfileId 非空时绑定到对应实例
func (a *App) AccountPoolCreate(input AccountInput) (*Account, error) {
	if a.accountPool == nil {
		return nil, errAccountPoolUnavailable
	}
	return a.accountPool.Create(input)
}

// AccountPoolUpdate 更新账号
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

var errAccountPoolUnavailable = &accountPoolUnavailableError{}

type accountPoolUnavailableError struct{}

func (e *accountPoolUnavailableError) Error() string { return "account pool is not available" }