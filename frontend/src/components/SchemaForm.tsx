import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import type { ToolField } from "@/lib/types"
import { cn } from "@/lib/utils"

export function SchemaForm({
  fields,
  values,
  onChange,
}: {
  fields: ToolField[]
  values: Record<string, unknown>
  onChange: (key: string, value: unknown) => void
}) {
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      {fields.map((field) => (
        <FieldControl key={field.key} field={field} value={values[field.key]} onChange={onChange} />
      ))}
    </div>
  )
}

function FieldControl({
  field,
  value,
  onChange,
}: {
  field: ToolField
  value: unknown
  onChange: (key: string, value: unknown) => void
}) {
  if (field.type === "bool") {
    return (
      <label className="flex items-start gap-3 rounded-md border border-line bg-paper/60 px-3 py-2 sm:col-span-2">
        <Checkbox
          className="mt-0.5"
          checked={value === true}
          onCheckedChange={(checked) => onChange(field.key, checked === true)}
        />
        <span>
          <span className="block text-sm font-medium">{field.label}</span>
          {field.help ? <span className="mt-0.5 block text-xs leading-5 text-muted">{field.help}</span> : null}
        </span>
      </label>
    )
  }

  const wide = field.type === "textarea"
  return (
    <div className={cn("space-y-1.5", wide && "sm:col-span-2")}>
      <div className="flex items-center justify-between gap-3">
        <Label htmlFor={field.key}>{field.label}</Label>
        {field.default !== undefined && value !== field.default ? (
          <button
            type="button"
            className="text-xs text-cinnabar hover:underline"
            onClick={() => onChange(field.key, field.default)}
          >
            恢复默认
          </button>
        ) : null}
      </div>
      {field.type === "textarea" ? (
        <textarea
          id={field.key}
          value={typeof value === "string" ? value : ""}
          onChange={(e) => onChange(field.key, e.target.value)}
          className="min-h-16 w-full rounded-md border border-line bg-[#fffaf3] px-3 py-2 font-mono text-xs leading-5 text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cinnabar/30"
        />
      ) : field.type === "select" ? (
        <select
          id={field.key}
          value={typeof value === "string" ? value : ""}
          onChange={(e) => onChange(field.key, e.target.value)}
          className="h-9 w-full rounded-md border border-line bg-paper-card px-3 text-sm"
        >
          {field.options?.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      ) : field.type === "number" ? (
        <Input
          id={field.key}
          type="number"
          min={field.min}
          max={field.max}
          step={field.step}
          value={typeof value === "number" ? value : ""}
          onChange={(e) => onChange(field.key, e.target.value === "" ? 0 : Number(e.target.value))}
        />
      ) : (
        <Input
          id={field.key}
          value={typeof value === "string" ? value : ""}
          onChange={(e) => onChange(field.key, e.target.value)}
        />
      )}
      {field.help ? <p className="text-xs leading-5 text-muted">{field.help}</p> : null}
    </div>
  )
}
