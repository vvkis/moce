import { useState } from "react"
import { SchemaForm } from "@/components/SchemaForm"
import { Button } from "@/components/ui/button"
import { callTool } from "@/lib/api"
import type { ToolMeta } from "@/lib/types"
import { errorMessage } from "@/lib/utils"

export function GenericPanel({ meta }: { meta: ToolMeta }) {
  const [values, setValues] = useState<Record<string, unknown>>(() => defaults(meta))
  const [result, setResult] = useState("")
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)

  return (
    <section className="mx-auto max-w-3xl space-y-5 p-5 md:p-8">
      <header>
        <h1 className="font-serif text-3xl">{meta.name}</h1>
        <p className="mt-2 text-sm leading-6 text-muted">{meta.description}</p>
      </header>
      <div className="rounded-lg border border-line bg-paper-card p-4 shadow-card">
        <SchemaForm
          fields={meta.schema}
          values={values}
          onChange={(key, value) => setValues((prev) => ({ ...prev, [key]: value }))}
        />
      </div>
      {error ? <p className="text-sm text-cinnabar">{error}</p> : null}
      <Button
        disabled={busy}
        onClick={() => {
          setBusy(true)
          setError("")
          callTool("" + meta.id, "run", values)
            .then((out) => setResult(JSON.stringify(out, null, 2)))
            .catch((err) => setError(errorMessage(err)))
            .finally(() => setBusy(false))
        }}
      >
        {busy ? "正在运行" : "运行"}
      </Button>
      {result ? (
        <pre className="overflow-auto rounded-md bg-ink p-4 text-xs leading-5 text-paper-card">{result}</pre>
      ) : null}
    </section>
  )
}

function defaults(meta: ToolMeta) {
  const out: Record<string, unknown> = {}
  for (const field of meta.schema) out[field.key] = field.default
  return out
}
