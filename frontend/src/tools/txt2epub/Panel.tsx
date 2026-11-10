import { useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react"
import sampleText from "../../../../samples/示例小说.txt?raw"
import { SchemaForm } from "@/components/SchemaForm"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { callTool } from "@/lib/api"
import type { ProbeResult, ToolField, ToolMeta } from "@/lib/types"
import {
  baseName,
  errorMessage,
  fileToBase64,
  formatBytes,
  isDesktop,
  utf8ToBase64,
} from "@/lib/utils"
import { AlertCircle, Check, FilePlus2, FolderOpen, ImagePlus, RotateCcw, Search, Star, Trash2, Type } from "lucide-react"
import { ChapterPreview } from "./ChapterPreview"
import { ScrapeDialog } from "./ScrapeDialog"
import { ShellContext } from "@/shell"

type CoverMode = "auto" | "file" | "generate"

const languages = [
  { value: "zh", label: "中文" },
  { value: "zh-Hant", label: "繁体中文" },
  { value: "en", label: "英文" },
  { value: "ja", label: "日文" },
  { value: "ko", label: "韩文" },
]

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

export function Txt2EpubPanel({ meta }: { meta: ToolMeta }) {
  const shell = useContext(ShellContext)
  const fileFields = meta.schema.filter((field) => field.scope === "file")
  const bookFields = meta.schema.filter((field) => field.scope !== "file")
  const encodingField = fileFields.find((field) => field.key === "encoding")
  const [options, setOptions] = useState<Record<string, unknown>>(() => defaults(bookFields))
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

  function update(id: string, patch: Partial<Job> | ((job: Job) => Job)) {
    setJobs((prev) =>
      prev.map((job) => {
        if (job.id !== id) return job
        return typeof patch === "function" ? patch(job) : { ...job, ...patch }
      }),
    )
  }

  async function probe(job: Job, encoding = job.encoding) {
    update(job.id, { status: "probing", error: undefined, encoding })
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
    // The drop handler reads the latest callbacks through refs and state setters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      for (const job of jobsRef.current) {
        if (job.path || job.dataBase64) void probe(job, job.encoding)
      }
    }, 350)
    return () => window.clearTimeout(timer)
    // Re-probe only when chapter rules change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [detectKey])

  const selectedJob = jobs.find((job) => job.id === selected) ?? jobs[0]
  const readyCount = jobs.filter((job) => job.probe).length
  const chapterTotal = jobs.reduce((sum, job) => sum + (job.probe?.chapterCount ?? 0), 0)

  const summary = useMemo(() => {
    if (jobs.length === 0) return "还没有文件"
    return `${jobs.length} 本 · ${chapterTotal} 章`
  }, [jobs.length, chapterTotal])

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
      <div className="flex-1 space-y-4 p-3 md:p-5">
        <header className="flex flex-wrap items-end justify-between gap-3 border-b border-line/80 pb-3">
          <div className="min-w-0">
            <p className="text-[11px] tracking-[0.22em] text-cinnabar">重新排版</p>
            <h1 className="mt-0.5 font-serif text-2xl tracking-wide">{meta.name}</h1>
            <p className="mt-1 max-w-2xl text-sm leading-5 text-muted">
              把中文 TXT 做成给 KOReader 用的 EPUB。编码、章节和封面都在本机处理。
              {isDesktop() ? "" : " 浏览器预览会下载文件。"}
            </p>
          </div>
          <div className="rounded-full border border-line bg-paper-card px-3 py-1 text-xs text-muted">{summary}</div>
        </header>

        {banner ? (
          <p className="rounded-md border border-cinnabar/30 bg-cinnabar/5 px-3 py-2 text-sm text-cinnabar">{banner}</p>
        ) : null}

        <div className="grid items-start gap-4 xl:grid-cols-[minmax(16rem,22rem)_minmax(0,1fr)]">
          <section className="space-y-3">
            <div
              className="rounded-xl border border-dashed border-ink/25 bg-paper-card/70 px-4 py-4 shadow-card"
              onDragOver={(e) => e.preventDefault()}
              onDrop={(e) => {
                e.preventDefault()
                if (!isDesktop() && e.dataTransfer.files.length) void addBrowserFiles(e.dataTransfer.files)
              }}
            >
              <div className="flex items-start gap-3">
                <FilePlus2 className="mt-0.5 h-5 w-5 shrink-0 text-cinnabar" />
                <div className="min-w-0 text-left">
                  <p className="font-serif text-base">拖入 TXT、EPUB、MOBI 或 AZW3</p>
                  <p className="mt-0.5 text-xs leading-5 text-muted">原文件不会被改写。电子书会先抽出正文再重新排版。</p>
                </div>
              </div>
              <div className="mt-3 flex flex-wrap gap-2">
                <Button
                  type="button"
                  variant="ink"
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
                  variant="outline"
                  onClick={() =>
                    addJob({
                      name: "示例小说.txt",
                      dataBase64: utf8ToBase64(sampleText),
                    })
                  }
                >
                  用示例试一次
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
              <div className="rounded-xl border border-dashed border-line bg-paper-card/70 px-4 py-8 text-center">
                <div className="mx-auto mb-3 h-20 w-14 rounded-sm border border-cinnabar/40 bg-gradient-to-b from-paper-card to-paper shadow-card">
                  <div className="mx-auto mt-3 h-px w-6 bg-cinnabar/50" />
                  <div className="mt-3 font-serif text-xs tracking-[0.3em] text-cinnabar">墨册</div>
                </div>
                <p className="font-serif text-lg">书架还是空的</p>
                <p className="mt-1 text-sm text-muted">拖进一本书，或用示例试一次。选中之后可以刮搜书名、作者和封面。</p>
              </div>
            ) : (
              <ul className="space-y-2">
                {jobs.map((job) => (
                  <li key={job.id}>
                    <article
                      className={`rounded-xl border bg-paper-card p-3 shadow-card ${
                        selectedJob?.id === job.id ? "border-cinnabar/70 ring-1 ring-cinnabar/20" : "border-line"
                      }`}
                    >
                      <button type="button" className="flex w-full items-start gap-3 text-left" onClick={() => setSelected(job.id)}>
                        <span className="book-face grid h-16 w-12 shrink-0 place-items-center overflow-hidden rounded-sm">
                          {job.cover?.dataBase64 ? (
                            <img src={`data:image/jpeg;base64,${job.cover.dataBase64}`} alt="" className="h-full w-full object-cover" />
                          ) : (
                            <span className="font-serif text-[10px] text-cinnabar">{fileKind(job.name)}</span>
                          )}
                        </span>
                        <span className="min-w-0 flex-1">
                          <span className="flex items-center gap-2">
                            <span className="rounded-sm border border-cinnabar/30 px-1 text-[10px] text-cinnabar">{fileKind(job.name)}</span>
                            <span className="truncate font-serif text-base">{job.title || job.name}</span>
                          </span>
                          <span className="mt-0.5 block truncate text-xs text-muted">{job.name}</span>
                        </span>
                        <Status job={job} />
                      </button>
                      <p className="mt-2 text-xs text-muted">{job.author || "作者未填"}{job.series ? ` · ${job.series}` : ""}</p>
                      {!job.scraped ? (
                        <button
                          type="button"
                          className="mt-2 inline-flex items-center gap-1 rounded-md bg-cinnabar/10 px-2 py-1 text-xs text-cinnabar hover:bg-cinnabar/15"
                          onClick={() => {
                            setSelected(job.id)
                            setScrapeOpen(true)
                          }}
                        >
                          <Search className="h-3.5 w-3.5" />
                          还没刮搜，点这里补书名和封面
                        </button>
                      ) : null}
                      <div className="mt-2 flex flex-wrap items-center gap-2">
                        <select
                          aria-label="编码"
                          className="h-8 rounded-md border border-line bg-paper px-2 text-xs"
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
                        <span className="text-xs text-muted">
                          {job.probe
                            ? `${labelEncoding(job.probe.encoding)}${job.probe.bom ? " · BOM" : ""} · ${job.probe.chapterCount} 章`
                            : job.status === "probing"
                              ? "正在识别"
                              : ""}
                        </span>
                        <button
                          type="button"
                          className="ml-auto text-muted hover:text-cinnabar"
                          aria-label="移除这本书"
                          onClick={() => {
                            setJobs((prev) => prev.filter((item) => item.id !== job.id))
                            if (selected === job.id) setSelected("")
                          }}
                        >
                          <Trash2 className="h-4 w-4" />
                        </button>
                      </div>
                      {job.error ? <p className="mt-2 text-xs text-cinnabar">{job.error}</p> : null}
                      {job.probe?.warning ? <p className="mt-2 text-xs text-muted">{job.probe.warning}</p> : null}
                      {job.status === "done" ? (
                        <p className="mt-2 break-all text-xs leading-5 text-ink">
                          已生成 {job.outputName}
                          {job.outputBytes ? ` · ${formatBytes(job.outputBytes)}` : ""}
                          {job.outputPath ? ` · ${job.outputPath}` : " · 已下载"}
                          {job.outputPath && isDesktop() ? (
                            <button
                              type="button"
                              className="ml-2 text-cinnabar hover:underline"
                              onClick={() => void window.go?.main.App.RevealPath(job.outputPath || "")}
                            >
                              打开位置
                            </button>
                          ) : null}
                        </p>
                      ) : null}
                    </article>
                  </li>
                ))}
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
            <div className="rounded-xl border border-line bg-paper-card p-4 shadow-card">
              <div className="mb-3 flex items-center justify-between gap-3">
                <h2 className="font-serif text-lg tracking-wide">排版和识别</h2>
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
              </div>
              <SchemaForm
                fields={bookFields}
                values={options}
                onChange={(key, value) => setOptions((prev) => ({ ...prev, [key]: value }))}
              />
              <div className="mt-4 grid gap-2 sm:grid-cols-2">
                <Button type="button" variant="outline" onClick={() => void pickAttachment("font", "global")}>
                  <Type className="h-4 w-4" />
                  {font ? font.name : "嵌入字体"}
                </Button>
                <Button type="button" variant="outline" onClick={() => void pickAttachment("cover", "global")}>
                  <ImagePlus className="h-4 w-4" />
                  {cover ? cover.name : "统一封面"}
                </Button>
              </div>
              <p className="mt-2 text-xs leading-5 text-muted">
                不选封面时，会用标题和作者画一张简单的封面。字号默认跟随阅读器。正文样式与 moon 重新排版相同：页边距 5%，行距 1.8，段间距 0.45em，首行缩进 2em，卷和章都是 1.5em 的标题。不用 px 字号，也不加 !important。
              </p>
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

            {selectedJob?.probe ? (
              <ChapterPreview
                key={`${selectedJob.id}:${selectedJob.probe.entries.length}:${selectedJob.probe.entries[0]?.title ?? ""}:${selectedJob.probe.entries.at(-1)?.title ?? ""}`}
                entries={selectedJob.probe.entries}
                volumeCount={selectedJob.probe.volumeCount}
                chapterCount={selectedJob.probe.chapterCount}
                runeCount={selectedJob.probe.runeCount}
              />
            ) : (
              <div className="rounded-xl border border-line bg-paper-card p-4 shadow-card">
                <h2 className="font-serif text-lg tracking-wide">章节预览</h2>
                <p className="mt-3 text-sm text-muted">添加文件后，这里列出识别到的章节。确认无误再转换。</p>
              </div>
            )}
          </section>
        </div>
      </div>

      <div className="sticky bottom-0 z-10 flex flex-wrap items-center gap-3 border-t border-line bg-[#f6efe4]/95 px-3 py-3 backdrop-blur md:px-5">
        {isDesktop() ? (
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              void window.go!.main.App.PickDirectory().then((dir) => {
                if (dir) setOutputDir(dir)
              })
            }}
          >
            <FolderOpen className="h-4 w-4" />
            {outputDir || "输出到源文件旁边"}
          </Button>
        ) : (
          <span className="text-xs text-muted">预览模式会把 EPUB 下载下来。桌面版可以选输出文件夹。</span>
        )}
        <Button type="button" className="ml-auto" size="lg" disabled={busy || readyCount === 0} onClick={() => void convertAll()}>
          {busy ? "正在转换" : "开始转换"}
        </Button>
      </div>
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
  return (
    <div className="rounded-xl border border-line bg-paper-card p-4 shadow-card">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="font-serif text-lg tracking-wide">书籍信息</h2>
        <Button type="button" size="lg" disabled={!job} onClick={() => onScrape(true)}>
          <Search className="h-5 w-5" />
          刮搜
        </Button>
      </div>
      <p className="mt-2 text-[11px] text-muted">创建日期自动写成今天。刮搜只改你勾选的书名、作者、简介和封面。</p>
      {!job ? (
        <p className="mt-3 text-sm text-muted">从左边点开一本书，再用上面的朱红按钮刮搜，或自己填书名和作者。</p>
      ) : (
        <div className="mt-3 grid items-start gap-4 lg:grid-cols-[11.5rem_minmax(0,1fr)]">
          <div className="mx-auto w-full max-w-[12rem] lg:mx-0">
            <CoverBox src={coverPreview(job, globalCover)} title={job.title} author={job.author} />
            <p className="mt-2 truncate text-center text-[11px] text-muted" title={coverCaption(job, globalCover)}>
              {coverCaption(job, globalCover)}
            </p>
            <div className="mt-2 grid grid-cols-3 gap-1">
              <Button type="button" size="sm" variant="outline" onClick={onBrowse}>
                浏览
              </Button>
              <Button type="button" size="sm" variant="outline" onClick={onRemove} disabled={job.coverMode === "auto" && !job.cover}>
                移除
              </Button>
              <Button type="button" size="sm" variant={job.coverMode === "generate" ? "ink" : "outline"} onClick={onGenerate}>
                生成
              </Button>
            </div>
          </div>
          <div className="grid min-w-0 content-start gap-4">
            <Group title="书与人">
              <div className="grid gap-2 sm:grid-cols-2">
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
                  className="min-h-16 w-full rounded-md border border-line bg-[#fffaf3] px-3 py-2 text-sm"
                  value={job.intro}
                  onChange={(e) => onChange({ intro: e.target.value })}
                />
              </Field>
            </Group>
            <Group title="丛书与评价">
              <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_5.5rem]">
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
              <Field label="评分">
                <div className="flex h-9 items-center gap-1 rounded-md border border-line bg-[#fffaf3] px-2">
                  {[1, 2, 3, 4, 5].map((star) => (
                    <button
                      key={star}
                      type="button"
                      aria-label={`${star} 星`}
                      className="text-cinnabar"
                      onClick={() => onChange({ rating: job.rating === star ? 0 : star })}
                    >
                      <Star className="h-4 w-4" fill={star <= job.rating ? "currentColor" : "none"} />
                    </button>
                  ))}
                  <span className="ml-2 text-xs text-muted">{job.rating ? `${job.rating} 星` : "未评分"}</span>
                </div>
              </Field>
              <Field label="标签">
                <Input aria-label="标签" placeholder="玄幻、完结" value={job.tags} onChange={(e) => onChange({ tags: e.target.value })} />
              </Field>
            </Group>
            <Group title="出版">
              <Field label="标识符">
                <Input
                  aria-label="标识符"
                  placeholder="ISBN 978-7-..."
                  value={job.identifier}
                  onChange={(e) => onChange({ identifier: e.target.value })}
                />
              </Field>
              <div className="grid gap-2 sm:grid-cols-3">
                <Field label="出版日期">
                  <Input aria-label="出版日期" placeholder="2012 或 2012-08-01" value={job.published} onChange={(e) => onChange({ published: e.target.value })} />
                </Field>
                <Field label="出版方">
                  <Input aria-label="出版方" value={job.publisher} onChange={(e) => onChange({ publisher: e.target.value })} />
                </Field>
                <Field label="语言">
                  <select
                    aria-label="语言"
                    className="h-9 w-full rounded-md border border-line bg-[#fffaf3] px-3 text-sm"
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
              <p className="text-[11px] leading-4 text-muted">封面可以本机选择、按书名生成，或用刮搜下载。</p>
            </Group>
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

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-2">
      <h3 className="text-[11px] tracking-[0.18em] text-cinnabar">{title}</h3>
      {children}
    </section>
  )
}

