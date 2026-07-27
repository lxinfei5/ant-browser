import { useEffect, useRef, useState } from 'react'
import { Select } from '../../../shared/components'
import { renderDockIconDataURL, DOCK_ICON_COLORS, colorForName, type DockIconKind } from '../utils/dockIcon'

interface AccountIconPickerProps {
  kind: string
  color: string
  text: string
  image: string
  fallbackText: string
  onChange: (patch: Partial<{ iconKind: string; iconColor: string; iconText: string; iconImage: string }>) => void
}

const KIND_OPTIONS = [
  { value: '', label: '不定制' },
  { value: 'text', label: '底色 + 首字母' },
  { value: 'color', label: '纯底色' },
  { value: 'image', label: '上传图片' },
]

// AccountIconPicker 账号 Dock 图标选择器：类型 + 色板 + 首字母 + 图片上传 + 实时预览。
export function AccountIconPicker({ kind, color, text, image, fallbackText, onChange }: AccountIconPickerProps) {
  const fileRef = useRef<HTMLInputElement>(null)
  const [preview, setPreview] = useState('')

  const effectiveColor = color || colorForName(text || fallbackText)
  const effectiveText = text || fallbackText

  useEffect(() => {
    let cancelled = false
    if (!kind) {
      setPreview('')
      return
    }
    void renderDockIconDataURL(kind as DockIconKind, effectiveColor, effectiveText, image).then((url) => {
      if (!cancelled) setPreview(url)
    })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [kind, effectiveColor, effectiveText, image])

  const handleFile = (file: File | undefined) => {
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      onChange({ iconImage: String(reader.result || ''), iconKind: 'image' })
    }
    reader.readAsDataURL(file)
  }

  return (
    <div className="flex items-start gap-4">
      {/* 预览 */}
      <div className="shrink-0">
        {preview ? (
          <img src={preview} alt="Dock 图标预览" className="w-14 h-14 rounded-2xl border border-[var(--color-border)]" />
        ) : (
          <div className="w-14 h-14 rounded-2xl border border-dashed border-[var(--color-border)] flex items-center justify-center text-xs text-[var(--color-text-tertiary)]">
            无
          </div>
        )}
      </div>

      <div className="flex-1 space-y-3">
        <Select
          value={kind}
          onChange={(e) => onChange({ iconKind: e.target.value })}
          options={KIND_OPTIONS}
        />

        {(kind === 'text' || kind === 'color') && (
          <div className="flex flex-wrap gap-2">
            {DOCK_ICON_COLORS.map((c) => (
              <button
                key={c}
                type="button"
                aria-label={`颜色 ${c}`}
                onClick={() => onChange({ iconColor: c })}
                className={`w-6 h-6 rounded-md border-2 transition ${
                  effectiveColor === c ? 'border-[var(--color-accent)] scale-110' : 'border-transparent'
                }`}
                style={{ backgroundColor: c }}
              />
            ))}
          </div>
        )}

        {kind === 'text' && (
          <input
            type="text"
            value={text}
            maxLength={4}
            onChange={(e) => onChange({ iconText: e.target.value })}
            placeholder={`首字母/短名（默认取 ${fallbackText || '账号名'}）`}
            className="w-full px-2 py-1.5 text-sm rounded-md border border-[var(--color-border)] bg-[var(--color-bg-surface)]"
          />
        )}

        {kind === 'image' && (
          <div>
            <input
              ref={fileRef}
              type="file"
              accept="image/*"
              className="text-sm"
              onChange={(e) => handleFile(e.target.files?.[0])}
            />
          </div>
        )}
      </div>
    </div>
  )
}
