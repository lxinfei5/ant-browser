import type { Account, AccountInput, AccountLease, AccountBatchRow, AccountBatchImportResult } from '../types'
import { getBindings, nowISOString } from './runtime'

// 本地 mock 账号池，仅在无 Wails 绑定时使用
let mockAccounts: Account[] = []

export function getMockAccounts(): Account[] {
  return mockAccounts
}

export function setMockAccounts(next: Account[]): void {
  mockAccounts = next
}

// AccountPoolList 获取账号池列表，platform/status 可为空表示不过滤
export async function listAccounts(platform = '', status = ''): Promise<Account[]> {
  const bindings: any = await getBindings()
  if (bindings?.AccountPoolList) {
    return (await bindings.AccountPoolList(platform, status)) || []
  }
  return mockAccounts.filter((item) => {
    if (platform && item.platform !== platform) return false
    if (status && item.status !== status) return false
    return true
  })
}

export async function getAccount(accountId: string): Promise<Account | null> {
  const bindings: any = await getBindings()
  if (bindings?.AccountPoolGet) {
    return (await bindings.AccountPoolGet(accountId)) || null
  }
  return mockAccounts.find((item) => item.accountId === accountId) || null
}

// AccountPoolCreate 创建账号；input.boundProfileId 非空时绑定到对应实例
export async function createAccount(input: AccountInput): Promise<Account | null> {
  const bindings: any = await getBindings()
  if (bindings?.AccountPoolCreate) {
    return (await bindings.AccountPoolCreate(input)) || null
  }
  const now = nowISOString()
  const account: Account = {
    accountId: `mock-${Date.now()}`,
    accountName: input.accountName,
    platform: input.platform,
    accountRef: input.accountRef,
    boundProfileId: input.boundProfileId,
    proxyId: input.proxyId,
    status: input.status || 'active',
    cooldownUntil: input.cooldownUntil,
    notes: input.notes,
    tags: input.tags || [],
    groupId: input.groupId,
    credential: input.credential || {},
    metadata: input.metadata || {},
    iconKind: input.iconKind || '',
    iconColor: input.iconColor || '',
    iconText: input.iconText || '',
    iconImage: input.iconImage || '',
    lastUsedAt: '',
    createdAt: now,
    updatedAt: now,
    deletedAt: '',
  }
  mockAccounts = [account, ...mockAccounts]
  return account
}

export async function updateAccount(accountId: string, input: AccountInput): Promise<Account | null> {
  const bindings: any = await getBindings()
  if (bindings?.AccountPoolUpdate) {
    return (await bindings.AccountPoolUpdate(accountId, input)) || null
  }
  const index = mockAccounts.findIndex((item) => item.accountId === accountId)
  if (index === -1) return null
  const next = [...mockAccounts]
  next[index] = {
    ...next[index],
    ...input,
    tags: input.tags || [],
    updatedAt: nowISOString(),
  }
  mockAccounts = next
  return next[index]
}

export async function deleteAccount(accountId: string): Promise<boolean> {
  const bindings: any = await getBindings()
  if (bindings?.AccountPoolDelete) {
    await bindings.AccountPoolDelete(accountId)
    return true
  }
  mockAccounts = mockAccounts.filter((item) => item.accountId !== accountId)
  return true
}

// AccountPoolActiveLease 返回账号当前持有的 held 租约；无则返回 null。
export async function getAccountActiveLease(accountId: string): Promise<AccountLease | null> {
  const bindings: any = await getBindings()
  if (bindings?.AccountPoolActiveLease) {
    return (await bindings.AccountPoolActiveLease(accountId)) || null
  }
  return null
}

// AccountPoolForceRelease 强制释放账号当前 held 租约。
// result: ok | risk | ban | need_login；cooldownSec 仅对 risk 生效（<=0 默认 3600）。
export async function forceReleaseAccount(
  accountId: string,
  result = 'ok',
  cooldownSec = 0,
): Promise<{ lease: AccountLease | null; account: Account | null } | null> {
  const bindings: any = await getBindings()
  if (bindings?.AccountPoolForceRelease) {
    const [lease, account] = await bindings.AccountPoolForceRelease(accountId, result, cooldownSec)
    return { lease: lease || null, account: account || null }
  }
  return null
}

