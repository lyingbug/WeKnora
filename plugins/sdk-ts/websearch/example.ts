import { serve } from "./serve.ts"

serve({
  search(req) {
    const q = (req.query || "").trim()
    return {
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
  },
})
