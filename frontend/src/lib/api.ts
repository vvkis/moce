import type { ToolMeta } from "./types"
import { isDesktop } from "./utils"

export async function listTools(): Promise<ToolMeta[]> {
  if (isDesktop()) {
    return window.go!.main.App.ListTools()
  }
  const res = await fetch("/api/tools")
  if (!res.ok) throw new Error("无法获取工具列表")
  return res.json()
}

export async function callTool<T>(id: string, action: string, payload: unknown): Promise<T> {
  if (isDesktop()) {
    const text = await window.go!.main.App.CallTool(id, action, JSON.stringify(payload ?? {}))
    return JSON.parse(text) as T
  }
  const res = await fetch("/api/call", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ id, action, payload: payload ?? {} }),
  })
  const body = (await res.json()) as { result?: T; error?: string }
  if (!res.ok) throw new Error(body.error || "调用失败")
  return body.result as T
}
