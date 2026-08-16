#!/usr/bin/env node
// WeKnora stdio plugin: JSON-RPC 2.0, one message per line. Logs go to stderr.
const readline = require("node:readline");

const rl = readline.createInterface({ input: process.stdin });
rl.on("line", (line) => {
  let msg;
  try {
    msg = JSON.parse(line);
  } catch {
    return;
  }
  if (msg.method === "shutdown") {
    process.exit(0);
  }
  if (msg.method !== "websearch.search") {
    write({
      jsonrpc: "2.0",
      id: msg.id,
      error: { code: -32601, message: "method not found" },
    });
    return;
  }
  const q = String((msg.params && msg.params.query) || "").trim();
  write({
    jsonrpc: "2.0",
    id: msg.id,
    result: {
      results: q
        ? [
            {
              title: "node-echo",
              url: "https://weknora.local/plugin/node-echo",
              snippet: q,
              content: q,
              source: "node-echo",
            },
          ]
        : [],
    },
  });
});

function write(obj) {
  process.stdout.write(JSON.stringify(obj) + "\n");
}
