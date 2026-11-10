import type { ComponentType } from "react"
import type { ToolMeta } from "@/lib/types"
import { Txt2EpubPanel } from "./txt2epub/Panel"

export const panels: Record<string, ComponentType<{ meta: ToolMeta }>> = {
  txt2epub: Txt2EpubPanel,
}
