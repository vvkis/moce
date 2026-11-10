import { useEffect, useMemo, useRef, useState } from "react"
import type { ProbeEntry } from "@/lib/types"
import { collectBranchIds, groupToc, visibleToc } from "./toc"

const rowHeight = 44
const overscan = 8

export function ChapterPreview({
  entries,
  volumeCount,
  chapterCount,
  runeCount,
}: {
  entries: ProbeEntry[]
  volumeCount: number
  chapterCount: number
  runeCount: number
}) {
  const tree = useMemo(() => groupToc(entries), [entries])
  const branches = useMemo(() => collectBranchIds(tree), [tree])
  const [collapsed, setCollapsed] = useState<Set<number>>(() => new Set())
  const rows = useMemo(() => visibleToc(tree, collapsed), [tree, collapsed])
  const scroller = useRef<HTMLDivElement>(null)
  const [scrollTop, setScrollTop] = useState(0)
  const [viewHeight, setViewHeight] = useState(320)
  const frame = useRef(0)

  useEffect(() => {
    const el = scroller.current
    if (!el) return
    const measure = () => setViewHeight(el.clientHeight || 320)
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

  function toggle(index: number) {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(index)) next.delete(index)
      else next.add(index)
      return next
    })
  }

  return (
    <div className="rounded-xl border border-line bg-paper-card p-4 shadow-card">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 className="font-serif text-lg tracking-wide">章节预览</h2>
        {branches.length > 0 ? (
          <div className="flex gap-3 text-xs">
            <button type="button" className="text-cinnabar hover:text-cinnabar-dark" onClick={() => setCollapsed(new Set())}>
              全部展开
            </button>
            <button
              type="button"
              className="text-cinnabar hover:text-cinnabar-dark"
              onClick={() => setCollapsed(new Set(branches))}
            >
              全部收起
            </button>
          </div>
        ) : null}
      </div>
      <p className="mt-1 text-xs text-muted">
        {volumeCount > 0 ? `${volumeCount} 卷 · ` : ""}
        {chapterCount} 章 · {runeCount.toLocaleString("zh-CN")} 字
      </p>
      <div
        ref={scroller}
        className="mt-3 h-80 overflow-auto pr-1"
        onScroll={(event) => {
          const top = event.currentTarget.scrollTop
          cancelAnimationFrame(frame.current)
          frame.current = requestAnimationFrame(() => setScrollTop(top))
        }}
      >
        <div className="relative" style={{ height: rows.length * rowHeight }}>
          {slice.map((node, offset) => {
            const open = !collapsed.has(node.index)
            const expandable = node.children.length > 0
            return (
              <div
                key={node.index}
                className="absolute left-0 right-0 flex items-start gap-1 overflow-hidden rounded px-1 hover:bg-paper"
                style={{ top: (start + offset) * rowHeight, height: rowHeight, paddingLeft: 4 + node.depth * 16 }}
              >
                {expandable ? (
                  <button
                    type="button"
                    aria-expanded={open}
                    aria-label={open ? "收起" : "展开"}
                    className="mt-1.5 grid h-4 w-4 shrink-0 place-items-center text-cinnabar"
                    onClick={() => toggle(node.index)}
                  >
                    <span
                      className={`block h-0 w-0 border-y-[4px] border-y-transparent border-l-[6px] border-l-current transition-transform ${open ? "rotate-90" : ""}`}
                    />
                  </button>
                ) : (
                  <span className="mt-1.5 h-4 w-4 shrink-0" />
                )}
                <div className="min-w-0 flex-1 py-1">
                  <div className={`truncate text-sm ${expandable ? "font-serif" : ""}`}>{node.entry.title}</div>
                  {node.entry.snippet ? <div className="truncate text-xs text-muted">{node.entry.snippet}</div> : null}
                </div>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}
