/// <reference types="vite/client" />

declare module "*.txt?raw" {
  const src: string
  export default src
}

type DesktopApp = {
  ListTools: () => Promise<import("./lib/types").ToolMeta[]>
  CallTool: (id: string, action: string, payload: string) => Promise<string>
  CollectTxtPaths: (paths: string[]) => Promise<string[]>
  RevealPath: (path: string) => Promise<void>
  OpenPath: (path: string) => Promise<void>
  LoadSettings: () => Promise<{ outputDir: string; accountMode?: string }>
  SaveSettings: (settings: { outputDir: string; accountMode?: string }) => Promise<void>
  PickTxtFiles: () => Promise<string[] | null>
  PickCover: () => Promise<string>
  ReadFileBase64: (path: string) => Promise<string>
  PickFont: () => Promise<string>
  PickDirectory: () => Promise<string>
}

interface Window {
  go?: { main: { App: DesktopApp } }
  runtime?: {
    OnFileDrop: (cb: (x: number, y: number, paths: string[]) => void, useDropTarget: boolean) => void
    OnFileDropOff: () => void
  }
}
