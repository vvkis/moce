import type { ProbeEntry } from "../../lib/types"

// A preview row. Depth comes from the TOC tree already produced by chapter
// recognition. A node is collapsible only when it has children.
export type TocNode = {
  entry: ProbeEntry
  index: number
  depth: number
  children: TocNode[]
}

export function groupToc(entries: ProbeEntry[]): TocNode[] {
  const roots: TocNode[] = []
  const stack: TocNode[] = []
  entries.forEach((entry, index) => {
    const level = entry.level > 0 ? entry.level : 1
    while (stack.length > 0 && nodeLevel(stack[stack.length - 1]) >= level) {
      stack.pop()
    }
    const node: TocNode = {
      entry: { ...entry, level },
      index,
      depth: stack.length,
      children: [],
    }
    if (stack.length === 0) roots.push(node)
    else stack[stack.length - 1].children.push(node)
    stack.push(node)
  })
  return roots
}

function nodeLevel(node: TocNode) {
  return node.entry.level > 0 ? node.entry.level : 1
}

export function collectBranchIds(nodes: TocNode[]): number[] {
  const ids: number[] = []
  const walk = (list: TocNode[]) => {
    for (const node of list) {
      if (node.children.length > 0) {
        ids.push(node.index)
        walk(node.children)
      }
    }
  }
  walk(nodes)
  return ids
}

// Visible rows after collapse. Collapsing a node hides every deeper descendant.
export function visibleToc(nodes: TocNode[], collapsed: ReadonlySet<number>): TocNode[] {
  const out: TocNode[] = []
  const walk = (list: TocNode[]) => {
    for (const node of list) {
      out.push(node)
      if (node.children.length > 0 && !collapsed.has(node.index)) walk(node.children)
    }
  }
  walk(nodes)
  return out
}
