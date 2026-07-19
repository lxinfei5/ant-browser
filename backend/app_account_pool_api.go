package backend

import (
	"ant-chrome/backend/internal/accountpool"
	"strings"
)

// 类型别名，供 Wails 前端绑定使用
type Account = accountpool.Account
type AccountInput = accountpool.AccountInput
type AccountLease = accountpool.Lease
type AccountBatchRow = accountpool.AccountBatchRow
type AccountBatchImportResult = accountpool.BatchImportResult

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

// AccountPoolActiveLease 返回账号当前持有的 held 租约；无则返回 nil。
func (a *App) AccountPoolActiveLease(accountId string) (*AccountLease, error) {
	if a.accountPool == nil {
		return nil, errAccountPoolUnavailable
	}
	return a.accountPool.GetActiveLease(accountId)
}

// AccountPoolForceRelease 强制释放账号当前的 held 租约。
// 若该租约是 auto_started（租约自动启动了实例），则停止绑定实例。
// result 为 ok/risk/ban/need_login，cooldownSec 仅对 risk 生效（<=0 时默认 3600）。
func (a *App) AccountPoolForceRelease(accountId string, result string, cooldownSec int) (*AccountLease, *Account, error) {
	if a.accountPool == nil {
		return nil, nil, errAccountPoolUnavailable
	}
	accountId = strings.TrimSpace(accountId)
	lease, err := a.accountPool.GetActiveLease(accountId)
	if err != nil {
		return nil, nil, err
	}
	if lease == nil {
		return nil, nil, nil // 无活跃租约，幂等返回
	}
	released, account, err := a.accountPool.Release(lease.LeaseID, result, cooldownSec)
	if err != nil {
		return nil, nil, err
	}
	// 自动启动的实例需要停止（与 lease release handler 逻辑一致）
	if released != nil && released.AutoStarted == 1 && strings.TrimSpace(released.ProfileID) != "" {
		_ = a.stopBoundProfile(released.ProfileID)
	}
	return released, account, nil
}

// stopBoundProfile 停止绑定实例，忽略错误（强制释放不应因停止失败而中断）。
func (a *App) stopBoundProfile(profileId string) error {
	_, err := a.StopInstance(profileId)
	return err
}

// AccountPoolBatchImport 批量导入账号（CSV 行）：为每行创建绑定实例、按名绑定代理、创建账号。
// 单行失败不中断整批，返回每行结果（含成功账号或失败原因）。
func (a *App) AccountPoolBatchImport(rows []AccountBatchRow) ([]AccountBatchImportResult, error) {
	if a.accountPool == nil {
		return nil, errAccountPoolUnavailable
	}
	return a.accountPool.BatchImport(rows), nil
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