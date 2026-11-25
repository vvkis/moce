import { useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react"
import sampleText from "../../../../samples/示例小说.txt?raw"
import { SchemaForm } from "@/components/SchemaForm"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { callTool } from "@/lib/api"
import type { ProbeResult, TocEdit, ToolField, ToolMeta } from "@/lib/types"
import {
  baseName,
  errorMessage,
  fileToBase64,
  formatBytes,
  isDesktop,
  utf8ToBase64,
} from "@/lib/utils"
import {
  AlertCircle,
  Check,
  ChevronDown,
  FilePlus2,
  FolderOpen,
  ImagePlus,
  RotateCcw,
  Search,
  SlidersHorizontal,
  Star,
  Trash2,
  Type,
} from "lucide-react"
import { ChapterPreview } from "./ChapterPreview"
import { ScrapeDialog } from "./ScrapeDialog"
import { ShellContext } from "@/shell"

type CoverMode = "auto" | "file" | "generate"
type OutputFormat = "epub" | "azw3" | "both"

const languages = [
  { value: "zh", label: "中文" },
  { value: "zh-Hant", label: "繁体中文" },
  { value: "en", label: "英文" },
  { value: "ja", label: "日文" },
  { value: "ko", label: "韩文" },
]

const outputFormatChoices: Array<{ value: OutputFormat; label: string; hint: string }> = [
  { value: "epub", label: "EPUB", hint: "KOReader / 通用" },
  { value: "azw3", label: "AZW3", hint: "Kindle 原生 KF8" },
  { value: "both", label: "双格式", hint: "EPUB + AZW3" },
]

type RegexPreset = {
  id: string
  label: string
  chapterPattern: string
  volumePattern: string
}

type Attachment = { path?: string; dataBase64?: string; name: string }

type Job = {
  id: string
  name: string
  path?: string
  dataBase64?: string
  encoding: string
  title: string
  author: string
  titleTouched: boolean
  authorTouched: boolean
  series: string
  seriesIndex: string
  tags: string
  intro: string
  publisher: string
  language: string
  published: string
  identifier: string
  rating: number
  scraped?: boolean
  coverMode: CoverMode
  cover?: Attachment
  tocEdits: Record<number, TocEdit>
  status: "probing" | "ready" | "converting" | "done" | "error"
  error?: string
  probe?: ProbeResult
  outputPath?: string
  outputName?: string
  outputBytes?: number
}

const detectKeys = [
  "chapterPattern",
  "volumePattern",
  "mergeBrokenLines",
  "dropEmptyLines",
  "removeDupCover",
  "fallbackRunes",
]

const quickOptionKeys = new Set(["volumePageStyle", "inlineToc", "includeCoverInToc"])

export function Txt2EpubPanel({ meta }: { meta: ToolMeta }) {
  const shell = useContext(ShellContext)
  const fileFields = meta.schema.filter((field) => field.scope === "file")
  const bookFields = meta.schema.filter((field) => field.scope !== "file")
  const encodingField = fileFields.find((field) => field.key === "encoding")
  const defaultChapterPat = String(bookFields.find((f) => f.key === "chapterPattern")?.default ?? "")
  const defaultVolumePat = String(bookFields.find((f) => f.key === "volumePattern")?.default ?? "")

  const regexPresets = useMemo<RegexPreset[]>(
    () => [
      {
        id: "default",
        label: "默认中文小说",
        chapterPattern: defaultChapterPat,
        volumePattern: defaultVolumePat,
      },
      {
        id: "numeric",
        label: "数字 001 标题",
        chapterPattern: `[0-9]{1,4}[\\.、\\s　]+\\S.*`,
        volumePattern: defaultVolumePat,
      },
      {
        id: "cn-num",
        label: "中文序号 一、二",
        chapterPattern: `[零〇一二三四五六七八九十百]+\\s*[、\\.\\s　]+\\S.*`,
        volumePattern: defaultVolumePat,
      },
      {
        id: "bracket",
        label: "【方括号】标题",
        chapterPattern: `【[^】]{1,40}】.*`,
        volumePattern: defaultVolumePat,
      },
    ],
    [defaultChapterPat, defaultVolumePat],
  )

  const quickFields = useMemo(
    () => bookFields.filter((f) => quickOptionKeys.has(f.key)),
    [bookFields],
  )
  const advancedFields = useMemo(
    () => bookFields.filter((f) => f.key !== "outputFormat" && !quickOptionKeys.has(f.key)),
    [bookFields],
  )

  const [options, setOptions] = useState<Record<string, unknown>>(() => defaults(bookFields))
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [jobs, setJobs] = useState<Job[]>([])
  const [selected, setSelected] = useState<string>("")
  const [outputDir, setOutputDir] = useState("")
  const [font, setFont] = useState<Attachment | null>(null)
  const [cover, setCover] = useState<Attachment | null>(null)
  const [banner, setBanner] = useState("")
  const [scrapeOpen, setScrapeOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const optionsRef = useRef(options)
  const jobsRef = useRef(jobs)
  optionsRef.current = options
  jobsRef.current = jobs
  const detectKey = detectKeys.map((key) => JSON.stringify(options[key] ?? null)).join("|")
  const inputRef = useRef<HTMLInputElement>(null)
  const coverRef = useRef<HTMLInputElement>(null)
  const fontRef = useRef<HTMLInputElement>(null)
  const coverTarget = useRef<"global" | string>("global")
  const settingsReady = useRef(false)

  const outputFormat = (options.outputFormat as OutputFormat) || "epub"

  function setOutputFormat(fmt: OutputFormat) {
    setOptions((prev) => ({ ...prev, outputFormat: fmt }))
  }

  function update(id: string, patch: Partial<Job> | ((job: Job) => Job)) {
    setJobs((prev) =>
      prev.map((job) => {
        if (job.id !== id) return job
        return typeof patch === "function" ? patch(job) : { ...job, ...patch }
      }),
    )
  }

  async function probe(job: Job, encoding = job.encoding) {
    update(job.id, { status: "probing", error: undefined, encoding, tocEdits: {} })
    try {
      const result = await callTool<ProbeResult>("txt2epub", "probe", {
        name: job.name,
        path: job.path,
        dataBase64: job.dataBase64,
        encoding,
        options: optionsRef.current,
      })
      update(job.id, (cur) => ({
        ...cur,
        status: "ready",
        probe: result,
        encoding,
        title: cur.titleTouched ? cur.title : result.title,
        author: cur.authorTouched ? cur.author : result.author,
        publisher: cur.publisher || result.publisher || "",
        language: result.language || cur.language,
        published: cur.published || result.published || "",
        identifier: cur.identifier || result.identifier || "",
        intro: cur.intro || result.intro || "",
        tags: cur.tags || result.tags || "",
        cover:
          cur.cover || !result.coverBase64
            ? cur.cover
            : { name: result.coverName || "cover.jpg", dataBase64: result.coverBase64 },
        coverMode: cur.cover || !result.coverBase64 ? cur.coverMode : "file",
        error: undefined,
      }))
    } catch (err) {
      update(job.id, { status: "error", error: errorMessage(err) })
    }
  }

  function addJob(partial: Pick<Job, "name" | "path" | "dataBase64">) {
    const job: Job = {
      id: crypto.randomUUID(),
      name: partial.name,
      path: partial.path,
      dataBase64: partial.dataBase64,
      encoding: "auto",
      title: "",
      author: "",
      titleTouched: false,
      authorTouched: false,
      series: "",
      seriesIndex: "",
      tags: "",
      intro: "",
      publisher: "",
      language: "zh",
      published: "",
      identifier: "",
      rating: 0,
      coverMode: "auto",
      tocEdits: {},
      status: "probing",
    }
    setJobs((prev) => [...prev, job])
    setSelected((cur) => cur || job.id)
    void probe(job)
  }

  async function addPaths(paths: string[]) {
    if (!window.go?.main.App.CollectTxtPaths) return
    try {
      const txts = await window.go.main.App.CollectTxtPaths(paths)
      setBanner("")
      for (const path of txts) addJob({ name: baseName(path), path })
    } catch (err) {
      setBanner(errorMessage(err))
    }
  }

  async function addBrowserFiles(list: FileList | File[]) {
    const files = Array.from(list).filter((file) => /\.(txt|epub|mobi|azw3)$/i.test(file.name))
    if (files.length === 0) {
      setBanner("请选择 txt、epub、mobi 或 azw3")
      return
    }
    setBanner("")
    for (const file of files) {
      const dataBase64 = await fileToBase64(file)
      addJob({ name: file.name, dataBase64 })
    }
  }

  useEffect(() => {
    if (!isDesktop() || !window.go?.main.App.LoadSettings) return
    void window.go.main.App.LoadSettings()
      .then((settings) => {
        if (settings?.outputDir) setOutputDir(settings.outputDir)
      })
      .catch(() => {})
      .finally(() => {
        settingsReady.current = true
      })
  }, [])

  useEffect(() => {
    if (!settingsReady.current || !isDesktop() || !window.go?.main.App.SaveSettings) return
    void window.go.main.App.SaveSettings({ outputDir }).catch(() => {})
  }, [outputDir])

  useEffect(() => {
    if (!isDesktop() || !window.runtime?.OnFileDrop) return
    window.runtime.OnFileDrop((_x, _y, paths) => {
      void addPaths(paths)
    }, false)
    return () => window.runtime?.OnFileDropOff()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      for (const job of jobsRef.current) {
        if (job.path || job.dataBase64) void probe(job, job.encoding)
      }
    }, 350)
    return () => window.clearTimeout(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [detectKey])

  const selectedJob = jobs.find((job) => job.id === selected) ?? jobs[0]
  const readyCount = jobs.filter((job) => job.probe).length
  const chapterTotal = jobs.reduce((sum, job) => sum + (job.probe?.chapterCount ?? 0), 0)

  const summary = useMemo(() => {
    if (jobs.length === 0) return "暂无文件"
    return `${jobs.length} 本 · ${chapterTotal.toLocaleString("zh-CN")} 章`
  }, [jobs.length, chapterTotal])

  function editSelectedToc(index: number, patch: Partial<TocEdit>) {
    if (!selectedJob) return
    update(selectedJob.id, (cur) => {
      const prev = cur.tocEdits[index] ?? { index }
      return {
        ...cur,
        tocEdits: {
          ...cur.tocEdits,
          [index]: { ...prev, ...patch, index },
        },
      }
    })
  }

  async function pickAttachment(kind: "cover" | "font", target: "global" | string) {
    if (kind === "cover") coverTarget.current = target
    if (isDesktop()) {
      const path = kind === "cover" ? await window.go!.main.App.PickCover() : await window.go!.main.App.PickFont()
      if (!path) return
      let dataBase64: string | undefined
      if (kind === "cover") {
        try {
          dataBase64 = await readLocalFile(path)
        } catch (err) {
          setBanner(errorMessage(err))
          return
        }
      }
      const item = { path, name: baseName(path), dataBase64 }
      if (target === "global") {
        if (kind === "cover") setCover(item)
        else setFont(item)
      } else if (kind === "cover") {
        update(target, { cover: item, coverMode: "file" })
      }
      return
    }
    if (kind === "cover") coverRef.current?.click()
    else fontRef.current?.click()
  }

  async function convertAll() {
    const list = jobsRef.current.filter((job) => job.probe)
    if (list.length === 0) {
      setBanner("先等章节识别完成")
      return
    }
    setBusy(true)
    setBanner("")
    const files = list.map((job) => {
      const current = jobsRef.current.find((item) => item.id === job.id) ?? job
      return {
        name: current.name,
        path: current.path,
        dataBase64: current.dataBase64,
        title: current.title,
        titleEdited: current.titleTouched,
        author: current.author,
        authorEdited: current.authorTouched,
        series: current.series,
        seriesIndex: current.seriesIndex,
        tags: current.tags,
        intro: current.intro,
        publisher: current.publisher,
        language: current.language,
        published: current.published,
        identifier: current.identifier,
        rating: current.rating,
        generateCover: current.coverMode === "generate",
        encoding: current.encoding,
        coverPath: chosenCover(current, cover)?.path || "",
        coverBase64: chosenCover(current, cover)?.dataBase64 || "",
        coverName: chosenCover(current, cover)?.name || "",
        fontPath: font?.path || "",
        fontBase64: font?.dataBase64 || "",
        fontName: font?.name || "",
        outputDir,
        outputFormat,
        tocEdits: Object.values(current.tocEdits),
        returnBytes: !isDesktop(),
        options: optionsRef.current,
      }
    })
    try {
      const queued = await callTool<{ batch: string }>("txt2epub", "enqueue", { files })
      shell.watchBatch(queued.batch)
      shell.openProgress()
    } catch (err) {
      setBanner(errorMessage(err))
    }
    setBusy(false)
  }

  return (
    <div className="flex min-h-full flex-col">
      <div className="flex-1 space-y-4 p-4 md:p-6">
        <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line pb-4">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <h1 className="text-xl font-semibold tracking-tight text-ink">{meta.name}</h1>
              <span className="rounded-full border border-line bg-white px-2.5 py-0.5 text-[11px] font-medium text-muted">
                {summary}
              </span>
            </div>
            <p className="mt-1 text-xs text-muted">
              支持 TXT、原生 EPUB、MOBI、AZW3 重新精排并输出为干净的 EPUB 或 Kindle AZW3，原文件不会被改写。
            </p>
          </div>
          <FormatSegmented value={outputFormat} onChange={setOutputFormat} />
        </header>

        {banner ? (
          <p className="rounded-lg border border-red-200 bg-danger-soft px-3 py-2 text-xs text-danger">{banner}</p>
        ) : null}

        <div className="grid items-start gap-4 xl:grid-cols-[minmax(17rem,21rem)_minmax(0,1fr)]">
          <section className="space-y-3">
            <div
              className="rounded-xl border border-dashed border-line bg-white p-4 shadow-card hover:border-ink/30"
              onDragOver={(e) => e.preventDefault()}
              onDrop={(e) => {
                e.preventDefault()
                if (!isDesktop() && e.dataTransfer.files.length) void addBrowserFiles(e.dataTransfer.files)
              }}
            >
              <div className="flex items-start gap-3">
                <div className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-paper text-ink ring-1 ring-black/5">
                  <FilePlus2 className="h-4 w-4" />
                </div>
                <div className="min-w-0 text-left">
                  <p className="text-sm font-medium text-ink">拖入书籍文件</p>
                  <p className="mt-0.5 text-xs leading-5 text-muted">TXT · EPUB · MOBI · AZW3，支持批量拖入</p>
                </div>
              </div>
              <div className="mt-3 flex flex-wrap gap-2">
                <Button
                  type="button"
                  size="sm"
                  onClick={() => {
                    if (isDesktop()) {
                      void window.go!.main.App.PickTxtFiles().then((paths) => {
                        if (paths?.length) void addPaths(paths)
                      })
                    } else {
                      inputRef.current?.click()
                    }
                  }}
                >
                  选择文件
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() =>
                    addJob({
                      name: "示例小说.txt",
                      dataBase64: utf8ToBase64(sampleText),
                    })
                  }
                >
                  载入示例
                </Button>
              </div>
              <input
                ref={inputRef}
                type="file"
                accept=".txt,.epub,.mobi,.azw3,text/plain"
                multiple
                className="hidden"
                onChange={(e) => {
                  if (e.target.files) void addBrowserFiles(e.target.files)
                  e.target.value = ""
                }}
              />
            </div>

            {jobs.length === 0 ? (
              <div className="rounded-xl border border-line bg-white px-4 py-10 text-center shadow-card">
                <p className="text-sm font-medium text-ink">待转换列表为空</p>
                <p className="mt-1 text-xs leading-5 text-muted">
                  拖入 TXT 或原生 EPUB，自动识别目录并支持手动微调与刮搜信息。
                </p>
              </div>
            ) : (
              <ul className="space-y-2">
                {jobs.map((job) => {
                  const isSel = selectedJob?.id === job.id
                  return (
                    <li key={job.id}>
                      <article
                        className={`rounded-xl border bg-white p-3 shadow-card ${
                          isSel ? "border-ink/40 ring-2 ring-ink/10" : "border-line hover:border-ink/20"
                        }`}
                      >
                        <button type="button" className="flex w-full items-start gap-2.5 text-left" onClick={() => setSelected(job.id)}>
                          <span className="book-face grid h-14 w-10 shrink-0 place-items-center overflow-hidden rounded">
                            {job.cover?.dataBase64 ? (
                              <img src={`data:image/jpeg;base64,${job.cover.dataBase64}`} alt="" className="h-full w-full object-cover" />
                            ) : (
                              <span className="font-mono text-[10px] font-medium text-muted">{fileKind(job.name)}</span>
                            )}
                          </span>
                          <span className="min-w-0 flex-1">
                            <span className="flex items-center gap-1.5">
                              <span className="rounded bg-paper-deep px-1.5 py-0.5 font-mono text-[10px] font-medium text-ink">
                                {fileKind(job.name)}
                              </span>
                              <span className="truncate text-sm font-medium text-ink">{job.title || job.name}</span>
                            </span>
                            <span className="mt-0.5 block truncate text-[11px] text-muted">
                              {job.author || "作者未填"}
                              {job.series ? ` · ${job.series}` : ""}
                            </span>
                          </span>
                          <Status job={job} />
                        </button>

                        <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
                          <select
                            aria-label="编码"
                            className="h-7 rounded-md border border-line bg-paper px-2 text-[11px] text-ink"
                            value={job.encoding}
                            onChange={(e) => {
                              const encoding = e.target.value
                              update(job.id, { encoding })
                              void probe({ ...job, encoding }, encoding)
                            }}
                          >
                            {(encodingField?.options ?? [{ value: "auto", label: "自动检测" }]).map((opt) => (
                              <option key={opt.value} value={opt.value}>
                                {opt.label}
                              </option>
                            ))}
                          </select>
                          {!job.scraped ? (
                            <button
                              type="button"
                              className="inline-flex h-7 items-center gap-1 rounded-md border border-line bg-white px-2 text-[11px] font-medium text-ink hover:bg-paper"
                              onClick={() => {
                                setSelected(job.id)
                                setScrapeOpen(true)
                              }}
                            >
                              <Search className="h-3 w-3 text-muted" />
                              刮搜
                            </button>
                          ) : null}
                          <span className="text-[11px] text-muted">
                            {job.probe ? `${labelEncoding(job.probe.encoding)} · ${job.probe.chapterCount} 章` : ""}
                          </span>
                          <button
                            type="button"
                            className="ml-auto rounded p-1 text-muted hover:bg-danger-soft hover:text-danger"
                            aria-label="移除这本书"
                            onClick={() => {
                              setJobs((prev) => prev.filter((item) => item.id !== job.id))
                              if (selected === job.id) setSelected("")
                            }}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </button>
                        </div>

                        {job.error ? <p className="mt-2 text-xs text-danger">{job.error}</p> : null}
                        {job.probe?.warning ? <p className="mt-2 text-[11px] text-warn">{job.probe.warning}</p> : null}
                        {job.status === "done" ? (
                          <p className="mt-2 break-all text-xs leading-5 text-ink">
                            已生成 {job.outputName}
                            {job.outputBytes ? ` · ${formatBytes(job.outputBytes)}` : ""}
                            {job.outputPath && isDesktop() ? (
                              <button
                                type="button"
                                className="ml-2 text-accent hover:underline"
                                onClick={() => void window.go?.main.App.RevealPath(job.outputPath || "")}
                              >
                                打开位置
                              </button>
                            ) : null}
                          </p>
                        ) : null}
                      </article>
                    </li>
                  )
                })}
              </ul>
            )}
          </section>

          <section className="space-y-4">
            <MetadataCard
              job={selectedJob}
              globalCover={cover}
              scrapeOpen={scrapeOpen}
              onScrape={setScrapeOpen}
              onChange={(patch) => selectedJob && update(selectedJob.id, patch)}
              onBrowse={() => selectedJob && void pickAttachment("cover", selectedJob.id)}
              onRemove={() => selectedJob && update(selectedJob.id, { cover: undefined, coverMode: "auto" })}
              onGenerate={() => selectedJob && update(selectedJob.id, { cover: undefined, coverMode: "generate" })}
            />

            {selectedJob?.probe ? (
              <ChapterPreview
                key={selectedJob.id}
                entries={selectedJob.probe.entries}
                volumeCount={selectedJob.probe.volumeCount}
                chapterCount={selectedJob.probe.chapterCount}
                runeCount={selectedJob.probe.runeCount}
                edits={selectedJob.tocEdits}
                onEdit={editSelectedToc}
                onResetEdits={() => update(selectedJob.id, { tocEdits: {} })}
              />
            ) : (
              <div className="rounded-xl border border-line bg-white p-4 shadow-card">
                <h2 className="text-sm font-semibold text-ink">目录预览与微调</h2>
                <p className="mt-2 text-xs text-muted">添加书籍后，这里会实时展示识别出的目录树、字数、异常标红及单条编辑。</p>
              </div>
            )}

            <div className="rounded-xl border border-line bg-white p-4 shadow-card">
              <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                <div className="flex items-center gap-2">
                  <SlidersHorizontal className="h-4 w-4 text-muted" />
                  <h2 className="text-sm font-semibold text-ink">目录排版与识别设置</h2>
                </div>
                <div className="flex items-center gap-3">
                  <button
                    type="button"
                    className="inline-flex items-center gap-1 text-xs text-muted hover:text-ink"
                    onClick={() => {
                      for (const job of jobsRef.current) void probe(job, job.encoding)
                    }}
                  >
                    <RotateCcw className="h-3.5 w-3.5" />
                    重新识别
                  </button>
                  <button
                    type="button"
                    className="inline-flex items-center gap-1 rounded-md border border-line bg-paper px-2.5 py-1 text-xs font-medium text-ink hover:bg-paper-deep"
                    onClick={() => setAdvancedOpen((v) => !v)}
                  >
                    高级正则与版式
                    <ChevronDown className={`h-3.5 w-3.5 transition-transform ${advancedOpen ? "rotate-180" : ""}`} />
                  </button>
                </div>
              </div>

              <div className="mb-3 space-y-1.5">
                <span className="text-xs font-medium text-muted">章节识别预设</span>
                <div className="flex flex-wrap gap-1.5">
                  {regexPresets.map((preset) => {
                    const active =
                      options.chapterPattern === preset.chapterPattern &&
                      options.volumePattern === preset.volumePattern
                    return (
                      <button
                        key={preset.id}
                        type="button"
                        onClick={() =>
                          setOptions((prev) => ({
                            ...prev,
                            chapterPattern: preset.chapterPattern,
                            volumePattern: preset.volumePattern,
                          }))
                        }
                        className={`rounded-full border px-2.5 py-1 text-xs transition ${
                          active
                            ? "border-ink bg-ink text-white"
                            : "border-line bg-paper text-ink hover:border-ink/30 hover:bg-paper-deep"
                        }`}
                      >
                        {preset.label}
                      </button>
                    )
                  })}
                </div>
              </div>

              <SchemaForm
                fields={quickFields}
                values={options}
                onChange={(key, value) => setOptions((prev) => ({ ...prev, [key]: value }))}
              />

              {advancedOpen ? (
                <div className="mt-4 space-y-4 border-t border-line pt-4">
                  <SchemaForm
                    fields={advancedFields}
                    values={options}
                    onChange={(key, value) => setOptions((prev) => ({ ...prev, [key]: value }))}
                  />
                  <div className="grid gap-2 sm:grid-cols-2">
                    <Button type="button" size="sm" variant="outline" onClick={() => void pickAttachment("font", "global")}>
                      <Type className="h-3.5 w-3.5" />
                      {font ? font.name : "嵌入自定义字体"}
                    </Button>
                    <Button type="button" size="sm" variant="outline" onClick={() => void pickAttachment("cover", "global")}>
                      <ImagePlus className="h-3.5 w-3.5" />
                      {cover ? cover.name : "设置统一封面"}
                    </Button>
                  </div>
                  <p className="text-[11px] leading-5 text-muted">
                    默认样式对齐 Calibre：行距与页边距交由阅读器原生调节（填 0 不写死），段距 1em、首行缩进 2em，不锁死 px 字号，也不加 !important。
                  </p>
                </div>
              ) : null}

              <input
                ref={coverRef}
                type="file"
                accept="image/jpeg,image/png,.jpg,.jpeg,.png"
                className="hidden"
                onChange={(e) => {
                  const file = e.target.files?.[0]
                  e.target.value = ""
                  if (!file) return
                  const target = coverTarget.current
                  void fileToBase64(file).then((dataBase64) => {
                    const item = { dataBase64, name: file.name }
                    if (target === "global") setCover(item)
                    else update(target, { cover: item, coverMode: "file" })
                  })
                }}
              />
              <input
                ref={fontRef}
                type="file"
                accept=".ttf,.otf"
                className="hidden"
                onChange={(e) => {
                  const file = e.target.files?.[0]
                  e.target.value = ""
                  if (!file) return
                  void fileToBase64(file).then((dataBase64) => setFont({ dataBase64, name: file.name }))
                }}
              />
            </div>
          </section>
        </div>
      </div>

      <div className="sticky bottom-0 z-10 flex flex-wrap items-center gap-3 border-t border-line bg-white/95 px-4 py-3 backdrop-blur md:px-6">
        {isDesktop() ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              void window.go!.main.App.PickDirectory().then((dir) => {
                if (dir) setOutputDir(dir)
              })
            }}
          >
            <FolderOpen className="h-3.5 w-3.5 text-muted" />
            <span className="max-w-[16rem] truncate">{outputDir || "输出到源文件同目录（自动防覆盖）"}</span>
          </Button>
        ) : (
          <span className="text-xs text-muted">浏览器预览模式会在后台生成并保留历史记录。</span>
        )}
        <div className="ml-auto flex items-center gap-3">
          <span className="hidden font-mono text-xs text-muted sm:inline">
            目标：{outputFormat === "both" ? "EPUB + AZW3" : outputFormat.toUpperCase()}
          </span>
          <Button type="button" size="lg" disabled={busy || readyCount === 0} onClick={() => void convertAll()}>
            {busy ? "正在提交…" : `开始转换${readyCount > 1 ? ` (${readyCount})` : ""}`}
          </Button>
        </div>
      </div>
    </div>
  )
}

