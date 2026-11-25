import type { ProbeEntry, TocEdit } from "../../lib/types"

// A preview row. Depth comes from the TOC tree already produced by chapter
// recognition. A node is collapsible only when it has children.
export type TocNode = {
  entry: ProbeEntry
  index: number // original index in ProbeResult.entries (for stable edit keys)
  depth: number
  children: TocNode[]
  leafCount: number
  edited?: boolean
}

export type AnomalyKind = "short" | "long" | null

export const SHORT_CHAPTER_RUNES = 50
export const LONG_CHAPTER_RUNES = 20000

export function entryAnomaly(node: TocNode): AnomalyKind {
  // Branch containers (volumes/parts) are allowed to have 0 own runes.
  if (node.children.length > 0) return null
  const runes = node.entry.runes ?? 0
  if (runes < SHORT_CHAPTER_RUNES) return "short"
  if (runes > LONG_CHAPTER_RUNES) return "long"
  return null
}

export function applyTocEdits(
  entries: ProbeEntry[],
  edits: Readonly<Record<number, TocEdit>>,
): { items: Array<{ entry: ProbeEntry; origIndex: number; edited: boolean }>; mergedCount: number } {
  const items: Array<{ entry: ProbeEntry; origIndex: number; edited: boolean }> = []
  let leadingRunes = 0
  let leadingParas = 0
  let mergedCount = 0

  for (let i = 0; i < entries.length; i++) {
    const raw = entries[i]
    const ed = edits[i]
    if (ed?.merge) {
      mergedCount++
      const addRunes = (raw.runes ?? 0) + raw.title.length
      const addParas = raw.paragraphs + 1
      if (items.length > 0) {
        const prev = items[items.length - 1]
        prev.entry = {
          ...prev.entry,
          runes: (prev.entry.runes ?? 0) + addRunes,
          paragraphs: prev.entry.paragraphs + addParas,
        }
      } else {
        leadingRunes += addRunes
        leadingParas += addParas
      }
      continue
    }

    let level = raw.level > 0 ? raw.level : 1
    let title = raw.title
    let edited = false
    if (ed) {
      if (ed.title && ed.title.trim()) {
        title = ed.title.trim()
        edited = true
      }
      if (ed.level && ed.level > 0) {
        level = ed.level
        edited = true
      } else if (ed.delta) {
        level += ed.delta
        edited = true
      }
    }
    if (level < 1) level = 1
    if (items.length === 0) {
      level = 1
    } else if (level > items[items.length - 1].entry.level + 1) {
      level = items[items.length - 1].entry.level + 1
    }

    const runes = (raw.runes ?? 0) + (items.length === 0 ? leadingRunes : 0)
    const paragraphs = raw.paragraphs + (items.length === 0 ? leadingParas : 0)
    if (items.length === 0) {
      leadingRunes = 0
      leadingParas = 0
    }
    items.push({
      entry: { ...raw, title, level, runes, paragraphs },
      origIndex: i,
      edited,
    })
  }

  return { items, mergedCount }
}

export function groupToc(entries: ProbeEntry[], edits?: Readonly<Record<number, TocEdit>>): TocNode[] {
  const source = edits && Object.keys(edits).length > 0
    ? applyTocEdits(entries, edits).items
    : entries.map((entry, origIndex) => ({ entry, origIndex, edited: false }))

  const roots: TocNode[] = []
  const stack: TocNode[] = []
  source.forEach(({ entry, origIndex, edited }) => {
    const level = entry.level > 0 ? entry.level : 1
    while (stack.length > 0 && nodeLevel(stack[stack.length - 1]) >= level) {
      stack.pop()
    }
    const node: TocNode = {
      entry: { ...entry, level },
      index: origIndex,
      depth: stack.length,
      children: [],
      leafCount: 0,
      edited,
    }
    if (stack.length === 0) roots.push(node)
    else stack[stack.length - 1].children.push(node)
    stack.push(node)
  })

  const computeCounts = (nodes: TocNode[]): number => {
    let total = 0
    for (const node of nodes) {
      if (node.children.length === 0) {
        node.leafCount = 0
        total += 1
      } else {
        const sub = computeCounts(node.children)
        node.leafCount = sub
        total += sub
      }
    }
    return total
  }
  computeCounts(roots)
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

export function countAnomalies(nodes: TocNode[]): number {
  let count = 0
  const walk = (list: TocNode[]) => {
    for (const node of list) {
      if (entryAnomaly(node) !== null) count++
      if (node.children.length > 0) walk(node.children)
    }
  }
  walk(nodes)
  return count
}

export function filterToc(nodes: TocNode[], query: string, onlyAnomalies: boolean): TocNode[] {
  const q = query.trim().toLowerCase()
  if (!q && !onlyAnomalies) return nodes
  const walk = (list: TocNode[]): TocNode[] => {
    const out: TocNode[] = []
    for (const node of list) {
      const filteredChildren = walk(node.children)
      const matchesQuery = !q || node.entry.title.toLowerCase().includes(q)
      const matchesAnomaly = !onlyAnomalies || entryAnomaly(node) !== null
      if ((matchesQuery && matchesAnomaly) || filteredChildren.length > 0) {
        out.push({ ...node, children: filteredChildren })
      }
    }
    return out
  }
  return walk(nodes)
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
