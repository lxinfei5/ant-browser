import { ReactNode } from 'react'
import clsx from 'clsx'

interface CardProps {
  title?: ReactNode
  subtitle?: string
  children: ReactNode
  className?: string
  padding?: 'none' | 'sm' | 'md' | 'lg'
  actions?: ReactNode
  hover?: boolean
}

export function Card({ 
  title, 
  subtitle, 
  children, 
  className,
  padding = 'md',
  actions,
  hover = false
}: CardProps) {
  const paddings = {
    none: '',
    sm: 'p-4',
    md: 'p-5',
    lg: 'p-6',
  }

  return (
    <div 
      className={clsx(
        'bg-[var(--color-bg-card)] rounded-[var(--radius-lg)] overflow-hidden',
        'border border-[var(--border-subtle)]',
        'transition-all duration-200',
        hover && 'hover:shadow-[var(--shadow-md)] hover:-translate-y-px hover:border-[var(--border-strong)]',
        className
      )}
    >
      {(title || actions) && (
        <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--border-subtle)]">
          <div>
            {title && (
              <h3 className="text-[15px] font-semibold text-[var(--text-primary)]">
                {title}
              </h3>
            )}
            {subtitle && (
              <p className="text-xs text-[var(--text-muted)] mt-0.5">
                {subtitle}
              </p>
            )}
          </div>
          {actions && <div className="flex items-center gap-2">{actions}</div>}
        </div>
      )}
      <div className={paddings[padding]}>{children}</div>
    </div>
  )
}
