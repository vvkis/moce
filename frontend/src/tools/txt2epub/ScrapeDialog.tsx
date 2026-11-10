import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { callTool } from "@/lib/api"
import { errorMessage } from "@/lib/utils"

export type ScrapeRow = {
  title: string
  author: string
  intro: string
  tags: string[]
  coverUrl: string
  coverHeaders?: Record<string, string>
  publisher: string
  year: string
  isbn: string
  rating: string
  series: string
  source: string
}

type Source = { id: string; name: string }

const fallbackSources: Source[] = [
  { id: "douban", name: "豆瓣" },
  { id: "weread", name: "微信读书" },
  { id: "qimao", name: "七猫小说" },
  { id: "jjwxc", name: "晋江文学城" },
  { id: "qidian", name: "起点中文网" },
  { id: "fanqie", name: "番茄小说" },
  { id: "changpei", name: "长佩文学" },
  { id: "xiaoxiang", name: "潇湘书院" },
]

type Change = { key: string; label: string; line: string }

export function ScrapeDialog({
  title,
  author,
  current,
  onClose,
  onApply,
}: {
  title: string
  author: string
  current: {
    title: string
    author: string
    intro: string
    series: string
    publisher: string
    published: string
    identifier: string
    tags: string
    rating: number
  }
  onClose: () => void
  onApply: (patch: Record<string, unknown>) => void
}) {
  const [sources, setSources] = useState<Source[]>(fallbackSources)
  const [query, setQuery] = useState(() => [title, author].map((s) => s.trim()).filter(Boolean).join(" "))
  const [source, setSource] = useState("douban")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [results, setResults] = useState<ScrapeRow[]>([])
  const [thumbs, setThumbs] = useState<Record<string, string>>({})
  const [picked, setPicked] = useState<ScrapeRow | null>(null)
  const [off, setOff] = useState<Record<string, boolean>>({})
  const [note, setNote] = useState("")

  useEffect(() => {
    void callTool<Source[]>("txt2epub", "scrapeSources", {}).then(setSources).catch(() => {})
    return () => {
      void callTool("txt2epub", "scrapeCancel", {}).catch(() => {})
    }
  }, [])

  async function search() {
    const q = query.trim()
    if (!q) {
      setError("书名不能为空")
      return
    }
    setBusy(true)
    setError("")
    setNote("")
    setPicked(null)
    setResults([])
    try {
      const res = await callTool<{ results: ScrapeRow[] }>("txt2epub", "scrape", { query: q, source })
      setResults(res.results || [])
      void loadThumbs(res.results || [])
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  async function loadThumbs(rows: ScrapeRow[]) {
    const next: Record<string, string> = {}
    await Promise.all(
      rows.slice(0, 10).map(async (row) => {
        if (!row.coverUrl) return
        try {
          const cover = await callTool<{ dataBase64: string; mime: string }>("txt2epub", "scrapeCover", {
            url: row.coverUrl,
            headers: row.coverHeaders || {},
          })
          next[row.coverUrl] = `data:${cover.mime};base64,${cover.dataBase64}`
        } catch {
          /* 缩略图失败仍可选用文字结果 */
        }
      }),
    )
    setThumbs((prev) => ({ ...prev, ...next }))
  }

  function choose(row: ScrapeRow) {
    const changes = diff(current, row)
    if (changes.length === 0) {
      setNote("元数据已是最新，无需修改")
      setPicked(null)
      return
    }
    setOff({})
    setNote("")
    setPicked(row)
  }

  async function apply() {
    if (!picked) return
    const changes = diff(current, picked).filter((item) => !off[item.key])
    if (changes.length === 0) {
      setNote("没有选中任何修改")
      return
    }
    const patch: Record<string, unknown> = {}
    for (const item of changes) {
      if (item.key === "title") {
        patch.title = picked.title
        patch.titleTouched = true
      } else if (item.key === "author") {
        patch.author = picked.author
        patch.authorTouched = true
      } else if (item.key === "intro") patch.intro = picked.intro
      else if (item.key === "series") patch.series = picked.series
      else if (item.key === "publisher") patch.publisher = picked.publisher
      else if (item.key === "year") patch.published = picked.year
      else if (item.key === "isbn") patch.identifier = picked.isbn
      else if (item.key === "rating") patch.rating = stars(picked.rating)
    }
    if (!off.cover && picked.coverUrl) {
      try {
        const cover = await callTool<{ dataBase64: string; name: string; path: string }>("txt2epub", "scrapeCover", {
          url: picked.coverUrl,
          headers: picked.coverHeaders || {},
        })
        if (cover.path) await callTool("txt2epub", "scrapeKeep", { path: cover.path })
        patch.cover = { name: cover.name || "cover.jpg", dataBase64: cover.dataBase64 }
        patch.coverMode = "file"
      } catch (err) {
        setError(`封面下载失败：${errorMessage(err)}`)
      }
    }
    onApply(patch)
    onClose()
  }

  const changes = picked ? diff(current, picked) : []
  const sourceName = sources.find((item) => item.id === source)?.name || source

  return (
    <div className="fixed inset-0 z-30 flex items-end justify-center bg-black/40 p-3 sm:items-center" onMouseDown={onClose}>
      <div
        className="flex max-h-[86vh] w-full max-w-xl flex-col overflow-hidden rounded-xl border border-line bg-paper shadow-card"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="flex items-baseline justify-between gap-3 border-b border-line px-4 py-3">
          <div>
            <h2 className="font-serif text-2xl">刮搜</h2>
            <p className="text-[11px] text-muted">选择数据来源</p>
          </div>
          <Button type="button" size="sm" variant="outline" onClick={onClose}>
            关闭
          </Button>
        </div>
        <div className="space-y-3 overflow-auto px-4 py-3">
          <label className="block space-y-1">
            <span className="text-xs text-muted">书名</span>
            <Input aria-label="刮搜关键词" value={query} onChange={(e) => setQuery(e.target.value)} />
          </label>
          <div className="flex flex-wrap gap-2">
            <select
              aria-label="数据来源"
              className="h-9 min-w-0 flex-1 rounded-md border border-line bg-[#fffaf3] px-3 text-sm"
              value={source}
              onChange={(e) => setSource(e.target.value)}
            >
              {sources.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </select>
            <Button type="button" disabled={busy} onClick={() => void search()}>
              {busy ? "搜索中..." : "刮搜"}
            </Button>
          </div>
          {error ? <p className="text-sm text-cinnabar">{error}</p> : null}
          {note ? <p className="text-sm text-muted">{note}</p> : null}
          {picked ? (
            <div className="space-y-2 rounded-lg border border-line bg-paper-card p-3">
              <h3 className="text-sm">确认修改</h3>
              <Button type="button" size="sm" onClick={() => void apply()}>
                应用所选修改
              </Button>
              {changes.map((item) => (
                <label key={item.key} className="flex items-start gap-2 text-sm">
                  <input
                    type="checkbox"
                    className="mt-1"
                    checked={!off[item.key]}
                    onChange={() => setOff((prev) => ({ ...prev, [item.key]: !prev[item.key] }))}
                  />
                  <span>
                    <span className="text-muted">{item.label}：</span>
                    {item.line}
                  </span>
                </label>
              ))}
            </div>
          ) : null}
          {results.length > 0 ? (
            <div>
              <p className="mb-2 text-xs text-muted">
                {sourceName} · {results.length} 条结果
              </p>
              <ul className="space-y-2">
                {results.map((row, index) => (
                  <li key={`${row.title}-${index}`}>
                    <button
                      type="button"
                      className="flex w-full gap-3 rounded-lg border border-line bg-[#fffaf3] p-2 text-left hover:border-cinnabar"
                      onClick={() => choose(row)}
                    >
                      {thumbs[row.coverUrl] ? (
                        <img src={thumbs[row.coverUrl]} alt="" className="h-16 w-12 shrink-0 rounded-sm object-cover" />
                      ) : (
                        <span className="flex h-16 w-12 shrink-0 items-center justify-center rounded-sm bg-paper text-[10px] text-muted">无封面</span>
                      )}
                      <span className="min-w-0">
                        <span className="block truncate font-serif text-sm">{row.title || "未命名"}</span>
                        <span className="block truncate text-xs text-muted">
                          {[row.author || "佚名", sourceName, row.rating && `评分 ${row.rating}`, row.year].filter(Boolean).join(" · ")}
                        </span>
                        {row.intro ? <span className="mt-1 line-clamp-2 block text-xs text-muted">{row.intro}</span> : null}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  )
}

function diff(
  current: {
    title: string
    author: string
    intro: string
    series: string
    publisher: string
    published: string
    identifier: string
    tags: string
    rating: number
  },
  row: ScrapeRow,
): Change[] {
  const fields: { key: string; label: string; old: string; next: string; norm?: (v: string) => string }[] = [
    { key: "title", label: "书名", old: current.title, next: row.title },
    { key: "author", label: "作者", old: current.author, next: row.author },
    { key: "intro", label: "简介", old: current.intro, next: row.intro },
    { key: "series", label: "系列", old: current.series, next: row.series },
    { key: "publisher", label: "出版社", old: current.publisher, next: row.publisher },
    { key: "year", label: "出版年份", old: current.published, next: row.year, norm: year },
    { key: "isbn", label: "ISBN", old: current.identifier, next: row.isbn, norm: digits },
    { key: "rating", label: "评分", old: current.rating ? String(current.rating) : "", next: row.rating ? String(stars(row.rating)) : "" },
  ]
  const out: Change[] = []
  for (const field of fields) {
    const prev = field.old.trim()
    const next = field.next.trim()
    const norm = field.norm || ((v: string) => v)
    if (next && norm(prev) !== norm(next)) {
      const line = field.key === "rating" ? `${prev || "未评分"} → ${row.rating}（${next} 星）` : `${clip(prev, 12) || "空"} → ${clip(next, 18)}`
      out.push({ key: field.key, label: field.label, line })
    }
  }
  if (row.coverUrl) out.push({ key: "cover", label: "封面", line: "更换为新封面" })
  return out
}

function digits(v: string) {
  return v.replace(/[^0-9Xx]/g, "").toUpperCase()
}

function year(v: string) {
  return v.match(/^\d{4}/)?.[0] || v
}

function stars(rating: string) {
  const n = Number.parseFloat(rating)
  if (!n || n <= 0) return 0
  const value = n > 5 ? Math.round(n / 2) : Math.round(n)
  return Math.max(1, Math.min(5, value))
}

function clip(s: string, max: number) {
  const text = s.replace(/\s+/g, " ")
  const chars = Array.from(text)
  if (chars.length <= max) return text
  return chars.slice(0, max).join("") + "…"
}