function FormatSegmented({
  value,
  onChange,
}: {
  value: OutputFormat
  onChange: (v: OutputFormat) => void
}) {
  return (
    <div className="inline-flex rounded-lg border border-line bg-paper-deep/70 p-0.5" role="radiogroup" aria-label="输出格式">
      {outputFormatChoices.map((item) => {
        const active = value === item.value
        return (
          <button
            key={item.value}
            type="button"
            role="radio"
            aria-checked={active}
            title={item.hint}
            onClick={() => onChange(item.value)}
            className={`rounded-md px-3 py-1.5 text-xs font-medium transition ${
              active ? "bg-white text-ink shadow-sm" : "text-muted hover:text-ink"
            }`}
          >
            {item.label}
          </button>
        )
      })}
    </div>
  )
}

function MetadataCard({
  job,
  globalCover,
  scrapeOpen,
  onScrape,
  onChange,
  onBrowse,
  onRemove,
  onGenerate,
}: {
  job?: Job
  globalCover: Attachment | null
  scrapeOpen: boolean
  onScrape: (open: boolean) => void
  onChange: (patch: Partial<Job>) => void
  onBrowse: () => void
  onRemove: () => void
  onGenerate: () => void
}) {
  const [moreMeta, setMoreMeta] = useState(false)
  return (
    <div className="rounded-xl border border-line bg-white p-4 shadow-card">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold text-ink">书籍信息</h2>
          <p className="mt-0.5 text-[11px] text-muted">支持直接编辑，或从豆瓣 / 微信读书等书源一键刮搜补全。</p>
        </div>
        <div className="flex items-center gap-2">
          {job ? (
            <button
              type="button"
              className="inline-flex items-center gap-1 rounded-md border border-line bg-paper px-2.5 py-1 text-xs text-muted hover:text-ink"
              onClick={() => setMoreMeta((v) => !v)}
            >
              出版与丛书
              <ChevronDown className={`h-3.5 w-3.5 transition-transform ${moreMeta ? "rotate-180" : ""}`} />
            </button>
          ) : null}
          <Button type="button" size="sm" disabled={!job} onClick={() => onScrape(true)}>
            <Search className="h-3.5 w-3.5" />
            刮搜元数据
          </Button>
        </div>
      </div>
      {!job ? (
        <p className="mt-3 text-xs text-muted">在左侧选择一本书后，可编辑书名、作者、封面和简介。</p>
      ) : (
        <div className="mt-3 grid items-start gap-4 lg:grid-cols-[10rem_minmax(0,1fr)]">
          <div className="mx-auto w-full max-w-[10rem] lg:mx-0">
            <CoverBox src={coverPreview(job, globalCover)} title={job.title} author={job.author} />
            <p className="mt-1.5 truncate text-center text-[11px] text-muted" title={coverCaption(job, globalCover)}>
              {coverCaption(job, globalCover)}
            </p>
            <div className="mt-2 grid grid-cols-3 gap-1">
              <Button type="button" size="sm" variant="outline" className="h-7 px-1 text-[11px]" onClick={onBrowse}>
                选图
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="h-7 px-1 text-[11px]"
                onClick={onRemove}
                disabled={job.coverMode === "auto" && !job.cover}
              >
                清除
              </Button>
              <Button
                type="button"
                size="sm"
                className="h-7 px-1 text-[11px]"
                variant={job.coverMode === "generate" ? "ink" : "outline"}
                onClick={onGenerate}
              >
                生成
              </Button>
            </div>
          </div>
          <div className="grid min-w-0 content-start gap-3">
            <div className="grid gap-2.5 sm:grid-cols-2">
              <Field label="书名">
                <Input aria-label="书名" value={job.title} onChange={(e) => onChange({ title: e.target.value, titleTouched: true })} />
              </Field>
              <Field label="作者">
                <Input
                  aria-label="作者"
                  value={job.author}
                  placeholder="佚名"
                  onChange={(e) => onChange({ author: e.target.value, authorTouched: true })}
                />
              </Field>
            </div>
            <Field label="简介">
              <textarea
                aria-label="简介"
                className="min-h-16 w-full rounded-lg border border-line bg-white px-3 py-2 text-sm text-ink focus-visible:border-ink/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ink/10"
                value={job.intro}
                onChange={(e) => onChange({ intro: e.target.value })}
              />
            </Field>
            {moreMeta ? (
              <div className="space-y-3 border-t border-line pt-3">
                <div className="grid gap-2.5 sm:grid-cols-[minmax(0,1fr)_5.5rem]">
                  <Field label="丛书">
                    <Input aria-label="丛书" value={job.series} onChange={(e) => onChange({ series: e.target.value })} />
                  </Field>
                  <Field label="序号">
                    <Input
                      aria-label="丛书序号"
                      inputMode="decimal"
                      placeholder="1"
                      value={job.seriesIndex}
                      onChange={(e) => onChange({ seriesIndex: e.target.value })}
                    />
                  </Field>
                </div>
                <div className="grid gap-2.5 sm:grid-cols-2">
                  <Field label="标签">
                    <Input aria-label="标签" placeholder="玄幻、完结" value={job.tags} onChange={(e) => onChange({ tags: e.target.value })} />
                  </Field>
                  <Field label="评分">
                    <div className="flex h-9 items-center gap-1 rounded-lg border border-line bg-white px-2.5">
                      {[1, 2, 3, 4, 5].map((star) => (
                        <button
                          key={star}
                          type="button"
                          aria-label={`${star} 星`}
                          className="text-amber-500"
                          onClick={() => onChange({ rating: job.rating === star ? 0 : star })}
                        >
                          <Star className="h-4 w-4" fill={star <= job.rating ? "currentColor" : "none"} />
                        </button>
                      ))}
                      <span className="ml-2 text-xs text-muted">{job.rating ? `${job.rating} 星` : "未评分"}</span>
                    </div>
                  </Field>
                </div>
                <div className="grid gap-2.5 sm:grid-cols-2">
                  <Field label="标识符 / ISBN">
                    <Input
                      aria-label="标识符"
                      placeholder="ISBN 978-7-..."
                      value={job.identifier}
                      onChange={(e) => onChange({ identifier: e.target.value })}
                    />
                  </Field>
                  <Field label="出版方">
                    <Input aria-label="出版方" value={job.publisher} onChange={(e) => onChange({ publisher: e.target.value })} />
                  </Field>
                </div>
                <div className="grid gap-2.5 sm:grid-cols-2">
                  <Field label="出版日期">
                    <Input aria-label="出版日期" placeholder="2026 或 2026-11-08" value={job.published} onChange={(e) => onChange({ published: e.target.value })} />
                  </Field>
                  <Field label="语言">
                    <select
                      aria-label="语言"
                      className="h-9 w-full rounded-lg border border-line bg-white px-3 text-sm text-ink"
                      value={job.language}
                      onChange={(e) => onChange({ language: e.target.value })}
                    >
                      {languages.map((item) => (
                        <option key={item.value} value={item.value}>
                          {item.label}
                        </option>
                      ))}
                    </select>
                  </Field>
                </div>
              </div>
            ) : null}
          </div>
        </div>
      )}
      {job && scrapeOpen ? (
        <ScrapeDialog
          title={job.title}
          author={job.author}
          current={{
            title: job.title,
            author: job.author,
            intro: job.intro,
            series: job.series,
            publisher: job.publisher,
            published: job.published,
            identifier: job.identifier,
            tags: job.tags,
            rating: job.rating,
          }}
          onClose={() => onScrape(false)}
          onApply={(patch) => onChange({ ...(patch as Partial<Job>), scraped: true })}
        />
      ) : null}
    </div>
  )
}

