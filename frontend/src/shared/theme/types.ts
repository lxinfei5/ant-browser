// 主题类型定义 — ProfilePool 为 light-default 产品
export type ThemeType = 'dark' | 'light'

export interface ThemeConfig {
  id: ThemeType
  name: string
  description: string
}

export const themeConfigs: ThemeConfig[] = [
  { id: 'light', name: '浅色', description: '简洁明亮的浅色风格，默认主题' },
  { id: 'dark', name: '深色', description: '石墨深色，护眼夜间风格' },
]

export const DEFAULT_THEME: ThemeType = 'light'

// 旧版 5 主题 localStorage 值迁移：cream/mint → light，ocean → dark
export function normalizeTheme(saved: string | null): ThemeType | null {
  if (!saved) return null
  if (saved === 'dark' || saved === 'light') return saved
  if (saved === 'cream' || saved === 'mint') return 'light'
  if (saved === 'ocean') return 'dark'
  return null
}
