import { createServer, type IncomingMessage, type ServerResponse } from "node:http"
import type { SearchRequest, SearchResponse } from "./protocol.ts"

const port = Number(process.env.PORT || 9101)

function readBody(req: IncomingMessage): Promise<string> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = []
    req.on("data", (c) => chunks.push(c))
    req.on("end", () => resolve(Buffer.concat(chunks).toString("utf8")))
    req.on("error", reject)
  })
}

const server = createServer(async (req: IncomingMessage, res: ServerResponse) => {
  if (req.method !== "POST" || req.url !== "/search") {
    res.writeHead(404)
    res.end()
    return
  }
  const body = JSON.parse(await readBody(req)) as SearchRequest
  const q = (body.query || "").trim()
  const out: SearchResponse = {
    results: q
      ? [
          {
            title: "ts-echo",
            url: "https://weknora.local/plugin/ts-echo",
            snippet: q,
            content: q,
            source: "ts-echo",
          },
        ]
      : [],
  }
  res.writeHead(200, { "content-type": "application/json" })
  res.end(JSON.stringify(out))
})

server.listen(port, "127.0.0.1", () => {
  console.log(`weknora ts websearch sidecar on http://127.0.0.1:${port}/search`)
})
