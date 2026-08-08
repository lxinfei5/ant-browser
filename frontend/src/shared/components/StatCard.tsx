import { ReactNode } from 'react'
import clsx from 'clsx'

interface StatCardProps {
  title: string
  value: string | number
  icon?: ReactNode
  trend?: {
    value: number
    label: string
  }
}

export function StatCard({ title, value, icon, trend }: StatCardProps) {
  return (
    <div 
      className={clsx(
        'min-w-0 bg-[var(--color-bg-card)] rounded-[var(--radius-lg)] overflow-hidden',
        'border border-[var(--border-subtle)]',
        'transition-all duration-200',
        'hover:border-[var(--border-strong)]',
        'group'
      )}
    >
      <div className="p-5">
        <div className="flex items-start justify-between gap-4">
          <div className="flex-1 min-w-0">
            <p className="text-[11px] text-[var(--text-muted)] font-semibold tracking-[0.06em] uppercase">
              {title}
            </p>
            <p className="text-2xl font-semibold font-numeric text-[var(--text-primary)] mt-2 tabular-nums">
              {value}
            </p>
            {trend && (
              <div className="flex items-center gap-1.5 mt-2">
                <span className={clsx(
                  'inline-flex items-center rounded-[var(--radius-sm)] px-1.5 py-0.5 text-[11px] font-semibold',
                  trend.value >= 0
                    ? 'bg-[var(--success-soft)] text-[var(--success)]'
                    : 'bg-[var(--danger-soft)] text-[var(--danger)]'
                )}>
                  {trend.value >= 0 ? '↑' : '↓'} {Math.abs(trend.value)}%
                </span>
                <span className="text-xs text-[var(--text-muted)]">
                  {trend.label}
                </span>
              </div>
            )}
          </div>
          {icon && (
            <div className="w-11 h-11 rounded-[var(--radius-lg)] bg-[var(--accent-soft)] flex items-center justify-center text-[var(--accent)] transition-colors">
              {icon}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
