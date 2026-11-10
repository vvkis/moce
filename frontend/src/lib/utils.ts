import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function isDesktop() {
  return typeof window !== "undefined" && !!window.go?.main?.App?.CallTool
}

export function errorMessage(err: unknown) {
  if (err instanceof Error && err.message) return err.message
  if (typeof err === "string" && err) return err
  return "出了点问题"
}

export async function fileToBase64(file: File) {
  const buf = await file.arrayBuffer()
  return btoa(binaryChunks(new Uint8Array(buf)))
}

function binaryChunks(bytes: Uint8Array) {
  const parts: string[] = []
  const size = 0x2000
  for (let i = 0; i < bytes.length; i += size) {
    parts.push(String.fromCharCode(...bytes.subarray(i, i + size)))
  }
  return parts.join("")
}

export function utf8ToBase64(text: string) {
  return btoa(binaryChunks(new TextEncoder().encode(text)))
}

export function downloadBase64(filename: string, b64: string) {
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  const blob = new Blob([bytes], { type: "application/epub+zip" })
  const a = document.createElement("a")
  a.href = URL.createObjectURL(blob)
  a.download = filename
  a.click()
  URL.revokeObjectURL(a.href)
}

export function baseName(path: string) {
  const parts = path.split(/[/\\]/)
  return parts[parts.length - 1] || path
}

export function formatBytes(n: number) {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${Math.max(1, Math.round(n / 1024))} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}
