import { useEffect, useMemo, useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { callTool } from "@/lib/api"
import { errorMessage, isDesktop } from "@/lib/utils"
import { FolderOpen, RotateCcw, Search, Trash2 } from "lucide-react"

export type ProgressJob = {
  id: string
  name: string
  title: string
  status: string
  step: string
  progress: number
  error?: string
  outputPath?: string
  format: string
  elapsed: string
  eta?: string
  batch: string
}

const filters = [
  { id: "all", label: "全部" },
  { id: "queued", label: "排队" },
  { id: "running", label: "进行中" },
  { id: "done", label: "完成" },
  { id: "failed", label: "失败" },
]

function running(status: string) {
  return status === "extracting" || status === "detecting" || status === "packaging"
}

export function ProgressPage() {
  const [jobs, setJobs] = useState<ProgressJob[]>([])
  const [query, setQuery] = useState("")
  const [filter, setFilter] = useState("all")
  const [error, setError] = useState("")

  async function reload() {
    try {
      const data = await callTool<{ jobs: ProgressJob[] }>("txt2epub", "listJobs", {})
      setJobs(data.jobs || [])
      setError("")
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  useEffect(() => {
    void reload()
    const timer = window.setInterval(() => void reload(), 800)
    return () => window.clearInterval(timer)
  }, [])

  const shown = useMemo(() => {
    const q = query.trim()
    return jobs.filter((job) => {
      if (filter === "running" && !running(job.status)) return false
      if (filter !== "all" && filter !== "running" && job.status !== filter) return false
      if (!q) return true
      return `${job.title} ${job.name}`.includes(q)
    })
  }, [jobs, query, filter])

  return (
    <div className="mx-auto max-w-3xl space-y-4 p-3 md:p-6">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="text-[11px] tracking-[0.22em] text-cinnabar">进度</p>
          <h1 className="font-serif text-3xl tracking-wide">排版队列</h1>
          <p className="mt-1 text-sm text-muted">转换在后台进行，可以回到书架继续改其他书。</p>
        </div>
        <Button
          type="button"
          variant="outline"
          onClick={() => void callTool("txt2epub", "clearJobs", {}).then(() => reload())}
        >
          <Trash2 className="h-4 w-4" />
          清空历史
        </Button>
      </header>
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-[12rem] flex-1">
          <Search className="pointer-events-none absolute left-2 top-2 h-4 w-4 text-muted" />
          <Input aria-label="搜索进度" className="pl-8" placeholder="按书名筛选" value={query} onChange={(e) => setQuery(e.target.value)} />
        </div>
        {filters.map((item) => (
          <button
            key={item.id}
            type="button"
            onClick={() => setFilter(item.id)}
            className={`rounded-full border px-3 py-1 text-xs transition ${filter === item.id ? "border-cinnabar bg-cinnabar text-paper-card" : "border-line bg-paper-card text-muted"}`}
          >
            {item.label}
          </button>
        ))}
      </div>
      {error ? <p className="text-sm text-cinnabar">{error}</p> : null}
      {shown.length === 0 ? (
        <div className="rounded-xl border border-dashed border-line bg-paper-card/80 px-6 py-10 text-center shadow-card">
          <div className="mx-auto mb-3 grid h-16 w-12 place-items-center rounded-sm border border-cinnabar/40 bg-paper font-serif text-cinnabar">册</div>
          <p className="font-serif text-lg">还没有排版记录</p>
          <p className="mt-1 text-sm text-muted">从书架开始转换后，进度会留在这里。</p>
        </div>
      ) : (
        <ul className="space-y-3">
          {shown.map((job) => (
            <li key={job.id} className="rounded-xl border border-line bg-paper-card p-4 shadow-card transition hover:-translate-y-0.5">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="rounded-sm border border-cinnabar/30 px-1.5 py-0.5 text-[10px] tracking-wide text-cinnabar">{job.format || "TXT"}</span>
                    <h2 className="truncate font-serif text-lg">{job.title || job.name}</h2>
                  </div>
                  <p className="mt-1 text-xs text-muted">
                    {job.step}
                    {job.elapsed ? ` · ${job.elapsed}` : ""}
                    {job.eta ? ` · 剩余 ${job.eta}` : ""}
                  </p>
                </div>
                <span className="text-xs text-muted">{Math.round((job.progress || 0) * 100)}%</span>
              </div>
              <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-line">
                <div className="h-full bg-cinnabar transition-all" style={{ width: `${Math.round((job.progress || 0) * 100)}%` }} />
              </div>
              {job.error ? <p className="mt-2 text-sm text-cinnabar">{job.error}</p> : null}
              <div className="mt-3 flex flex-wrap gap-2">
                {running(job.status) || job.status === "queued" ? (
                  <Button type="button" size="sm" variant="outline" onClick={() => void callTool("txt2epub", "cancelJob", { id: job.id }).then(() => reload())}>
                    取消
                  </Button>
                ) : null}
                {job.status === "failed" || job.status === "cancelled" ? (
                  <Button type="button" size="sm" variant="outline" onClick={() => void callTool("txt2epub", "retryJob", { id: job.id }).then(() => reload())}>
                    <RotateCcw className="h-3.5 w-3.5" />
                    重试
                  </Button>
                ) : null}
                {job.status === "done" && job.outputPath && isDesktop() ? (
                  <>
                    <Button type="button" size="sm" variant="outline" onClick={() => void window.go?.main.App.RevealPath(job.outputPath!)}>
                      <FolderOpen className="h-3.5 w-3.5" />
                      打开所在文件夹
                    </Button>
                    <Button type="button" size="sm" onClick={() => void window.go?.main.App.OpenPath(job.outputPath!)}>
                      打开文件
                    </Button>
                  </>
                ) : null}
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
