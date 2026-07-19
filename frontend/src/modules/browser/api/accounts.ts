import type { Account, AccountInput } from '../types'
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