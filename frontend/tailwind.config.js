/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        primary: {
          50: '#fdf8f3',
          100: '#f9ede0',
          200: '#f3dbc2',
          300: '#e9c49d',
          400: '#dda876',
          500: '#c9915c',
          600: '#b87d4a',
          700: '#9a6840',
          800: '#7d5538',
          900: '#664630',
        },
        accent: {
          yellow: '#f5d547',
          orange: '#f5a962',
        },
        background: {
          DEFAULT: '#e5e7eb',
          card: '#ffffff',
        },
      },
      fontFamily: {
        sans: ['"Monaco"', '"Microsoft YaHei"', '"PingFang SC"', 'sans-serif'],
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
        'card': 'var(--shadow-sm)',
        'card-hover': 'var(--shadow-md)',
      },
    },
  },
  plugins: [],
}
