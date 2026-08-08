// ProfilePool 品牌标志 — 内联 SVG，避免 PNG 加载失败回退逻辑。
// 设计：涟漪/池（ripple / pool）—— 同心圆环由外向内收敛到核心，
// 呼应 "profile pool"：众多身份汇入同一个池。右上角一枚亮色
// 卫星点，代表池中一个活跃身份。以本组件为单一图形真源，
// favicon.svg / logo.svg 复用同一图形。
export function Logo({ size = 24, className }: { size?: number; className?: string }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 64 64"
      width={size}
      height={size}
      className={className}
      aria-hidden="true"
    >
      <defs>
        <linearGradient id="pp-ripple" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#0F766E" />
          <stop offset="0.55" stopColor="#14B8A6" />
          <stop offset="1" stopColor="#2DD4BF" />
        </linearGradient>
      </defs>
      {/* outer ripple ring */}
      <circle cx="32" cy="32" r="29" fill="none" stroke="url(#pp-ripple)" strokeWidth="4" opacity="0.28" />
      {/* middle ripple ring */}
      <circle cx="32" cy="32" r="20" fill="none" stroke="url(#pp-ripple)" strokeWidth="4.5" opacity="0.6" />
      {/* core identity */}
      <circle cx="32" cy="32" r="11" fill="url(#pp-ripple)" />
      {/* active satellite profile */}
      <circle cx="44" cy="20" r="5" fill="#5EEAD4" />
    </svg>
  )
}
