import logo from "@/assets/logo.png"
import type { ToolMeta } from "@/lib/types"
import { cn } from "@/lib/utils"
import { BookOpen, KeyRound, ListChecks } from "lucide-react"

export function Sidebar({
  tools,
  active,
  progress,
  accounts,
  onSelect,
  onProgress,
  onAccounts,
}: {
  tools: ToolMeta[]
  active: string
  progress: boolean
  accounts: boolean
  onSelect: (id: string) => void
  onProgress: () => void
  onAccounts: () => void
}) {
  return (
    <aside className="flex shrink-0 flex-col border-b border-line bg-[#f2f3f5] text-ink md:h-full md:w-56 md:border-b-0 md:border-r">
      <div className="flex items-center gap-2.5 px-4 pb-3 pt-4">
        <img src={logo} alt="" className="h-8 w-8 rounded-lg ring-1 ring-black/10" />
        <div className="min-w-0">
          <div className="text-sm font-semibold tracking-tight text-ink">墨册 Moce</div>
          <div className="text-[11px] text-muted">本地重排 · EPUB / AZW3</div>
        </div>
      </div>
      <div className="px-4 pb-1.5 pt-2 text-[11px] font-medium uppercase tracking-wider text-muted/80">工作区</div>
      <nav className="flex gap-1 overflow-x-auto px-2.5 pb-3 md:block md:flex-1 md:space-y-0.5 md:overflow-y-auto">
        {tools.map((tool) => {
          const on = tool.id === active
          return (
            <button
              key={tool.id}
              type="button"
              onClick={() => onSelect(tool.id)}
              className={cn(
                "flex min-w-[10rem] items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm md:w-full md:min-w-0",
                on ? "bg-white font-medium text-ink shadow-sm ring-1 ring-black/5" : "text-muted hover:bg-black/[0.04] hover:text-ink",
              )}
            >
              <BookOpen className={cn("h-4 w-4 shrink-0", on ? "text-ink" : "text-muted")} />
              <span className="truncate">{tool.name}</span>
            </button>
          )
        })}
        <button
          type="button"
          onClick={onProgress}
          className={cn(
            "flex min-w-[9rem] items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm md:w-full md:min-w-0",
            progress ? "bg-white font-medium text-ink shadow-sm ring-1 ring-black/5" : "text-muted hover:bg-black/[0.04] hover:text-ink",
          )}
        >
          <ListChecks className={cn("h-4 w-4 shrink-0", progress ? "text-ink" : "text-muted")} />
          <span className="truncate">转换进度</span>
        </button>
        <button
          type="button"
          onClick={onAccounts}
          className={cn(
            "flex min-w-[9rem] items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm md:w-full md:min-w-0",
            accounts ? "bg-white font-medium text-ink shadow-sm ring-1 ring-black/5" : "text-muted hover:bg-black/[0.04] hover:text-ink",
          )}
        >
          <KeyRound className={cn("h-4 w-4 shrink-0", accounts ? "text-ink" : "text-muted")} />
          <span className="truncate">刮搜账号</span>
        </button>
      </nav>
      <div className="hidden border-t border-line px-4 py-3 text-[11px] leading-4 text-muted md:block">
        纯本地处理 · 免 Calibre · 便携不写 C 盘
      </div>
    </aside>
  )
}