function CoverBox({ src, title, author }: { src: string | null; title: string; author: string }) {
  const [size, setSize] = useState("")
  useEffect(() => {
    setSize("")
  }, [src])
  return (
    <div className="book-face relative aspect-[3/4] overflow-hidden rounded-sm">
      {src ? (
        <>
          <img
            src={src}
            alt=""
            className="h-full w-full bg-[#1c1612] object-contain"
            onLoad={(e) => setSize(`${e.currentTarget.naturalWidth} × ${e.currentTarget.naturalHeight}`)}
          />
          {size ? (
            <span className="absolute bottom-2 right-2 rounded-sm bg-black/70 px-1.5 py-0.5 text-[10px] tracking-wide text-white">{size}</span>
          ) : null}
        </>
      ) : (
        <div className="flex h-full flex-col items-center px-4 pb-6 pt-7 text-center">
          <div className="text-[10px] tracking-[0.45em] text-cinnabar">墨册</div>
          <div className="mt-3 h-px w-8 bg-cinnabar/50" />
          <div className="mt-4 font-serif text-[1.35rem] leading-snug text-ink">{title || "未命名"}</div>
          <div className="mt-auto text-xs tracking-wide text-muted">{author || "佚名"}</div>
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
      <span className="text-xs text-muted">{label}</span>
      {children}
    </label>
  )
}

function coverCaption(job: Job, globalCover: Attachment | null) {
  if (job.coverMode === "generate") return "生成文字封面"
  const chosen = chosenCover(job, globalCover)
  if (job.cover) return job.cover.name
  if (chosen) return `统一封面 · ${chosen.name}`
  return "未选封面，将按书名生成"
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
  if (job.status === "done") return <Check className="h-4 w-4 text-cinnabar" />
  if (job.status === "error") return <AlertCircle className="h-4 w-4 text-cinnabar" />
  const text = job.status === "probing" ? "识别中" : job.status === "converting" ? "转换中" : job.probe ? `${job.probe.chapterCount} 章` : ""
  return <span className="shrink-0 text-xs text-muted">{text}</span>
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
