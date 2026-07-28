// ProfilePool 品牌标志 — 内联 SVG，避免 PNG 加载失败回退逻辑。
// 三张叠放的身份卡片，前面一张带人像镂空，呼应 "profile pool"。
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
        <linearGradient id="pp-grad" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#0F766E" />
          <stop offset="0.55" stopColor="#14B8A6" />
          <stop offset="1" stopColor="#5EEAD4" />
        </linearGradient>
        <linearGradient id="pp-grad-back" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#0F766E" stopOpacity="0.55" />
          <stop offset="1" stopColor="#2DD4BF" stopOpacity="0.55" />
        </linearGradient>
      </defs>
      {/* back card (third profile) */}
      <rect x="30" y="6" width="26" height="26" rx="8" fill="url(#pp-grad-back)" />
      {/* middle card (second profile) */}
      <rect x="22" y="14" width="26" height="26" rx="8" fill="url(#pp-grad)" opacity="0.75" />
      {/* front card with identity cutout */}
      <path
        fill="url(#pp-grad)"
        fillRule="evenodd"
        d="M14 24 h26 a8 8 0 0 1 8 8 v18 a8 8 0 0 1 -8 8 h-18 a8 8 0 0 1 -8 -8 v-18 a8 8 0 0 1 8 -8 z
           M31 36 a5 5 0 1 0 0.001 0 z
           M31 43 c-5 0 -8.5 2.6 -8.5 6.2 c0 1.2 1 2.3 2.3 2.3 h12.4 c1.3 0 2.3 -1.1 2.3 -2.3 c0 -3.6 -3.5 -6.2 -8.5 -6.2 z"
      />
    </svg>
  )
}
