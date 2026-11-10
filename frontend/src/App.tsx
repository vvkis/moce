import { useEffect, useState } from "react"
import { GenericPanel } from "@/components/GenericPanel"
import { Sidebar } from "@/components/Sidebar"
import { Button } from "@/components/ui/button"
import { callTool, listTools } from "@/lib/api"
import type { ToolMeta } from "@/lib/types"
import { errorMessage, isDesktop } from "@/lib/utils"
import { ShellContext } from "@/shell"
import { panels } from "@/tools/registry"
import { AccountsPage } from "@/tools/accounts/Page"
import { ProgressPage, type ProgressJob } from "@/tools/progress/Page"

type DoneItem = { title: string; path?: string; error: string }

export default function App() {
  const [tools, setTools] = useState<ToolMeta[]>([])
  const [active, setActive] = useState("")
  const [page, setPage] = useState<"tool" | "progress" | "accounts">("tool")
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(true)
  const [batch, setBatch] = useState("")
  const [report, setReport] = useState<DoneItem[] | null>(null)

  function load() {
    setLoading(true)
    setError("")
    listTools()
      .then((list) => {
        setTools(list)
        setActive((cur) => cur || list[0]?.id || "")
      })
      .catch((err) => setError(errorMessage(err)))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load()
  }, [])

  useEffect(() => {
    if (!batch) return
    let stop = false
    const timer = window.setInterval(() => {
      void callTool<{ jobs: ProgressJob[] }>("txt2epub", "listJobs", {})
        .then((data) => {
          if (stop) return
          const mine = (data.jobs || []).filter((job) => job.batch === batch)
          const pending = mine.some((job) => job.status === "queued" || job.status === "extracting" || job.status === "detecting" || job.status === "packaging")
          if (mine.length > 0 && !pending) {
            setReport(
              mine.map((job) => ({
                title: job.title || job.name,
                path: job.outputPath,
                error: job.status === "done" ? "" : job.error || job.step,
              })),
            )
            setBatch("")
          }
        })
        .catch(() => {})
    }, 800)
    return () => {
      stop = true
      window.clearInterval(timer)
    }
  }, [batch])

  const meta = tools.find((tool) => tool.id === active)
  const Panel = meta?.panel ? panels[meta.panel] : undefined

  return (
    <ShellContext.Provider
      value={{
        openProgress: () => setPage("progress"),
        watchBatch: (id) => setBatch(id),
      }}
    >
      <div className="paper-app flex h-full min-h-0 flex-col md:flex-row">
        <Sidebar
          tools={tools}
          active={page === "tool" ? active : ""}
          progress={page === "progress"}
          accounts={page === "accounts"}
          onSelect={(id) => {
            setPage("tool")
            setActive(id)
          }}
          onProgress={() => setPage("progress")}
          onAccounts={() => setPage("accounts")}
        />
        <main className="min-h-0 min-w-0 flex-1 overflow-x-hidden overflow-y-auto">
          {loading ? <p className="p-8 text-sm text-muted">正在打开书架…</p> : null}
          {!loading && error ? (
            <div className="p-8">
              <p className="text-sm text-cinnabar">{error}</p>
              <Button className="mt-4" variant="outline" onClick={load}>
                重试
              </Button>
            </div>
          ) : null}
          {!loading && page === "progress" ? <ProgressPage /> : null}
          {!loading && page === "accounts" ? <AccountsPage /> : null}
          {!loading && page === "tool" && meta && Panel ? <Panel meta={meta} /> : null}
          {!loading && page === "tool" && meta && !Panel ? <GenericPanel meta={meta} /> : null}
        </main>
        {report ? <DonePanel items={report} onClose={() => setReport(null)} onProgress={() => setPage("progress")} /> : null}
      </div>
    </ShellContext.Provider>
  )
}

function DonePanel({ items, onClose, onProgress }: { items: DoneItem[]; onClose: () => void; onProgress: () => void }) {
  const ok = items.filter((item) => !item.error)
  const bad = items.filter((item) => item.error)
  const headline = items.length === 1 && bad.length === 0 ? `《${items[0].title}》已生成` : `${ok.length} 本已完成，${bad.length} 本失败`
  return (
    <div className="fixed inset-x-0 bottom-0 z-40 flex justify-center p-3 sm:p-6">
      <div className="w-full max-w-lg rounded-xl border border-cinnabar/30 bg-paper-card p-4 shadow-card">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 className="font-serif text-2xl">排版完成</h2>
            <p className="mt-1 text-sm">{headline}</p>
          </div>
          <Button type="button" size="sm" variant="outline" onClick={onClose}>
            关闭
          </Button>
        </div>
        <div className="mt-3 flex flex-wrap gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              onProgress()
            }}
          >
            查看进度
          </Button>
          {ok.length === 1 && ok[0].path && isDesktop() ? (
            <>
              <Button type="button" size="sm" variant="outline" onClick={() => void window.go?.main.App.RevealPath(ok[0].path!)}>
                打开所在文件夹
              </Button>
              <Button type="button" size="sm" onClick={() => void window.go?.main.App.OpenPath(ok[0].path!)}>
                打开文件
              </Button>
            </>
          ) : null}
        </div>
        {bad.length > 0 ? (
          <ul className="mt-3 space-y-1 text-sm text-cinnabar">
            {bad.map((item) => (
              <li key={item.title + item.error}>《{item.title}》{item.error}</li>
            ))}
          </ul>
        ) : null}
      </div>
    </div>
  )
}
