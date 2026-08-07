import { useCallback, useEffect, useMemo, useState } from 'react'
import { Mail, Phone, Plus, RefreshCw, Search, Trash2, User, UserCircle } from 'lucide-react'
import {
  Badge,
  Button,
  Card,
  ConfirmModal,
  FormItem,
  Input,
  Modal,
  Select,
  Textarea,
  toast,
} from '../../../shared/components'
import {
  ACCOUNT_STATUS_OPTIONS,
  accountStatusLabel,
  accountStatusVariant,
  createAccount,
  deleteAccount,
  fetchBrowserProfiles,
  fetchKnownTags,
  listAccounts,
  startBrowserInstance,
  updateAccount,
} from '../api'
import type { Account, AccountInput, BrowserProfile } from '../types'
import { TagInput } from '../components/TagInput'
import { mergeTags, normalizeTag, tagsContain } from '../utils/tagMatch'

interface AccountFormState {
  accountName: string
  email: string
  phone: string
  accountRef: string
  tags: string[]
  boundProfileId: string
  notes: string
  status: string
}

const EMPTY_FORM: AccountFormState = {
  accountName: '',
  email: '',
  phone: '',
  accountRef: '',
  tags: [],
  boundProfileId: '',
  notes: '',
  status: 'active',
}

// 表单内可选状态（不含「全部」占位）
const FORM_STATUS_OPTIONS = ACCOUNT_STATUS_OPTIONS.filter((opt) => opt.value !== '')

function formatTime(value?: string): string {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString('zh-CN')
}