// AccountPoolBatchImport 批量导入账号，返回每行结果（含成功账号或失败原因）。
export async function batchImportAccounts(
  rows: AccountBatchRow[],
): Promise<AccountBatchImportResult[]> {
  const bindings: any = await getBindings()
  if (bindings?.AccountPoolBatchImport) {
    return (await bindings.AccountPoolBatchImport(rows)) || []
  }
  // mock：按行生成账号
  const results: AccountBatchImportResult[] = []
  for (const row of rows) {
    const account = await createAccount({
      accountName: row.username,
      platform: row.platform,
      accountRef: row.username,
      boundProfileId: '',
      proxyId: '',
      status: 'active',
      cooldownUntil: '',
      notes: row.notes,
      tags: row.tags || [],
      groupId: '',
      credential: {},
      metadata: {},
    })
    results.push({ row, account: account || undefined, error: account ? '' : 'create failed' })
  }
  return results
}

// AccountPoolCooldownByProxy 将绑定到指定代理的账号置为冷却，返回受影响账号 ID。
export async function cooldownAccountsByProxy(
  proxyId: string,
  cooldownSec = 3600,
): Promise<string[]> {
  const bindings: any = await getBindings()
  if (bindings?.AccountPoolCooldownByProxy) {
    return (await bindings.AccountPoolCooldownByProxy(proxyId, cooldownSec)) || []
  }
  return []
}

export interface SetAccountIconPayload {
  kind: string
  color: string
  text: string
  imageDataURL: string
}

// AccountPoolSetIcon 设置账号的 Dock 图标（kind: '' | color | text | image）。
export async function setAccountIcon(
  accountId: string,
  payload: SetAccountIconPayload,
): Promise<Account | null> {
  const bindings: any = await getBindings()
  if (bindings?.AccountPoolSetIcon) {
    return (
      (await bindings.AccountPoolSetIcon(
        accountId,
        payload.kind,
        payload.color,
        payload.text,
        payload.imageDataURL,
      )) || null
    )
  }
  // mock：直接更新本地账号图标字段
  const index = mockAccounts.findIndex((item) => item.accountId === accountId)
  if (index === -1) return null
  const next = [...mockAccounts]
  next[index] = {
    ...next[index],
    iconKind: payload.kind,
    iconColor: payload.color,
    iconText: payload.text,
    updatedAt: nowISOString(),
  }
  mockAccounts = next
  return next[index]
}

// BrowserProfileRebuildIcons 失效并重建全部 Dock 图标克隆（惰性，下次启动时重建）。
export async function rebuildProfileIcons(): Promise<boolean> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserProfileRebuildIcons) {
    await bindings.BrowserProfileRebuildIcons()
    return true
  }
  return false
}

// 便捷工具：按 boundProfileId 建立映射
export function indexAccountsByProfileId(accounts: Account[]): Map<string, Account> {
  const map = new Map<string, Account>()
  accounts.forEach((account) => {
    if (account.boundProfileId) {
      // 同一个 profile 只保留最新的一条
      if (!map.has(account.boundProfileId)) {
        map.set(account.boundProfileId, account)
      }
    }
  })
  return map
}

// 平台选项
export const ACCOUNT_PLATFORM_OPTIONS = [
  { value: 'xhs', label: '小红书' },
  { value: 'x', label: 'X (Twitter)' },
  { value: 'other', label: '其他' },
]

export const ACCOUNT_STATUS_OPTIONS = [
  { value: '', label: '全部账号状态' },
  { value: 'active', label: '正常' },
  { value: 'disabled', label: '已停用' },
]

export function platformLabel(platform: string): string {
  return ACCOUNT_PLATFORM_OPTIONS.find((opt) => opt.value === platform)?.label || platform || '-'
}