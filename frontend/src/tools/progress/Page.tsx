import { useEffect, useMemo, useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { callTool } from "@/lib/api"
import { errorMessage, isDesktop } from "@/lib/utils"
import { ArrowRight, FolderOpen, RotateCcw, Search, Trash2 } from "lucide-react"

export type ProgressJob = {
  id: string
  name: string
  title: string
  status: string
  step: string
  progress: number
  error?: string
  outputPath?: string
  outputFormat?: string
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
    const q = query.trim().toLowerCase()
    return jobs.filter((job) => {
      if (filter === "running" && !running(job.status)) return false
      if (filter !== "all" && filter !== "running" && job.status !== filter) return false
      if (!q) return true
      return `${job.title} ${job.name}`.toLowerCase().includes(q)
    })
  }, [jobs, query, filter])

  return (
    <div className="mx-auto max-w-3xl space-y-4 p-4 md:p-6">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line pb-4">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink">转换进度</h1>
          <p className="mt-1 text-xs text-muted">任务在后台依次执行，你可以随时切回工作区继续整理其他书。</p>
        </div>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => void callTool("txt2epub", "clearJobs", {}).then(() => reload())}
        >
          <Trash2 className="h-3.5 w-3.5" />
          清空已完成
        </Button>
      </header>
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-[12rem] flex-1">
          <Search className="pointer-events-none absolute left-2.5 top-2.5 h-4 w-4 text-muted" />
          <Input aria-label="搜索进度" className="pl-8" placeholder="按书名筛选" value={query} onChange={(e) => setQuery(e.target.value)} />
        </div>
        <div className="flex rounded-lg border border-line bg-paper-deep/60 p-0.5">
          {filters.map((item) => (
            <button
              key={item.id}
              type="button"
              onClick={() => setFilter(item.id)}
              className={`rounded-md px-2.5 py-1 text-xs font-medium transition ${
                filter === item.id ? "bg-white text-ink shadow-sm" : "text-muted hover:text-ink"
              }`}
            >
              {item.label}
            </button>
          ))}
        </div>
      </div>
      {error ? <p className="rounded-lg border border-red-200 bg-danger-soft px-3 py-2 text-xs text-danger">{error}</p> : null}
      {shown.length === 0 ? (
        <div className="rounded-xl border border-dashed border-line bg-paper-card px-6 py-12 text-center shadow-card">
          <p className="text-sm font-medium text-ink">还没有转换记录</p>
          <p className="mt-1 text-xs text-muted">从「电子书重排」开始转换后，实时进度和导出路径会显示在这里。</p>
        </div>
      ) : (
        <ul className="space-y-2.5">
          {shown.map((job) => (
            <li key={job.id} className="rounded-xl border border-line bg-paper-card p-4 shadow-card">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-1.5">
                    <span className="inline-flex items-center gap-1 rounded-md border border-line bg-paper px-1.5 py-0.5 font-mono text-[10px] font-medium text-ink">
                      {job.format || "TXT"}
                      <ArrowRight className="h-2.5 w-2.5 text-muted" />
                      {job.outputFormat || "EPUB"}
                    </span>
                    <h2 className="truncate text-sm font-semibold text-ink">{job.title || job.name}</h2>
                  </div>
                  <p className="mt-1 text-xs text-muted">
                    {job.step}
                    {job.elapsed ? ` · ${job.elapsed}` : ""}
                    {job.eta ? ` · 剩余 ${job.eta}` : ""}
                  </p>
                </div>
                <span className="font-mono text-xs font-medium text-muted">{Math.round((job.progress || 0) * 100)}%</span>
              </div>
              <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-paper-deep">
                <div
                  className={`h-full transition-all ${job.status === "failed" ? "bg-danger" : "bg-ink"}`}
                  style={{ width: `${Math.round((job.progress || 0) * 100)}%` }}
                />
              </div>
              {job.error ? <p className="mt-2 text-xs text-danger">{job.error}</p> : null}
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