export function AccountsPage() {
  const [accounts, setAccounts] = useState<Account[]>([])
  const [profiles, setProfiles] = useState<BrowserProfile[]>([])
  const [knownTags, setKnownTags] = useState<string[]>([])
  const [loading, setLoading] = useState(true)

  // 过滤条件
  const [search, setSearch] = useState('')
  const [filterTag, setFilterTag] = useState('')
  const [filterStatus, setFilterStatus] = useState('')

  // 新建 / 编辑
  const [editorOpen, setEditorOpen] = useState(false)
  const [editing, setEditing] = useState<Account | null>(null)
  const [form, setForm] = useState<AccountFormState>(EMPTY_FORM)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  // 删除
  const [deleteTarget, setDeleteTarget] = useState<Account | null>(null)
  const [deleting, setDeleting] = useState(false)

  // 打开浏览器
  const [openingId, setOpeningId] = useState<string | null>(null)

  const profileNameById = useMemo(() => {
    const map = new Map<string, string>()
    for (const profile of profiles) {
      if (profile.profileId) {
        map.set(profile.profileId, profile.profileName || profile.profileId)
      }
    }
    return map
  }, [profiles])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [list, profileList, registryTags] = await Promise.all([
        listAccounts(),
        fetchBrowserProfiles().catch(() => [] as BrowserProfile[]),
        fetchKnownTags().catch(() => [] as string[]),
      ])
      setAccounts(list)
      setProfiles(profileList)
      setKnownTags(registryTags)
    } catch (error: any) {
      toast.error(error?.message || '加载账号失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  // 服务标签建议源：注册表 + 所有账号已用标签（标签册三源归一口径）
  const tagSuggestions = useMemo(() => {
    return mergeTags(knownTags, accounts.flatMap((account) => account.tags || []))
  }, [knownTags, accounts])

  const filteredAccounts = useMemo(() => {
    const query = normalizeTag(search)
    return accounts.filter((account) => {
      if (filterStatus && account.status !== filterStatus) return false
      if (filterTag && !tagsContain(account.tags, filterTag)) return false
      if (query) {
        const haystacks = [
          account.accountName,
          account.email,
          account.phone,
          account.accountRef,
          ...(account.tags || []),
        ]
        const hit = haystacks.some((value) => normalizeTag(String(value || '')).includes(query))
        if (!hit) return false
      }
      return true
    })
  }, [accounts, search, filterTag, filterStatus])

  const openCreate = () => {
    setEditing(null)
    setForm(EMPTY_FORM)
    setFormError('')
    setEditorOpen(true)
  }

  const openEdit = (account: Account) => {
    setEditing(account)
    setForm({
      accountName: account.accountName || '',
      email: account.email || '',
      phone: account.phone || '',
      accountRef: account.accountRef || '',
      tags: account.tags || [],
      boundProfileId: account.boundProfileId || '',
      notes: account.notes || '',
      status: account.status || 'active',
    })
    setFormError('')
    setEditorOpen(true)
  }

  const handleSubmit = async () => {
    const accountName = form.accountName.trim()
    if (!accountName) {
      setFormError('请输入账号名称')
      return
    }
    const input: AccountInput = {
      accountName,
      accountRef: form.accountRef.trim(),
      email: form.email.trim(),
      phone: form.phone.trim(),
      boundProfileId: form.boundProfileId,
      // 编辑时保留后端维护的代理/分组/凭证/元数据，避免被表单清空
      proxyId: editing?.proxyId || '',
      status: form.status || 'active',
      notes: form.notes.trim(),
      tags: form.tags,
      groupId: editing?.groupId || '',
      credential: editing?.credential || {},
      metadata: editing?.metadata || {},
    }
    setSaving(true)
    setFormError('')
    try {
      if (editing) {
        await updateAccount(editing.accountId, input)
        toast.success(`已保存：${accountName}`)
      } else {
        await createAccount(input)
        toast.success(`已创建：${accountName}`)
      }
      setEditorOpen(false)
      await load()
    } catch (error: any) {
      // 后端返回友好的中文校验错误（如「邮箱已被另一个账号使用」），直接透出
      const message = typeof error === 'string' ? error : error?.message || '保存失败'
      setFormError(message)
      toast.error(message)
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      await deleteAccount(deleteTarget.accountId)
      toast.success(`已删除：${deleteTarget.accountName}`)
      setDeleteTarget(null)
      await load()
    } catch (error: any) {
      toast.error(error?.message || '删除失败')
    } finally {
      setDeleting(false)
    }
  }

  // 打开该账号绑定的浏览器实例（个人「打开这个账号的浏览器」）
  const handleOpenBrowser = async (account: Account) => {
    const boundId = (account.boundProfileId || '').trim()
    if (!boundId) {
      toast.error('该账号未绑定实例')
      return
    }
    if (profiles.length > 0 && !profileNameById.has(boundId)) {
      toast.error('绑定的实例当前未加载，请刷新实例列表或重启应用后重试')
      return
    }
    setOpeningId(account.accountId)
    try {
      const profile = await startBrowserInstance(boundId)
      if (profile) {
        toast.success(`已打开：${account.accountName}`)
      } else {
        toast.error('启动失败')
      }
    } catch (error: any) {
      toast.error(error?.message || '启动失败')
    } finally {
      setOpeningId(null)
    }
  }

  const boundProfileName = (account: Account): string => {
    const boundId = (account.boundProfileId || '').trim()
    if (!boundId) return '未绑定'
    return profileNameById.get(boundId) || `${boundId.slice(0, 8)}…`
  }

  return (
    <div className="space-y-5 animate-fade-in">
      <Card
        title="账号管理"
        subtitle="管理你的登录身份：一个身份可关联多个服务标签，并绑定到专属浏览器实例"
        actions={
          <div className="flex items-center gap-2">
            <div className="relative">
              <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--text-muted)]" />
              <Input
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="搜索名称/邮箱/用户名/标签..."
                className="w-56 pl-8"
              />
            </div>
            <Select
              value={filterTag}
              onChange={(e) => setFilterTag(e.target.value)}
              className="w-36"
              options={[{ value: '', label: '全部服务' }, ...tagSuggestions.map((t) => ({ value: t, label: t }))]}
            />
            <Select
              value={filterStatus}
              onChange={(e) => setFilterStatus(e.target.value)}
              className="w-32"
              options={ACCOUNT_STATUS_OPTIONS}
            />
            <Button variant="secondary" size="sm" onClick={() => void load()}>
              <RefreshCw className="h-3.5 w-3.5" />
              刷新
            </Button>
            <Button size="sm" onClick={openCreate}>
              <Plus className="h-3.5 w-3.5" />
              新建账号
            </Button>
          </div>
        }
      >
        {loading ? (
          <div className="flex items-center justify-center py-16 text-[13px] text-[var(--text-muted)]">加载中...</div>
        ) : filteredAccounts.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 py-16 text-center">
            <UserCircle className="h-8 w-8 text-[var(--text-muted)]" />
            <p className="text-[13px] text-[var(--text-muted)]">
              {accounts.length === 0 ? '还没有账号，点击右上角「新建账号」开始' : '没有匹配的账号'}
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
            {filteredAccounts.map((account) => (
              <div
                key={account.accountId}
                className="flex flex-col gap-3 rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--bg-raised)] p-4 shadow-[var(--shadow-xs)] transition-all duration-150 hover:border-[var(--border-strong)] hover:shadow-[var(--shadow-sm)]"
              >
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <div className="truncate text-[14px] font-semibold text-[var(--text-primary)]" title={account.accountName}>
                      {account.accountName || '-'}
                    </div>
                    {account.accountRef && (
                      <div className="mt-0.5 flex items-center gap-1 text-xs text-[var(--text-muted)]" title={account.accountRef}>
                        <User className="h-3 w-3 shrink-0" />
                        <span className="truncate">{account.accountRef}</span>
                      </div>
                    )}
                  </div>
                  <Badge variant={accountStatusVariant(account.status)} dot size="sm">
                    {accountStatusLabel(account.status)}
                  </Badge>
                </div>

                {(account.tags || []).length > 0 && (
                  <div className="flex flex-wrap gap-1" title={`关联服务：${account.tags.join('、')}`}>
                    {account.tags.map((tag) => (
                      <Badge variant="info" size="sm" key={tag}>
                        {tag}
                      </Badge>
                    ))}
                  </div>
                )}

                <div className="flex flex-col gap-1 text-xs text-[var(--text-secondary)]">
                  {account.email && (
                    <div className="flex items-center gap-1.5" title={account.email}>
                      <Mail className="h-3 w-3 shrink-0 text-[var(--text-muted)]" />
                      <span className="truncate">{account.email}</span>
                    </div>
                  )}
                  {account.phone && (
                    <div className="flex items-center gap-1.5" title={account.phone}>
                      <Phone className="h-3 w-3 shrink-0 text-[var(--text-muted)]" />
                      <span className="truncate">{account.phone}</span>
                    </div>
                  )}
                  <div className="flex items-center gap-1.5">
                    <span className="shrink-0 text-[var(--text-muted)]">实例</span>
                    <span className="truncate">{boundProfileName(account)}</span>
                  </div>
                </div>

                <div className="mt-auto flex items-center justify-between gap-2 border-t border-[var(--border-subtle)] pt-2.5">
                  <span className="text-[11px] text-[var(--text-muted)]" title={`创建：${formatTime(account.createdAt)}`}>
                    更新 {formatTime(account.updatedAt)}
                  </span>
                  <div className="flex items-center gap-1.5">
                    <Button
                      size="sm"
                      onClick={() => void handleOpenBrowser(account)}
                      disabled={!account.boundProfileId || openingId === account.accountId}
                      loading={openingId === account.accountId}
                    >
                      打开浏览器
                    </Button>
                    <Button size="sm" variant="secondary" onClick={() => openEdit(account)}>
                      编辑
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="px-2 text-[var(--danger)]"
                      title="删除"
                      onClick={() => setDeleteTarget(account)}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Modal
        open={editorOpen}
        onClose={() => setEditorOpen(false)}
        title={editing ? '编辑账号' : '新建账号'}
        width="560px"
        footer={
          <>
            <Button variant="secondary" onClick={() => setEditorOpen(false)} disabled={saving}>
              取消
            </Button>
            <Button onClick={() => void handleSubmit()} loading={saving} disabled={saving}>
              {editing ? '保存' : '创建'}
            </Button>
          </>
        }
      >
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <FormItem label="账号名称" required error={formError && !form.accountName.trim() ? formError : undefined}>
            <Input
              value={form.accountName}
              onChange={(e) => setForm((prev) => ({ ...prev, accountName: e.target.value }))}
              placeholder="例如：我的 Google 主号"
            />
          </FormItem>
          <FormItem label="用户名" hint="登录用户名 / UID（身份锚点）">
            <Input
              value={form.accountRef}
              onChange={(e) => setForm((prev) => ({ ...prev, accountRef: e.target.value }))}
              placeholder="请输入用户名"
            />
          </FormItem>
          <FormItem label="邮箱">
            <Input
              value={form.email}
              onChange={(e) => setForm((prev) => ({ ...prev, email: e.target.value }))}
              placeholder="name@example.com"
            />
          </FormItem>
          <FormItem label="手机号">
            <Input
              value={form.phone}
              onChange={(e) => setForm((prev) => ({ ...prev, phone: e.target.value }))}
              placeholder="选填"
            />
          </FormItem>
          <FormItem label="绑定实例" className="md:col-span-2" hint="该账号在哪个浏览器实例中打开">
            <Select
              value={form.boundProfileId}
              onChange={(e) => setForm((prev) => ({ ...prev, boundProfileId: e.target.value }))}
              options={[
                { value: '', label: '不绑定实例' },
                ...profiles.map((p) => ({ value: p.profileId, label: p.profileName || p.profileId })),
              ]}
            />
          </FormItem>
          <FormItem label="关联服务标签" className="md:col-span-2" hint="一个身份可挂多个服务，如 google / gpt / x">
            <TagInput
              value={form.tags}
              onChange={(tags) => setForm((prev) => ({ ...prev, tags }))}
              suggestions={tagSuggestions}
              placeholder="输入服务标签后按回车"
            />
          </FormItem>
          <FormItem label="状态">
            <Select
              value={form.status}
              onChange={(e) => setForm((prev) => ({ ...prev, status: e.target.value }))}
              options={FORM_STATUS_OPTIONS}
            />
          </FormItem>
          <FormItem label="备注" className="md:col-span-2">
            <Textarea
              value={form.notes}
              onChange={(e) => setForm((prev) => ({ ...prev, notes: e.target.value }))}
              rows={2}
              placeholder="备注信息"
            />
          </FormItem>
        </div>
        {formError && form.accountName.trim() && (
          <p className="mt-3 text-xs text-[var(--danger)]">{formError}</p>
        )}
      </Modal>

      <ConfirmModal
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => void handleDelete()}
        title="删除账号"
        confirmText={deleting ? '删除中...' : '删除'}
        danger
        content={
          <p className="text-sm text-[var(--text-secondary)]">
            确认删除账号 <b>{deleteTarget?.accountName}</b>？此操作会从账号库中移除该身份，且不影响已绑定的浏览器实例。
          </p>
        }
      />
    </div>
  )
}
