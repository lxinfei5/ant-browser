// 图表配色 — recharts 的常量色值在模块初始化时从 CSS token 读取一次，
// 保持与主题同步；读取失败时回退到 dark 主题的等价 hex。
// 注意：模块加载后再切换主题，图表色不会动态刷新（示例图表可接受）。

const FALLBACKS = {
  accent: '#2DD4BF',
  info: '#60A5FA',
  warning: '#FBBF24',
  success: '#34D399',
  violet: '#A78BFA',
} as const

function readToken(name: string, fallback: string): string {
  try {
    const value = getComputedStyle(document.documentElement)
      .getPropertyValue(name)
      .trim()
    return value || fallback
  } catch {
    return fallback
  }
}

export const CHART_COLORS: string[] = [
  readToken('--accent', FALLBACKS.accent),
  readToken('--info', FALLBACKS.info),
  readToken('--warning', FALLBACKS.warning),
  readToken('--success', FALLBACKS.success),
  FALLBACKS.violet,
]

export const CHART_COLOR = {
  accent: CHART_COLORS[0],
  info: CHART_COLORS[1],
  warning: CHART_COLORS[2],
  success: CHART_COLORS[3],
  violet: CHART_COLORS[4],
}
