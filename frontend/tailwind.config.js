/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: { DEFAULT: "#111827", soft: "#27272a" },
        paper: { DEFAULT: "#f7f7f8", deep: "#eceef1", card: "#ffffff" },
        cinnabar: { DEFAULT: "#18181b", dark: "#09090b" },
        accent: { DEFAULT: "#2563eb", soft: "#eff6ff" },
        warn: { DEFAULT: "#b45309", soft: "#fffbeb" },
        danger: { DEFAULT: "#dc2626", soft: "#fef2f2" },
        line: "#e5e7eb",
        muted: "#6b7280",
      },
      fontFamily: {
        sans: [
          "Inter",
          "-apple-system",
          "BlinkMacSystemFont",
          "Segoe UI",
          "PingFang SC",
          "Microsoft YaHei",
          "Noto Sans CJK SC",
          "Source Han Sans SC",
          "sans-serif",
        ],
        serif: [
          "Inter",
          "-apple-system",
          "BlinkMacSystemFont",
          "Segoe UI",
          "PingFang SC",
          "Microsoft YaHei",
          "Noto Sans CJK SC",
          "sans-serif",
        ],
        mono: [
          "JetBrains Mono",
          "Cascadia Code",
          "Consolas",
          "ui-monospace",
          "SFMono-Regular",
          "monospace",
        ],
      },
      boxShadow: {
        card: "0 1px 2px rgba(15,23,42,0.04), 0 1px 1px rgba(15,23,42,0.02)",
      },
    },
  },
  plugins: [],
}
