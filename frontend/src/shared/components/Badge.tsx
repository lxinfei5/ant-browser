import { ReactNode } from 'react'
import clsx from 'clsx'

type BadgeVariant = 'default' | 'success' | 'error' | 'warning' | 'info'
type BadgeSize = 'sm' | 'md' | 'lg'

interface BadgeProps {
  children: ReactNode
  variant?: BadgeVariant
  size?: BadgeSize
  dot?: boolean
  dotClassName?: string
  className?: string
}

const variantStyles = {
  default: 'bg-[var(--bg-hover)] text-[var(--text-secondary)] border border-[var(--border-subtle)]',
  success: 'bg-[var(--success-soft)] text-[var(--success)]',
  error: 'bg-[var(--danger-soft)] text-[var(--danger)]',
  warning: 'bg-[var(--warning-soft)] text-[var(--warning)]',
  info: 'bg-[var(--info-soft)] text-[var(--info)]',
}

const sizeStyles = {
  sm: 'px-1.5 py-0.5 text-[11px]',
  md: 'px-2 py-0.5 text-[11px]',
  lg: 'px-2.5 py-1 text-xs',
}

const dotStyles = {
  default: 'bg-[var(--text-muted)]',
  success: 'bg-[var(--success)]',
  error: 'bg-[var(--danger)]',
  warning: 'bg-[var(--warning)]',
  info: 'bg-[var(--info)]',
}

export function Badge({
  children,
  variant = 'default',
  size = 'md',
  dot = false,
  dotClassName = 'w-1.5 h-1.5',
  className,
}: BadgeProps) {
  return (
    <span
      className={clsx(
        'inline-flex items-center gap-1.5 rounded-[var(--radius-sm)] font-semibold',
        variantStyles[variant],
        sizeStyles[size],
        className
      )}
    >
      {dot && (
        <span className={clsx('rounded-full', dotClassName, dotStyles[variant])} />
      )}
      {children}
    </span>
  )
}
