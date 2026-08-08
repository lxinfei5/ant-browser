/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      fontFamily: {
        sans: ['var(--font-sans)'],
        mono: ['var(--font-mono)'],
        // Semantic numeric chain: stats, table value columns, timestamps,
        // latency, chart ticks. Same single source as mono (base.css).
        numeric: ['var(--font-numeric)'],
      },
      // Semantic type scale -> tokens. Prefer these over ad-hoc text-[Npx].
      fontSize: {
        'display': 'var(--text-display)',
        'title': 'var(--text-title)',
        'body': 'var(--text-body)',
        'caption': 'var(--text-caption)',
        'micro': 'var(--text-micro)',
      },
      borderRadius: {
        'sm': 'var(--radius-sm)',
        'DEFAULT': 'var(--radius-md)',
        'md': 'var(--radius-md)',
        'lg': 'var(--radius-lg)',
        'xl': 'var(--radius-xl)',
      },
      boxShadow: {
        'xs': 'var(--shadow-xs)',
        'sm': 'var(--shadow-sm)',
        'DEFAULT': 'var(--shadow-sm)',
        'md': 'var(--shadow-md)',
        'lg': 'var(--shadow-lg)',
        // Token scale tops out at --shadow-lg; alias the heavier Tailwind
        // names onto it so raw shadow-xl/2xl stay on the token scale.
        'xl': 'var(--shadow-lg)',
        '2xl': 'var(--shadow-lg)',
      },
    },
  },
  plugins: [],
}
