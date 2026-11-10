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
    <aside className="flex shrink-0 flex-col border-b border-ink bg-ink text-paper-card md:h-full md:w-56 md:border-b-0 md:border-r md:border-black/40">
      <div className="flex items-center gap-3 px-4 pb-3 pt-4">
        <img src={logo} alt="" className="h-10 w-10 rounded-lg shadow-sm" />
        <div className="min-w-0">
          <div className="font-serif text-lg tracking-[0.18em]">墨册</div>
          <div className="text-[11px] text-paper-card/55">本地书架 · 不上传</div>
        </div>
      </div>
      <div className="px-4 pb-2 text-[11px] tracking-[0.22em] text-paper-card/35">工具</div>
      <nav className="flex gap-1 overflow-x-auto px-2 pb-3 md:block md:flex-1 md:space-y-1 md:overflow-y-auto md:px-2">
        <button
          type="button"
          onClick={onProgress}
          className={cn(
            "flex min-w-[12rem] items-start gap-2 border-l-2 px-3 py-2.5 text-left md:mb-1 md:w-full md:min-w-0",
            progress ? "border-cinnabar bg-white/10" : "border-transparent hover:bg-white/5",
          )}
        >
          <ListChecks className={cn("mt-0.5 h-4 w-4 shrink-0", progress ? "text-cinnabar" : "text-paper-card/40")} />
          <span>
            <span className="block text-sm">进度</span>
            <span className="mt-0.5 block text-[11px] leading-4 text-paper-card/50">排队、耗时和已完成的书</span>
          </span>
        </button>
        <button
          type="button"
          onClick={onAccounts}
          className={cn(
            "flex min-w-[12rem] items-start gap-2 border-l-2 px-3 py-2.5 text-left md:mb-1 md:w-full md:min-w-0",
            accounts ? "border-cinnabar bg-white/10" : "border-transparent hover:bg-white/5",
          )}
        >
          <KeyRound className={cn("mt-0.5 h-4 w-4 shrink-0", accounts ? "text-cinnabar" : "text-paper-card/40")} />
          <span>
            <span className="block text-sm">账号</span>
            <span className="mt-0.5 block text-[11px] leading-4 text-paper-card/50">刮搜登录，保存在本文件夹</span>
          </span>
        </button>
        {tools.map((tool) => {
          const on = tool.id === active
          return (
            <button
              key={tool.id}
              type="button"
              onClick={() => onSelect(tool.id)}
              className={cn(
                "flex min-w-[12rem] items-start gap-2 border-l-2 px-3 py-2.5 text-left md:w-full md:min-w-0",
                on ? "border-cinnabar bg-white/10" : "border-transparent hover:bg-white/5",
              )}
            >
              <BookOpen className={cn("mt-0.5 h-4 w-4 shrink-0", on ? "text-cinnabar" : "text-paper-card/40")} />
              <span className="min-w-0">
                <span className="block truncate text-sm">{tool.name}</span>
                <span className="mt-0.5 block text-[11px] leading-4 text-paper-card/50">{tool.description}</span>
              </span>
            </button>
          )
        })}
      </nav>
    </aside>
  )
}
