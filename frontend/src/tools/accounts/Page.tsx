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
    <div className="mx-auto max-w-3xl space-y-4 p-3 md:p-6">
      <header>
        <p className="text-[11px] tracking-[0.22em] text-cinnabar">账号</p>
        <h1 className="font-serif text-3xl tracking-wide">刮搜登录</h1>
        <p className="mt-1 text-sm text-muted">登录信息写在这个文件夹的 data\accounts，不上传。默认明文，整夹搬走后仍能用。</p>
      </header>
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={mode === "dpapi"} onChange={(e) => void saveMode(e.target.checked ? "dpapi" : "plain")} />
        仅本机 Windows 用户可解密（换电脑就读不出，便携请保持关闭）
      </label>
      {error ? <p className="text-sm text-cinnabar">{error}</p> : null}
      <ul className="space-y-3">
        {rows.map((row) => (
          <li key={row.id} className="rounded-xl border border-line bg-paper-card p-4 shadow-card">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h2 className="font-serif text-lg">{row.name}</h2>
                <p className="text-xs text-muted">{row.status}</p>
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
              <div className="mt-3 space-y-2">
                {row.id === "weread" ? (
                  <Button type="button" size="sm" onClick={() => void startQR()}>
                    扫码登录
                  </Button>
                ) : null}
                {qr && editing.id === "weread" ? <img src={`data:image/png;base64,${qr}`} alt="登录二维码" className="h-48 w-48 border border-line bg-white" /> : null}
                {row.id !== "weread" ? (
                  <>
                    <p className="text-xs leading-5 text-muted">{row.hint}</p>
                    <textarea aria-label="登录信息" className="min-h-20 w-full rounded-md border border-line bg-[#fffaf3] px-3 py-2 text-sm" value={secret} onChange={(e) => setSecret(e.target.value)} />
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
