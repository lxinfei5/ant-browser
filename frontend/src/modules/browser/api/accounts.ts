import type { Account, AccountInput } from '../types'
import { getBindings, nowISOString } from './runtime'

// 本地 mock 账号存储，仅在无 Wails 绑定时使用
let mockAccounts: Account[] = []

export function getMockAccounts(): Account[] {
  return mockAccounts
}

export function setMockAccounts(next: Account[]): void {
  mockAccounts = next
}

// AccountPoolList 获取账号列表；platform 参数后端已废弃（平台并入 tags），这里始终传 ''，status 可为空表示不过滤
export async function listAccounts(status = ''): Promise<Account[]> {
  const bindings: any = await getBindings()
  if (bindings?.AccountPoolList) {
    return (await bindings.AccountPoolList('', status)) || []
  }
  return mockAccounts.filter((item) => {
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
    accountRef: input.accountRef,
    email: input.email,
    phone: input.phone,
    boundProfileId: input.boundProfileId,
    proxyId: input.proxyId,
    status: input.status || 'active',
    notes: input.notes,
    tags: input.tags || [],
    groupId: input.groupId,
    credential: input.credential || {},
    metadata: input.metadata || {},
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

// AccountPoolCooldownByProxy 将绑定到指定代理的账号置为冷却，返回受影响账号 ID。
// 供代理失败自动冷却流程使用（非账号管理 UI 入口）。
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

// 便捷工具：按 boundProfileId 建立映射
// 注意:同一 profileId 只保留列表首条;后端 DAO 按 created_at ASC 排序,首条实为「最旧」一条账号。
// 因此一个实例绑定多个账号时,只有其一的标签参与标签册反查/筛选——已知限制,非主路径,暂未处理。
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

// 账号状态选项 / 展示辅助（active | disabled | cooldown）
export const ACCOUNT_STATUS_OPTIONS = [
  { value: '', label: '全部状态' },
  { value: 'active', label: '正常' },
  { value: 'disabled', label: '已停用' },
  { value: 'cooldown', label: '冷却中' },
]

export function accountStatusLabel(status: string): string {
  switch (status) {
    case 'active':
      return '正常'
    case 'disabled':
      return '已停用'
    case 'cooldown':
      return '冷却中'
    default:
      return status || '-'
  }
}

export function accountStatusVariant(status: string): 'default' | 'success' | 'error' | 'warning' | 'info' {
  switch (status) {
    case 'active':
      return 'success'
    case 'cooldown':
      return 'warning'
    case 'disabled':
      return 'default'
    default:
      return 'default'
  }
}
