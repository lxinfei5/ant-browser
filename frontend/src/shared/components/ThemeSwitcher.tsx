import { Check } from 'lucide-react'
import clsx from 'clsx'
import { useTheme, themeConfigs, ThemeType } from '../theme'

interface ThemeSwitcherProps {
  className?: string
}

// 迷你预览色板 — 与 themes/base.css / light.css 中的 token 保持一致
const themePreview: Record<ThemeType, { bg: string; sidebar: string; accent: string; text: string }> = {
  dark: { bg: '#0B0E12', sidebar: '#12161C', accent: '#2DD4BF', text: '#E8ECF1' },
  light: { bg: '#F5F6F8', sidebar: '#FFFFFF', accent: '#0D9488', text: '#161B22' },
}

export function ThemeSwitcher({ className }: ThemeSwitcherProps) {
  const { theme, setTheme } = useTheme()

  return (
    <div className={clsx('space-y-4', className)}>
      <div className="grid grid-cols-2 gap-3 max-w-md">
        {themeConfigs.map((config) => {
          const isActive = theme === config.id
          const preview = themePreview[config.id]

          return (
            <button
              key={config.id}
              onClick={() => setTheme(config.id)}
              className={clsx(
                'group relative flex flex-col items-center gap-2.5 p-3 rounded-[var(--radius-lg)] border transition-all duration-150',
                isActive
                  ? 'border-[var(--accent)] bg-[var(--accent-soft)]'
                  : 'border-[var(--border-subtle)] hover:border-[var(--border-strong)] bg-[var(--color-bg-card)]'
              )}
              title={config.description}
            >
              {/* 主题预览 - 模拟界面布局 */}
              <div
                className="w-full aspect-[4/3] rounded-[var(--radius-md)] overflow-hidden border border-[var(--border-subtle)]"
                style={{ backgroundColor: preview.bg }}
              >
                {/* 侧边栏 */}
                <div
                  className="w-1/4 h-full float-left"
                  style={{ backgroundColor: preview.sidebar }}
                >
                  <div
                    className="w-2/3 h-1 mt-2 mx-auto rounded-full"
                    style={{ backgroundColor: preview.accent }}
                  />
                  <div className="mt-2 mx-1 space-y-1">
                    <div className="h-0.5 rounded-full" style={{ backgroundColor: preview.text, opacity: 0.15 }} />
                    <div className="h-0.5 rounded-full" style={{ backgroundColor: preview.text, opacity: 0.15 }} />
                  </div>
                </div>
                {/* 内容区 */}
                <div className="p-1">
                  <div className="h-1 w-1/2 rounded-full mb-1" style={{ backgroundColor: preview.text, opacity: 0.15 }} />
                  <div className="grid grid-cols-2 gap-0.5">
                    <div className="h-2 rounded-sm" style={{ backgroundColor: preview.sidebar }} />
                    <div className="h-2 rounded-sm" style={{ backgroundColor: preview.sidebar }} />
                  </div>
                </div>
              </div>

              {/* 主题名称 */}
              <span className={clsx(
                'text-xs font-medium transition-colors',
                isActive ? 'text-[var(--accent)]' : 'text-[var(--text-secondary)]'
              )}>
                {config.name}
              </span>

              {/* 选中标记 */}
              {isActive && (
                <div className="absolute -top-1.5 -right-1.5 w-5 h-5 rounded-full bg-[var(--accent)] flex items-center justify-center shadow-sm">
                  <Check className="w-3 h-3 text-[var(--accent-contrast)]" />
                </div>
              )}
            </button>
          )
        })}
      </div>

      {/* 当前主题描述 */}
      <p className="text-xs text-[var(--text-muted)]">
        {themeConfigs.find(c => c.id === theme)?.description}
      </p>
    </div>
  )
}
