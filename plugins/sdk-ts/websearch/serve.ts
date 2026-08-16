import { createInterface } from "node:readline"
import { writeSync } from "node:fs"
import type { SearchRequest, SearchResponse } from "./protocol.ts"

export type Handlers = {
  search: (req: SearchRequest) => SearchResponse | Promise<SearchResponse>
}

type JsonRpcRequest = {
  jsonrpc?: string
  id?: number | string
  method?: string
  params?: SearchRequest
}

/**
 * Speak the WeKnora plugin ABI on stdin/stdout.
 * Do not open a port. Logs must go to stderr.
 */
export function serve(handlers: Handlers): void {
  const rl = createInterface({ input: process.stdin })
  rl.on("line", (line) => {
    void handleLine(line, handlers)
  })
}

async function handleLine(line: string, handlers: Handlers): Promise<void> {
  const trimmed = line.trim()
  if (!trimmed) {
    return
  }
  let msg: JsonRpcRequest
  try {
    msg = JSON.parse(trimmed) as JsonRpcRequest
  } catch {
    return
  }
  if (msg.method === "shutdown") {
    process.exit(0)
  }
  if (msg.method !== "websearch.search") {
    write({
      jsonrpc: "2.0",
      id: msg.id,
      error: { code: -32601, message: `method not found: ${msg.method}` },
    })
    return
  }
  try {
    const result = await handlers.search(msg.params || ({} as SearchRequest))
    write({ jsonrpc: "2.0", id: msg.id, result })
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    write({ jsonrpc: "2.0", id: msg.id, error: { code: -32000, message } })
  }
}

function write(obj: unknown): void {
  writeSync(1, JSON.stringify(obj) + "\n")
}
