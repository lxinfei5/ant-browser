// dockIcon 渲染账号 Dock 图标为 PNG dataURL。
// 在前端用 canvas 绘制「圆角底色 + 首字母/短名」（或直接使用上传图），
// 后端只做 PNG→icns 转换，不渲染文字，避免在后端嵌入字体。

export type DockIconKind = '' | 'color' | 'text' | 'image'

const SIZE = 1024
const RADIUS_RATIO = 0.225 // 接近 macOS 图标的圆角比例

// 候选底色板（辨识度优先）。
export const DOCK_ICON_COLORS = [
  '#3b82f6', // 蓝
  '#8b5cf6', // 紫
  '#ec4899', // 粉
  '#ef4444', // 红
  '#f97316', // 橙
  '#eab308', // 黄
  '#22c55e', // 绿
  '#14b8a6', // 青
  '#06b6d4', // 青蓝
  '#64748b', // 灰
]

function roundRectPath(ctx: CanvasRenderingContext2D, size: number, radius: number): void {
  ctx.beginPath()
  if (typeof ctx.roundRect === 'function') {
    ctx.roundRect(0, 0, size, size, radius)
    return
  }
  // 兼容无 roundRect 的环境
  const r = radius
  ctx.moveTo(r, 0)
  ctx.arcTo(size, 0, size, size, r)
  ctx.arcTo(size, size, 0, size, r)
  ctx.arcTo(0, size, 0, 0, r)
  ctx.arcTo(0, 0, size, 0, r)
  ctx.closePath()
}

// 取首字母/短名：优先取前两个字符（CJK 取首字符），大写。
export function iconInitials(text: string): string {
  const t = (text || '').trim()
  if (!t) return '?'
  const chars = Array.from(t)
  // 含 CJK 时取首字符即可；否则取前 1-2 个拉丁字符。
  if (/[一-鿿]/.test(chars[0])) {
    return chars[0]
  }
  return chars.slice(0, 2).join('').toUpperCase()
}

// 根据账号名推导一个稳定底色（无色板选择时兜底）。
export function colorForName(name: string): string {
  let h = 0
  for (const ch of name || '') {
    h = (h << 5) - h + ch.charCodeAt(0)
    h |= 0
  }
  const idx = Math.abs(h) % DOCK_ICON_COLORS.length
  return DOCK_ICON_COLORS[idx]
}

function loadImage(dataURL: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => resolve(img)
    img.onerror = () => reject(new Error('image load failed'))
    img.src = dataURL
  })
}

// renderDockIconDataURL 渲染 1024×1024 PNG dataURL。
// - color：纯底色圆角块（叠加首字母，若有 text）
// - text：底色 + 首字母/短名
// - image：直接使用 imageDataURL（居中裁剪为方形），失败回退 text 渲染
export async function renderDockIconDataURL(
  kind: DockIconKind,
  color: string,
  text: string,
  imageDataURL?: string,
): Promise<string> {
  const canvas = document.createElement('canvas')
  canvas.width = SIZE
  canvas.height = SIZE
  const ctx = canvas.getContext('2d')
  if (!ctx) return ''

  const bg = color || colorForName(text)
  const radius = SIZE * RADIUS_RATIO

  if (kind === 'image' && imageDataURL) {
    try {
      const img = await loadImage(imageDataURL)
      // 方形居中裁剪
      const side = Math.min(img.width, img.height)
      const sx = (img.width - side) / 2
      const sy = (img.height - side) / 2
      roundRectPath(ctx, SIZE, radius)
      ctx.save()
      ctx.clip()
      ctx.drawImage(img, sx, sy, side, side, 0, 0, SIZE, SIZE)
      ctx.restore()
      return canvas.toDataURL('image/png')
    } catch {
      // 落到文字渲染
    }
  }

  // 底色圆角块
  roundRectPath(ctx, SIZE, radius)
  ctx.fillStyle = bg
  ctx.fill()

  // 首字母/短名
  const label = iconInitials(text)
  if (label) {
    const fontSize = label.length > 1 ? SIZE * 0.42 : SIZE * 0.52
    ctx.fillStyle = '#ffffff'
    ctx.font = `600 ${fontSize}px -apple-system, "Helvetica Neue", "PingFang SC", "Microsoft YaHei", sans-serif`
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillText(label, SIZE / 2, SIZE / 2 + fontSize * 0.04)
  }

  return canvas.toDataURL('image/png')
}
