import { tagEquals } from '../utils/tagMatch'

interface TagFilterBarProps {
  tags: string[]
  selected: Set<string>
  onChange: (next: Set<string>) => void
}

export function TagFilterBar({ tags, selected, onChange }: TagFilterBarProps) {
  if (tags.length === 0) return null

  // 大小写不敏感判断 selected 是否含某标签（兼容历史持久化的不同 casing）
  const isSelected = (tag: string) => Array.from(selected).some(s => tagEquals(s, tag))

  const toggle = (tag: string) => {
    const next = new Set<string>()
    let removed = false
    for (const s of selected) {
      if (tagEquals(s, tag)) { removed = true; continue }
      next.add(s)
    }
    if (!removed) next.add(tag)
    onChange(next)
  }

  const isAllSelected = selected.size === 0

  return (
    <div className="flex items-center gap-2 flex-wrap">
      <span className="text-xs text-[var(--color-text-muted)] shrink-0">标签：</span>
      <button
        onClick={() => onChange(new Set())}
        className={`px-2.5 py-0.5 rounded-full text-xs font-medium transition-colors cursor-pointer ${
          isAllSelected
            ? 'bg-[var(--accent)] text-[var(--accent-contrast)]'
            : 'bg-[var(--color-bg-muted)] text-[var(--color-text-muted)] hover:bg-[var(--color-bg-subtle)]'
        }`}
      >
        全部
      </button>
      {tags.map(tag => (
        <button
          key={tag}
          onClick={() => toggle(tag)}
          className={`px-2.5 py-0.5 rounded-full text-xs font-medium transition-colors cursor-pointer ${
            isSelected(tag)
              ? 'bg-[var(--accent)] text-[var(--accent-contrast)]'
              : 'bg-[var(--color-bg-muted)] text-[var(--color-text-muted)] hover:bg-[var(--color-bg-subtle)]'
          }`}
        >
          {tag}
        </button>
      ))}
    </div>
  )
}
