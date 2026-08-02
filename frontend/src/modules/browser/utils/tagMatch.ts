// 标签匹配/归并工具
// 统一标签的大小写与去重口径，避免「OpenCode」与「opencode」在不同页面被当作两个标签、
// 导致标签册点击其一却筛不出实例。所有标签比较都应走这里，不直接用 === / Array.includes。

/** 规范化标签：trim + 转小写，仅用于比较与去重 key，不用于展示。 */
export function normalizeTag(tag: string | null | undefined): string {
  return (tag || '').trim().toLowerCase()
}

/** 两个标签是否等价（大小写、首尾空白不敏感）。 */
export function tagEquals(a: string | null | undefined, b: string | null | undefined): boolean {
  return normalizeTag(a) === normalizeTag(b)
}

/** tags 列表中是否包含 query（大小写不敏感）。 */
export function tagsContain(tags: string[] | null | undefined, query: string | null | undefined): boolean {
  const q = normalizeTag(query)
  if (!q) return false
  return (tags || []).some(t => normalizeTag(t) === q)
}

/** 从多个标签来源合并去重，保留每个规范值首次出现的原 casing 作为展示值。 */
export function mergeTags(...lists: Array<string[] | null | undefined>): string[] {
  const seen = new Map<string, string>()
  for (const list of lists) {
    for (const raw of list || []) {
      const display = (raw || '').trim()
      if (!display) continue
      const key = display.toLowerCase()
      if (!seen.has(key)) {
        seen.set(key, display)
      }
    }
  }
  return Array.from(seen.values()).sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()))
}