function CoverBox({ src, title, author }: { src: string | null; title: string; author: string }) {
  const [size, setSize] = useState("")
  useEffect(() => {
    setSize("")
  }, [src])
  return (
    <div className="book-face relative aspect-[3/4] overflow-hidden rounded-lg">
      {src ? (
        <>
          <img
            src={src}
            alt=""
            className="h-full w-full bg-zinc-900 object-contain"
            onLoad={(e) => setSize(`${e.currentTarget.naturalWidth}×${e.currentTarget.naturalHeight}`)}
          />
          {size ? (
            <span className="absolute bottom-1.5 right-1.5 rounded bg-black/70 px-1.5 py-0.5 font-mono text-[10px] text-white">
              {size}
            </span>
          ) : null}
        </>
      ) : (
        <div className="flex h-full flex-col items-center px-3 pb-4 pt-6 text-center">
          <div className="text-[10px] font-medium tracking-[0.25em] text-muted">MOCE</div>
          <div className="mt-2 h-px w-6 bg-line" />
          <div className="mt-4 line-clamp-4 text-sm font-semibold leading-snug text-ink">{title || "未命名"}</div>
          <div className="mt-auto truncate text-[11px] text-muted">{author || "佚名"}</div>
        </div>
      )}
    </div>
  )
}

