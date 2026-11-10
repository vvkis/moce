/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: { DEFAULT: "#241c17", soft: "#3a3028" },
        paper: { DEFAULT: "#f4eadc", deep: "#e6d8c4", card: "#fbf6ee" },
        cinnabar: { DEFAULT: "#9d2c2c", dark: "#7c2222" },
        line: "#e0d2c0",
        muted: "#7a6b5d",
      },
      fontFamily: {
        sans: [
          "Segoe UI",
          "PingFang SC",
          "Microsoft YaHei",
          "Noto Sans CJK SC",
          "Source Han Sans SC",
          "WenQuanYi Micro Hei",
          "Droid Sans Fallback",
          "sans-serif",
        ],
        serif: [
          "Moce Serif",
          "Iowan Old Style",
          "Palatino Linotype",
          "Songti SC",
          "Noto Serif CJK SC",
          "Source Han Serif SC",
          "SimSun",
          "WenQuanYi Micro Hei",
          "Droid Sans Fallback",
          "serif",
        ],
      },
      boxShadow: {
        card: "0 1px 0 rgba(36,28,23,0.04), 0 12px 30px rgba(36,28,23,0.05)",
      },
    },
  },
  plugins: [],
}
