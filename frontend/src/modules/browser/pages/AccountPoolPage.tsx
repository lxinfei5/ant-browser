import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Badge,
  Button,
  Card,
  ConfirmModal,
  FormItem,
  Input,
  Modal,
  Select,
  Table,
  type TableColumn,
  Textarea,
  toast,
} from '../../../shared/components'
import {
  ACCOUNT_PLATFORM_OPTIONS,
  ACCOUNT_STATUS_OPTIONS,
  batchImportAccounts,
  forceReleaseAccount,
  getAccountActiveLease,
  listAccounts,
  platformLabel,
} from '../api'
import type { Account, AccountBatchRow, AccountLease } from '../types'

interface AccountPoolRow {
  account: Account
  profileName: string
  lease: AccountLease | null
}

function parseCooldownRemaining(cooldownUntil: string): string {
  const t = (cooldownUntil || '').trim()
  if (!t) return ''
  const target = new Date(t).getTime()
  if (Number.isNaN(target)) return ''
  const diff = target - Date.now()
  if (diff <= 0) return ''
  const totalSec = Math.floor(diff / 1000)
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  const s = totalSec % 60
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

function statusVariant(status: string): 'default' | 'success' | 'error' | 'warning' | 'info' {
  switch (status) {
    case 'active':
      return 'success'
    case 'cooldown':
      return 'warning'
    case 'banned':
      return 'error'
    case 'need_login':
      return 'info'
    default:
      return 'default'
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case 'active':
      return '正常'
    case 'cooldown':
      return '冷却中'
    case 'banned':
      return '已封禁'
    case 'need_login':
      return '需登录'
    case 'disabled':
      return '已停用'
    default:
      return status || '-'
  }
}

export function AccountPoolPage() {
  const [accounts, setAccounts] = useState<Account[]>([])
  const [loading, setLoading] = useState(true)
  const [filterPlatform, setFilterPlatform] = useState('')
  const [filterStatus, setFilterStatus] = useState('')
  const [leaseMap, setLeaseMap] = useState<Record<string, AccountLease | null>>({})
  const [forceReleaseTarget, setForceReleaseTarget] = useState<Account | null>(null)
  const [forceReleaseResult, setForceReleaseResult] = useState('ok')

  // 批量导入
  const [importOpen, setImportOpen] = useState(false)
  const [importText, setImportText] = useState('')
  const [importing, setImporting] = useState(false)

  const loadAccounts = useCallback(async () => {
    setLoading(true)
    try {
      const list = await listAccounts(filterPlatform, filterStatus)
      setAccounts(list)
      // 并发查询每个账号的活跃租约
      const entries = await Promise.all(
        list.map(async (acc) => [acc.accountId, await getAccountActiveLease(acc.accountId)] as const),
      )
      const next: Record<string, AccountLease | null> = {}
      for (const [id, lease] of entries) {
        next[id] = lease
      }
      setLeaseMap(next)
    } catch (error: any) {
      toast.error(error?.message || '加载账号池失败')
    } finally {
      setLoading(false)
    }
  }, [filterPlatform, filterStatus])

  useEffect(() => {
    void loadAccounts()
  }, [loadAccounts])

  const rows: AccountPoolRow[] = useMemo(() => {
    return accounts.map((account) => ({
      account,
      profileName: account.boundProfileId ? account.boundProfileId.slice(0, 8) : '-',
      lease: leaseMap[account.accountId] ?? null,
    }))
  }, [accounts, leaseMap])

  const handleForceRelease = async () => {
    if (!forceReleaseTarget) return
    try {
      const res = await forceReleaseAccount(forceReleaseTarget.accountId, forceReleaseResult, 0)
      if (res && res.lease) {
        toast.success('已强制释放租约')
      } else {
        toast.success('账号当前无活跃租约')
      }
      setForceReleaseTarget(null)
      await loadAccounts()
    } catch (error: any) {
      toast.error(error?.message || '强制释放失败')
    }
  }

  const handleImport = async () => {
    const text = importText.trim()
    if (!text) {
      toast.error('请粘贴或上传 CSV 内容')
      return
    }
    const rows = parseCsvRows(text)
    if (rows.length === 0) {
      toast.error('未解析到有效行（首行需为表头：platform,username,proxy_name,notes,tags）')
      return
    }
    setImporting(true)
    try {
      const results = await batchImportAccounts(rows)
      const created = results.filter((r) => !r.error).length
      const failed = results.length - created
      if (failed === 0) {
        toast.success(`导入完成：成功 ${created} 个`)
      } else {
        toast.warning(`导入完成：成功 ${created} 个，失败 ${failed} 个`)
      }
      setImportOpen(false)
      setImportText('')
      await loadAccounts()
    } catch (error: any) {
      toast.error(error?.message || '批量导入失败')
    } finally {
      setImporting(false)
    }
  }

  const handleFileUpload = (file: File) => {
    const reader = new FileReader()
    reader.onload = () => {
      setImportText(String(reader.result || ''))
    }
    reader.readAsText(file)
  }

  const columns: TableColumn<AccountPoolRow>[] = [
    {
      key: 'accountName',
      title: '账号',
      render: (_v, row) => (
        <div className="flex flex-col">
          <span className="text-[var(--color-text-primary)] font-medium">{row.account.accountName || '-'}</span>
          <span className="text-xs text-[var(--color-text-muted)]">{row.account.accountRef || ''}</span>
        </div>
      ),
    },
    {
      key: 'platform',
      title: '平台',
      width: 110,
      render: (_v, row) => platformLabel(row.account.platform),
    },
    {
      key: 'status',
      title: '状态',
      width: 110,
      render: (_v, row) => (
        <Badge variant={statusVariant(row.account.status)} size="sm">
          {statusLabel(row.account.status)}
        </Badge>
      ),
    },
    {
      key: 'cooldownUntil',
      title: '冷却倒计时',
      width: 120,
      render: (_v, row) => {
        const remaining = parseCooldownRemaining(row.account.cooldownUntil)
        return remaining ? <span className="text-[var(--color-warning)]">{remaining}</span> : <span className="text-[var(--color-text-muted)]">-</span>
      },
    },
    {
      key: 'profileName',
      title: '绑定实例',
      width: 130,
      render: (_v, row) => (
        <span className="text-xs text-[var(--color-text-muted)]">{row.profileName}</span>
      ),
    },
    {
      key: 'lease',
      title: '租约',
      width: 110,
      render: (_v, row) =>
        row.lease ? (
          <Badge variant="info" size="sm">
            占用中
          </Badge>
        ) : (
          <span className="text-[var(--color-text-muted)]">空闲</span>
        ),
    },
    {
      key: 'lastUsedAt',
      title: '最后使用',
      width: 160,
      render: (_v, row) => (
        <span className="text-xs text-[var(--color-text-muted)]">
          {row.account.lastUsedAt ? new Date(row.account.lastUsedAt).toLocaleString() : '-'}
        </span>
      ),
    },
    {
      key: 'actions',
      title: '操作',
      width: 110,
      align: 'right',
      render: (_v, row) =>
        row.lease ? (
          <Button size="sm" variant="danger" onClick={() => setForceReleaseTarget(row.account)}>
            强制释放
          </Button>
        ) : (
          <span className="text-[var(--color-text-muted)] text-xs">-</span>
        ),
    },
  ]

  return (
    <div className="space-y-5 animate-fade-in">
      <Card
        title="账号池"
        subtitle="管理账号、查看租约占用与冷却状态，支持 CSV 批量导入"
        actions={
          <div className="flex items-center gap-2">
            <Select
              value={filterPlatform}
              onChange={(e) => setFilterPlatform(e.target.value)}
              className="w-36"
              options={[{ value: '', label: '全部平台' }, ...ACCOUNT_PLATFORM_OPTIONS]}
            />
            <Select
              value={filterStatus}
              onChange={(e) => setFilterStatus(e.target.value)}
              className="w-36"
              options={ACCOUNT_STATUS_OPTIONS}
            />
            <Button variant="secondary" size="sm" onClick={() => void loadAccounts()}>
              刷新
            </Button>
            <Button size="sm" onClick={() => setImportOpen(true)}>
              批量导入
            </Button>
          </div>
        }
      >
        <Table columns={columns} data={rows} rowKey={(row) => row.account.accountId} loading={loading} emptyText="暂无账号" />
      </Card>

      <Modal
        open={importOpen}
        onClose={() => setImportOpen(false)}
        title="CSV 批量导入账号"
        width="640px"
        footer={
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setImportOpen(false)} disabled={importing}>
              取消
            </Button>
            <Button onClick={() => void handleImport()} loading={importing} disabled={importing}>
              开始导入
            </Button>
          </div>
        }
      >
        <div className="space-y-3">
          <FormItem
            label="上传 CSV 文件"
            hint="可选，或直接在下方文本框粘贴"
          >
            <Input
              type="file"
              accept=".csv,text/csv"
              onChange={(e) => {
                const f = e.target.files?.[0]
                if (f) handleFileUpload(f)
              }}
            />
          </FormItem>
          <FormItem
            label="CSV 内容"
            hint="表头：platform,username,proxy_name,notes,tags（tags 用 | 分隔）"
          >
            <Textarea
              value={importText}
              onChange={(e) => setImportText(e.target.value)}
              rows={8}
              placeholder={`platform,username,proxy_name,notes,tags\nxhs,user01,proxy-a,,vip|xhs\nx,user02,,,`}
            />
          </FormItem>
        </div>
      </Modal>

      <ConfirmModal
        open={!!forceReleaseTarget}
        onClose={() => setForceReleaseTarget(null)}
        onConfirm={() => void handleForceRelease()}
        title="强制释放租约"
        confirmText="强制释放"
        danger
        content={
          <div className="space-y-3">
            <p className="text-sm text-[var(--color-text-secondary)]">
              确认强制释放账号 <b>{forceReleaseTarget?.accountName}</b> 当前持有的租约？若该租约由系统自动启动实例，会同时停止该实例。
            </p>
            <FormItem label="释放结果">
              <Select
                value={forceReleaseResult}
                onChange={(e) => setForceReleaseResult(e.target.value)}
                className="w-full"
                options={[
                  { value: 'ok', label: 'ok（恢复正常）' },
                  { value: 'risk', label: 'risk（冷却，默认 60min）' },
                  { value: 'ban', label: 'ban（封禁）' },
                  { value: 'need_login', label: 'need_login（需登录）' },
                ]}
              />
            </FormItem>
          </div>
        }
      />
    </div>
  )
}

