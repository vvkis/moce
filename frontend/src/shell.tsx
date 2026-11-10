import { createContext } from "react"

export const ShellContext = createContext({
  openProgress: () => {},
  watchBatch: (_batch: string) => {},
})
