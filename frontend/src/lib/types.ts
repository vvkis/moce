export type ToolOption = { value: string; label: string }

export type ToolField = {
  key: string
  label: string
  type: string
  scope?: string
  default?: unknown
  min?: number
  max?: number
  step?: number
  help?: string
  options?: ToolOption[]
}

export type ToolMeta = {
  id: string
  name: string
  description: string
  panel: string
  schema: ToolField[]
}

export type ProbeEntry = {
  title: string
  level: number
  kind: string
  paragraphs: number
  snippet: string
}

export type ProbeResult = {
  title: string
  author: string
  encoding: string
  confidence: number
  bom: boolean
  fallback: boolean
  warning?: string
  chapterCount: number
  volumeCount: number
  runeCount: number
  entries: ProbeEntry[]
  publisher?: string
  language?: string
  published?: string
  identifier?: string
  intro?: string
  tags?: string
  coverBase64?: string
  coverName?: string
  format?: string
}

export type ConvertResult = {
  path?: string
  filename: string
  bytes: number
  dataBase64?: string
  chapterCount: number
}
