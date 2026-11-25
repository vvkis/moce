import { useEffect, useMemo, useRef, useState } from "react"
import type { ProbeEntry, TocEdit } from "@/lib/types"
import { Input } from "@/components/ui/input"
import {
  AlertTriangle,
  ArrowLeft,
  ArrowRight,
  Check,
  ChevronRight,
  GitMerge,
  Pencil,
  RotateCcw,
  Search,
  X,
} from "lucide-react"
import {
  collectBranchIds,
  countAnomalies,
  entryAnomaly,
  filterToc,
  groupToc,
  visibleToc,
  type TocNode,
} from "./toc"

const rowHeight = 46
const overscan = 8

function formatRunes(n?: number) {
  if (!n || n <= 0) return ""
  if (n >= 10000) return `${(n / 10000).toFixed(1)}万字`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k字`
  return `${n}字`
}

export function ChapterPreview({
  entries,
  volumeCount,
  chapterCount,
  runeCount,
  edits,
  onEdit,
  onResetEdits,
}: {
  entries: ProbeEntry[]
  volumeCount: number
  chapterCount: number
  runeCount: number
  edits: Readonly<Record<number, TocEdit>>
  onEdit: (index: number, patch: Partial<TocEdit>) => void
  onResetEdits: () => void
}) {
  const tree = useMemo(() => groupToc(entries, edits), [entries, edits])
  const anomalyCount = useMemo(() => countAnomalies(tree), [tree])
  const [query, setQuery] = useState("")
  const [onlyAnomalies, setOnlyAnomalies] = useState(false)
  const filteredTree = useMemo(
    () => filterToc(tree, query, onlyAnomalies),
    [tree, query, onlyAnomalies],
  )
  const branches = useMemo(() => collectBranchIds(filteredTree), [filteredTree])
  const [collapsed, setCollapsed] = useState<Set<number>>(() => new Set())
  const [editingIdx, setEditingIdx] = useState<number | null>(null)
  const [draftTitle, setDraftTitle] = useState("")

  const effectiveCollapsed = query.trim() || onlyAnomalies ? new Set<number>() : collapsed
  const rows = useMemo(() => visibleToc(filteredTree, effectiveCollapsed), [filteredTree, effectiveCollapsed])

  const scroller = useRef<HTMLDivElement>(null)
  const [scrollTop, setScrollTop] = useState(0)
  const [viewHeight, setViewHeight] = useState(340)
  const frame = useRef(0)

  // Reset collapse/filter state whenever a new book or new probe result arrives.
  useEffect(() => {
    setCollapsed(new Set())
    setEditingIdx(null)
    setQuery("")
    setOnlyAnomalies(false)
    if (scroller.current) scroller.current.scrollTop = 0
    setScrollTop(0)
  }, [entries])

  useEffect(() => {
    const el = scroller.current
    if (!el) return
    const measure = () => setViewHeight(el.clientHeight || 340)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  useEffect(() => {
    const el = scroller.current
    if (!el) return
    const max = Math.max(0, rows.length * rowHeight - el.clientHeight)
    if (el.scrollTop > max) el.scrollTop = max
  }, [rows.length])

  const start = Math.max(0, Math.floor(scrollTop / rowHeight) - overscan)
  const count = Math.ceil(viewHeight / rowHeight) + overscan * 2
  const slice = rows.slice(start, start + count)
  const editCount = Object.keys(edits).length

  function toggle(index: number) {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(index)) next.delete(index)
      else next.add(index)
      return next
    })
  }

  function startRename(node: TocNode) {
    setEditingIdx(node.index)
    setDraftTitle(node.entry.title)
  }

  function commitRename(index: number) {
    const trimmed = draftTitle.trim()
    if (trimmed) onEdit(index, { title: trimmed })
    setEditingIdx(null)
  }

  return (
    <div className="rounded-xl border border-line bg-paper-card p-4 shadow-card">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-baseline gap-2.5">
          <h2 className="text-sm font-semibold text-ink">目录预览与微调</h2>
          <span className="text-xs text-muted">
            {volumeCount > 0 ? `${volumeCount} 卷/部 · ` : ""}
            {chapterCount} 章 · {runeCount.toLocaleString("zh-CN")} 字
          </span>
        </div>
        <div className="flex flex-wrap items-center gap-2 text-xs">
          {anomalyCount > 0 ? (
            <button
              type="button"
              onClick={() => setOnlyAnomalies((v) => !v)}
              className={`inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 font-medium transition ${
                onlyAnomalies
                  ? "border-warn bg-warn text-white"
                  : "border-amber-200 bg-warn-soft text-warn hover:border-amber-300"
              }`}
            >
              <AlertTriangle className="h-3 w-3" />
              异常 {anomalyCount} 处
            </button>
          ) : null}
          {editCount > 0 ? (
            <button
              type="button"
              onClick={onResetEdits}
              className="inline-flex items-center gap-1 rounded-full border border-line bg-paper px-2.5 py-0.5 text-ink hover:bg-paper-deep"
            >
              <RotateCcw className="h-3 w-3" />
              还原微调 ({editCount})
            </button>
          ) : null}
          {branches.length > 0 && !query.trim() && !onlyAnomalies ? (
            <>
              <button type="button" className="text-muted hover:text-ink" onClick={() => setCollapsed(new Set())}>
                全部展开
              </button>
              <span className="text-line">|</span>
              <button
                type="button"
                className="text-muted hover:text-ink"
                onClick={() => setCollapsed(new Set(branches))}
              >
                全部收起
              </button>
            </>
          ) : null}
        </div>
      </div>

      <div className="mt-3 flex items-center gap-2">
        <div className="relative flex-1">
          <Search className="pointer-events-none absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted" />
          <Input
            aria-label="搜索章节"
            className="h-8 pl-8 pr-7 text-xs"
            placeholder="搜索目录标题…（悬停行可改名、升降级或并入上一章）"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          {query ? (
            <button
              type="button"
              aria-label="清空搜索"
              className="absolute right-2 top-2 text-muted hover:text-ink"
              onClick={() => setQuery("")}
            >
              <X className="h-4 w-4" />
            </button>
          ) : null}
        </div>
      </div>

      {rows.length === 0 ? (
        <div className="mt-3 flex h-40 items-center justify-center rounded-lg border border-dashed border-line bg-paper/60 text-xs text-muted">
          没有符合条件的目录项
        </div>
      ) : (
        <div
          ref={scroller}
          className="mt-3 h-80 overflow-auto rounded-lg border border-line/70 bg-paper/40 p-1"
          onScroll={(event) => {
            const top = event.currentTarget.scrollTop
            cancelAnimationFrame(frame.current)
            frame.current = requestAnimationFrame(() => setScrollTop(top))
          }}
        >
          <div className="relative" style={{ height: rows.length * rowHeight }}>
            {slice.map((node, offset) => {
              const open = !effectiveCollapsed.has(node.index)
              const expandable = node.children.length > 0
              const anomaly = entryAnomaly(node)
              const isEditing = editingIdx === node.index
              return (
                <div
                  key={node.index}
                  className="group absolute left-0 right-0 flex items-center gap-1.5 overflow-hidden rounded-md px-2 hover:bg-white hover:shadow-sm"
                  style={{ top: (start + offset) * rowHeight, height: rowHeight, paddingLeft: 8 + node.depth * 18 }}
                >
                  {expandable ? (
                    <button
                      type="button"
                      aria-expanded={open}
                      aria-label={open ? "收起" : "展开"}
                      className="grid h-5 w-5 shrink-0 place-items-center rounded text-muted hover:bg-paper hover:text-ink"
                      onClick={() => toggle(node.index)}
                    >
                      <ChevronRight className={`h-3.5 w-3.5 transition-transform ${open ? "rotate-90" : ""}`} />
                    </button>
                  ) : (
                    <span className="h-5 w-5 shrink-0" />
                  )}

                  <div className="min-w-0 flex-1">
                    {isEditing ? (
                      <div className="flex items-center gap-1">
                        <input
                          autoFocus
                          className="h-7 flex-1 rounded border border-ink/40 bg-white px-2 text-xs text-ink outline-none"
                          value={draftTitle}
                          onChange={(e) => setDraftTitle(e.target.value)}
                          onKeyDown={(e) => {
                            if (e.key === "Enter") commitRename(node.index)
                            if (e.key === "Escape") setEditingIdx(null)
                          }}
                        />
                        <button
                          type="button"
                          aria-label="保存标题"
                          className="grid h-6 w-6 place-items-center rounded bg-ink text-white"
                          onClick={() => commitRename(node.index)}
                        >
                          <Check className="h-3.5 w-3.5" />
                        </button>
                        <button
                          type="button"
                          aria-label="取消改名"
                          className="grid h-6 w-6 place-items-center rounded border border-line bg-white text-muted hover:text-ink"
                          onClick={() => setEditingIdx(null)}
                        >
                          <X className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    ) : (
                      <>
                        <div className="flex items-center gap-1.5">
                          <span
                            className={`truncate text-[13px] ${expandable ? "font-semibold text-ink" : "text-ink"}`}
                            onDoubleClick={() => startRename(node)}
                            title={node.entry.title}
                          >
                            {node.entry.title}
                          </span>
                          {node.edited ? (
                            <span className="shrink-0 rounded bg-accent-soft px-1 py-0.5 text-[10px] font-medium text-accent">
                              已改
                            </span>
                          ) : null}
                        </div>
                        {node.entry.snippet ? (
                          <div className="truncate text-[11px] text-muted">{node.entry.snippet}</div>
                        ) : null}
                      </>
                    )}
                  </div>

                  {!isEditing ? (
                    <div className="flex shrink-0 items-center gap-1">
                      {expandable ? (
                        <span className="rounded-full bg-paper-deep px-2 py-0.5 text-[10px] font-medium text-muted">
                          {node.leafCount} 章
                        </span>
                      ) : anomaly ? (
                        <span
                          className="inline-flex items-center gap-0.5 rounded-full bg-warn-soft px-2 py-0.5 text-[10px] font-medium text-warn ring-1 ring-amber-200"
                          title={anomaly === "short" ? "正文字数少于 50 字，疑似误识别标题" : "正文字数超过 2 万字，疑似漏分章"}
                        >
                          <AlertTriangle className="h-2.5 w-2.5" />
                          {anomaly === "short" ? `过短 ${formatRunes(node.entry.runes) || "0字"}` : `过长 ${formatRunes(node.entry.runes)}`}
                        </span>
                      ) : node.entry.runes ? (
                        <span className="font-mono text-[11px] text-muted/80 group-hover:hidden">
                          {formatRunes(node.entry.runes)}
                        </span>
                      ) : null}

                      <div className="hidden items-center gap-0.5 group-hover:flex">
                        <button
                          type="button"
                          title="重命名标题"
                          aria-label="重命名标题"
                          className="grid h-6 w-6 place-items-center rounded text-muted hover:bg-paper hover:text-ink"
                          onClick={() => startRename(node)}
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </button>
                        {node.entry.level > 1 ? (
                          <button
                            type="button"
                            title="提升一级"
                            aria-label="提升一级"
                            className="grid h-6 w-6 place-items-center rounded text-muted hover:bg-paper hover:text-ink"
                            onClick={() => onEdit(node.index, { level: node.entry.level - 1, delta: 0 })}
                          >
                            <ArrowLeft className="h-3.5 w-3.5" />
                          </button>
                        ) : null}
                        <button
                          type="button"
                          title="降级为子目录"
                          aria-label="降级为子目录"
                          className="grid h-6 w-6 place-items-center rounded text-muted hover:bg-paper hover:text-ink"
                          onClick={() => onEdit(node.index, { level: node.entry.level + 1, delta: 0 })}
                        >
                          <ArrowRight className="h-3.5 w-3.5" />
                        </button>
                        <button
                          type="button"
                          title="并入上一章（移除该目录项，不丢正文）"
                          aria-label="并入上一章"
                          className="grid h-6 w-6 place-items-center rounded text-muted hover:bg-danger-soft hover:text-danger"
                          onClick={() => onEdit(node.index, { merge: true })}
                        >
                          <GitMerge className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    </div>
                  ) : null}
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