async function readLocalFile(path: string) {
  return window.go!.main.App.ReadFileBase64(path)
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block space-y-1">
      <span className="text-xs font-medium text-muted">{label}</span>
      {children}
    </label>
  )
}

function coverCaption(job: Job, globalCover: Attachment | null) {
  if (job.coverMode === "generate") return "自动生成文字封面"
  const chosen = chosenCover(job, globalCover)
  if (job.cover) return job.cover.name
  if (chosen) return `统一封面 · ${chosen.name}`
  return "默认生成文字封面"
}

function chosenCover(job: Job, globalCover: Attachment | null): Attachment | undefined {
  if (job.coverMode === "generate") return undefined
  if (job.coverMode === "file") return job.cover
  return job.cover ?? globalCover ?? undefined
}

function coverPreview(job: Job, globalCover: Attachment | null) {
  const chosen = chosenCover(job, globalCover)
  if (!chosen?.dataBase64) return null
  const mime = chosen.name.toLowerCase().endsWith(".png") ? "image/png" : "image/jpeg"
  return `data:${mime};base64,${chosen.dataBase64}`
}

function Status({ job }: { job: Job }) {
  if (job.status === "done") return <Check className="h-4 w-4 text-emerald-600" />
  if (job.status === "error") return <AlertCircle className="h-4 w-4 text-danger" />
  const text = job.status === "probing" ? "识别中…" : job.status === "converting" ? "转换中" : job.probe ? `${job.probe.chapterCount} 章` : ""
  return <span className="shrink-0 font-mono text-[11px] text-muted">{text}</span>
}

function defaults(fields: ToolField[]) {
  const out: Record<string, unknown> = {}
  for (const field of fields) out[field.key] = field.default
  return out
}

function fileKind(name: string) {
  const ext = name.split(".").pop()?.toLowerCase()
  if (ext === "epub" || ext === "mobi" || ext === "azw3") return ext.toUpperCase()
  return "TXT"
}

function labelEncoding(name: string) {
  switch (name) {
    case "utf-8":
      return "UTF-8"
    case "gb18030":
      return "GB18030"
    case "big5":
      return "Big5"
    case "utf-16le":
      return "UTF-16 LE"
    case "utf-16be":
      return "UTF-16 BE"
    default:
      return name
  }
}