// parseCsvRows 解析简易 CSV：首行为表头，支持逗号分隔；tags 列以 | 分隔多个标签。
function parseCsvRows(text: string): AccountBatchRow[] {
  const lines = text.split(/\r?\n/).filter((l) => l.trim() !== '')
  if (lines.length < 2) return []
  const headers = splitCsvLine(lines[0]).map((h) => h.trim().toLowerCase())
  const idxPlatform = headers.indexOf('platform')
  const idxUsername = headers.indexOf('username')
  const idxProxy = headers.indexOf('proxy_name')
  const idxNotes = headers.indexOf('notes')
  const idxTags = headers.indexOf('tags')
  const rows: AccountBatchRow[] = []
  for (let i = 1; i < lines.length; i++) {
    const cols = splitCsvLine(lines[i])
    const platform = (idxPlatform >= 0 ? cols[idxPlatform] : '').trim()
    const username = (idxUsername >= 0 ? cols[idxUsername] : '').trim()
    if (!platform || !username) continue
    const proxyName = (idxProxy >= 0 ? cols[idxProxy] : '').trim()
    const notes = (idxNotes >= 0 ? cols[idxNotes] : '').trim()
    const tagsRaw = (idxTags >= 0 ? cols[idxTags] : '').trim()
    const tags = tagsRaw ? tagsRaw.split('|').map((t) => t.trim()).filter(Boolean) : []
    rows.push({ platform, username, proxyName, notes, tags })
  }
  return rows
}

function splitCsvLine(line: string): string[] {
  // 简易解析：不支持引号转义，按逗号拆分
  return line.split(',').map((c) => c.trim())
}