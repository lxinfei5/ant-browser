import clsx from 'clsx'

/**
 * 工具条竖向分隔线 — 全 App 统一规格，替代各处手写的
 * `w-px h-4/h-5 bg-[var(--border-*)] mx-1/mx-1.5`。
 * 放在 flex 工具条里靠 self-center 垂直居中。
 */
export function ToolbarDivider({ className }: { className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={clsx('w-px h-4 self-center bg-[var(--border-strong)] mx-1.5', className)}
    />
  )
}
