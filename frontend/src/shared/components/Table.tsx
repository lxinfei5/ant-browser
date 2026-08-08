import { ReactNode } from 'react'
import clsx from 'clsx'
import { ArrowUp, ArrowDown, Inbox } from 'lucide-react'

export type SortOrder = 'asc' | 'desc' | undefined

export interface SorterResult {
  column: string
  order: SortOrder
}

export interface TableColumn<T> {
  key: string
  title: ReactNode
  width?: string | number
  align?: 'left' | 'center' | 'right'
  render?: (value: any, record: T, index: number) => ReactNode
  sortable?: boolean // 是否可排序
}

interface TableProps<T> {
  columns: TableColumn<T>[]
  data: T[]
  rowKey: string | ((record: T) => string)
  loading?: boolean
  emptyText?: string
  onRowClick?: (record: T) => void
  className?: string
  maxHeight?: string  // 表格最大高度，默认 'calc(100vh - 320px)'
  stickyHeader?: boolean  // 是否固定表头，默认 true
  onSort?: (sorterResult: SorterResult) => void // 排序变化回调
  sortColumn?: string // 当前排序的列
  sortOrder?: SortOrder // 当前排序方式
}

export function Table<T extends Record<string, any>>({
  columns,
  data,
  rowKey,
  loading = false,
  emptyText = '暂无数据',
  onRowClick,
  className,
  maxHeight = 'calc(100vh - 320px)',
  stickyHeader = true,
  onSort,
  sortColumn,
  sortOrder,
}: TableProps<T>) {
  const getRowKey = (record: T, index: number): string => {
    if (typeof rowKey === 'function') {
      return rowKey(record)
    }
    return record[rowKey] ?? index.toString()
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-16" style={{ maxHeight }}>
        <div className="flex flex-col items-center gap-3">
          <div className="w-6 h-6 border-2 border-[var(--color-border-default)] border-t-[var(--color-accent)] rounded-full animate-spin" />
          <span className="text-sm text-[var(--color-text-muted)]">加载中...</span>
        </div>
      </div>
    )
  }

  const handleSortClick = (column: TableColumn<T>) => {
    if (!column.sortable || !onSort) return;

    let newOrder: SortOrder;
    if (sortColumn !== column.key) {
      newOrder = 'asc';
    } else {
      newOrder = sortOrder === 'asc' ? 'desc' : sortOrder === 'desc' ? undefined : 'asc';
    }

    onSort({ column: column.key, order: newOrder });
  };

  // 渲染排序图标（两态同尺寸，避免点击排序时表头横向抖动）
  const renderSortIcon = (column: TableColumn<T>) => {
    if (!column.sortable) return null;

    if (sortColumn === column.key) {
      if (sortOrder === 'asc') {
        return <ArrowUp className="w-3.5 h-3.5 ml-1 text-[var(--color-accent)]" />;
      }
      if (sortOrder === 'desc') {
        return <ArrowDown className="w-3.5 h-3.5 ml-1 text-[var(--color-accent)]" />;
      }
    }

    return (
      <span className="text-[var(--color-text-muted)] ml-1 opacity-40 group-hover:opacity-70">
        <ArrowUp className="w-3.5 h-3.5" />
      </span>
    );
  };

  return (
    <div
      className={clsx('overflow-auto', className)}
      style={{ maxHeight }}
    >
      <table className="min-w-full">
        <thead className={clsx(stickyHeader && 'sticky top-0 z-10')}>
          <tr>
            {columns.map((col) => (
              <th
                key={col.key}
                className={clsx(
                  'px-4 py-3 text-[11px] font-semibold text-[var(--text-muted)] uppercase tracking-[0.06em] bg-[var(--bg-base)] border-b border-[var(--border-subtle)]',
                  col.align === 'center' && 'text-center',
                  col.align === 'right' && 'text-right',
                  !col.align && 'text-left',
                  col.sortable && 'cursor-pointer group hover:text-[var(--text-primary)]'
                )}
                style={{ width: col.width }}
                onClick={() => col.sortable && handleSortClick(col)}
              >
                <span className="flex items-center">
                  {col.title}
                  {renderSortIcon(col)}
                </span>
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-[var(--border-subtle)] bg-[var(--color-bg-card)]">
          {data.length === 0 ? (
            <tr>
              <td colSpan={columns.length} className="px-4 py-16 text-center">
                <div className="flex flex-col items-center gap-2">
                  <div className="w-12 h-12 rounded-full bg-[var(--color-bg-muted)] flex items-center justify-center">
                    <Inbox className="w-6 h-6 text-[var(--color-text-muted)]" strokeWidth={1.75} />
                  </div>
                  <span className="text-sm text-[var(--color-text-muted)]">{emptyText}</span>
                </div>
              </td>
            </tr>
          ) : (
            data.map((record, index) => (
              <tr
                key={getRowKey(record, index)}
                className={clsx(
                  'h-11 hover:bg-[var(--bg-hover)] transition-colors duration-150',
                  onRowClick && 'cursor-pointer'
                )}
                onClick={() => onRowClick?.(record)}
              >
                {columns.map((col) => (
                  <td
                    key={col.key}
                    className={clsx(
                      'px-4 py-3 text-[13px] text-[var(--text-secondary)] tabular-nums',
                      col.align === 'center' && 'text-center',
                      col.align === 'right' && 'text-right'
                    )}
                  >
                    {col.render
                      ? col.render(record[col.key], record, index)
                      : record[col.key]}
                  </td>
                ))}
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  )
}
