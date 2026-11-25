import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { callTool } from "@/lib/api"
import { errorMessage, isDesktop } from "@/lib/utils"

type Source = {
  id: string
  name: string
  status: string
  logged: boolean
  method: string
  hint: string
  qr: boolean
}

export function AccountsPage() {
  const [rows, setRows] = useState<Source[]>([])
  const [error, setError] = useState("")
  const [mode, setMode] = useState("plain")
  const [editing, setEditing] = useState<Source | null>(null)
  const [secret, setSecret] = useState("")
  const [busy, setBusy] = useState(false)
  const [qr, setQr] = useState("")
  const [session, setSession] = useState("")

  async function reload() {
    const data = await callTool<{ sources: Source[] }>("txt2epub", "accountList", {})
    setRows(data.sources || [])
  }

  useEffect(() => {
    void reload().catch((err) => setError(errorMessage(err)))
    if (isDesktop() && window.go?.main.App.LoadSettings) {
      void window.go.main.App.LoadSettings().then((s) => setMode(s.accountMode === "dpapi" ? "dpapi" : "plain"))
    }
  }, [])

  useEffect(() => {
    if (!session) return
    const timer = window.setInterval(() => {
      void callTool<{ done: boolean }>("txt2epub", "accountQRPoll", { session })
        .then((res) => {
          if (res.done) {
            setSession("")
            setQr("")
            setEditing(null)
            return reload()
          }
        })
        .catch((err) => {
          setError(errorMessage(err))
          setSession("")
        })
    }, 2000)
    return () => window.clearInterval(timer)
  }, [session])

  async function saveMode(next: string) {
    setMode(next)
    if (isDesktop() && window.go?.main.App.LoadSettings) {
      const cur = await window.go.main.App.LoadSettings()
      await window.go.main.App.SaveSettings({ ...cur, accountMode: next })
    }
  }

  async function commit() {
    if (!editing) return
    setBusy(true)
    setError("")
    try {
      await callTool("txt2epub", "accountCommit", { id: editing.id, secret })
      setSecret("")
      setEditing(null)
      await reload()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  async function startQR() {
    setError("")
    try {
      const res = await callTool<{ session: string; imageBase64: string }>("txt2epub", "accountQRStart", {})
      setSession(res.session)
      setQr(res.imageBase64)
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  return (
    <div className="mx-auto max-w-3xl space-y-4 p-4 md:p-6">
      <header className="border-b border-line pb-4">
        <h1 className="text-xl font-semibold tracking-tight text-ink">刮搜账号</h1>
        <p className="mt-1 text-xs text-muted">
          登录信息仅保存在本目录 <code className="rounded bg-paper-deep px-1 py-0.5 font-mono">data\accounts</code>，不会上传。默认明文便于整夹便携迁移。
        </p>
      </header>
      <label className="flex cursor-pointer items-center gap-2 rounded-lg border border-line bg-paper-card px-3 py-2.5 text-xs text-ink">
        <input type="checkbox" checked={mode === "dpapi"} onChange={(e) => void saveMode(e.target.checked ? "dpapi" : "plain")} />
        仅本机 Windows 用户可解密（DPAPI；换电脑后不可读取，便携请保持关闭）
      </label>
      {error ? <p className="rounded-lg border border-red-200 bg-danger-soft px-3 py-2 text-xs text-danger">{error}</p> : null}
      <ul className="space-y-2.5">
        {rows.map((row) => (
          <li key={row.id} className="rounded-xl border border-line bg-paper-card p-4 shadow-card">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h2 className="text-sm font-semibold text-ink">{row.name}</h2>
                <p className="mt-0.5 text-xs text-muted">{row.status}</p>
              </div>
              <div className="flex gap-2">
                <Button type="button" size="sm" variant="outline" onClick={() => { setEditing(row); setSecret(""); setQr(""); setError("") }}>
                  登录
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={!row.logged}
                  onClick={() => void callTool("txt2epub", "accountDelete", { id: row.id }).then(() => reload())}
                >
                  退出
                </Button>
              </div>
            </div>
            {editing?.id === row.id ? (
              <div className="mt-3 space-y-2 border-t border-line pt-3">
                {row.id === "weread" ? (
                  <Button type="button" size="sm" onClick={() => void startQR()}>
                    扫码登录
                  </Button>
                ) : null}
                {qr && editing.id === "weread" ? <img src={`data:image/png;base64,${qr}`} alt="登录二维码" className="h-44 w-44 rounded-lg border border-line bg-white p-1" /> : null}
                {row.id !== "weread" ? (
                  <>
                    <p className="text-xs leading-5 text-muted">{row.hint}</p>
                    <textarea
                      aria-label="登录信息"
                      className="min-h-20 w-full rounded-lg border border-line bg-white px-3 py-2 font-mono text-xs text-ink focus-visible:border-ink/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ink/10"
                      value={secret}
                      onChange={(e) => setSecret(e.target.value)}
                    />
                    <Button type="button" size="sm" disabled={busy} onClick={() => void commit()}>
                      {busy ? "正在校验登录…" : "登录"}
                    </Button>
                  </>
                ) : null}
              </div>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  )
}
