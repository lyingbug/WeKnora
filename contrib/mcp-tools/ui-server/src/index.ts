#!/usr/bin/env node

// Set process title so MCP servers are distinguishable from user node processes.
// Without this, "killall node" (to stop a user's app) would also kill MCP servers.
process.title = "weknora-mcp-ui";

import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { stat } from "node:fs/promises";
import { join } from "node:path";
import { z } from "zod";

function ok(text: string) {
  return { content: [{ type: "text" as const, text }] };
}

function err(text: string) {
  return { isError: true, content: [{ type: "text" as const, text }] };
}

const server = new McpServer({
  name: "ui",
  version: "1.0.0",
});

// ---------------------------------------------------------------------------
// show_file + show_web_app — display content to the user
//
// These tools work by side effect: the frontend observes tool call results
// containing a `renderRequest` in `structuredContent` and renders accordingly.
// The server just validates inputs and acknowledges — it doesn't render anything.
// ---------------------------------------------------------------------------

function renderResult(req: Record<string, unknown>) {
  const output = { message: "successfully rendered", renderRequest: req };
  return {
    ...ok(JSON.stringify(output)),
    structuredContent: output,
  };
}

server.registerTool(
  "show_file",
  {
    title: "Show File",
    description:
      "Show a file to the user (image, video, audio, or any document). Always use this tool when you need to present a file to the user — do not describe it in text when you can show it directly.",
    inputSchema: {
      path: z
        .string()
        .describe("Absolute path to the file to show. Must be a file, not a directory."),
      alt: z
        .string()
        .optional()
        .describe("Alternative text or description for the content"),
    },
  },
  async ({ path, alt }) => {
    try {
      const stats = await stat(path);
      if (stats.isDirectory()) {
        return err(`Path "${path}" is a directory, not a file`);
      }
    } catch {
      return err(`File "${path}" does not exist`);
    }
    return renderResult({ path, alt });
  },
);

server.registerTool(
  "show_web_app",
  {
    title: "Show Web App",
    description:
      "Show a running web application to the user. The app must be a viewable page with HTML content — it will be displayed in an embedded preview. Supports both static sites (HTML/CSS/JS served by a file server) and dynamic apps (SSR, Node.js backend, etc.). Use this after starting a local server on port 3000. Users cannot access localhost directly — this tool is the only way to present a running app. Do not tell users to visit a URL; use this tool instead.",
    inputSchema: {
      url: z
        .string()
        .describe(
          "Local server root URL (must be http://localhost:3000/). Port 3000 is the sandbox reserved port for user preview. Only root URLs are allowed.",
        ),
      static: z
        .boolean()
        .describe(
          "true if this is a static site served by a simple file server (HTML/CSS/JS only). false if this is a dynamic app (SSR, Node.js server, backend logic, etc.).",
        ),
      sourceDir: z
        .string()
        .describe(
          "Absolute path to the app source directory. For static sites, this is the directory being served (must contain index.html). For dynamic apps, this is the project root (e.g. where package.json or server entry point lives).",
        ),
    },
  },
  async ({ url, static: isStatic, sourceDir }) => {
    // Validate URL — agent must target port 3000
    let parsedUrl: URL;
    try {
      parsedUrl = new URL(url);
    } catch {
      return err(`Invalid URL "${url}"`);
    }

    const localHostnames = ["localhost", "127.0.0.1", "0.0.0.0", "::1"];
    if (!localHostnames.includes(parsedUrl.hostname)) {
      return err(`URL must be a local URL (localhost, 127.0.0.1, etc.). Got: ${parsedUrl.hostname}`);
    }
    if (parsedUrl.port !== "3000") {
      return err(`URL must use port 3000 (the sandbox reserved port). Got: ${url}`);
    }
    if (parsedUrl.pathname !== "/") {
      return err(`URL must be a root URL (e.g., http://localhost:3000/). Got pathname: ${parsedUrl.pathname}`);
    }

    // Validate sourceDir
    try {
      const stats = await stat(sourceDir);
      if (!stats.isDirectory()) {
        return err(`sourceDir "${sourceDir}" is not a directory`);
      }
    } catch {
      return err(`sourceDir "${sourceDir}" does not exist`);
    }

    if (isStatic) {
      try {
        const indexStats = await stat(join(sourceDir, "index.html"));
        if (!indexStats.isFile()) {
          return err(`"${join(sourceDir, "index.html")}" exists but is not a file`);
        }
      } catch {
        return err(`sourceDir "${sourceDir}" does not contain an index.html file`);
      }
    }

    // Check server is running on port 3000
    try {
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 3000);
      const response = await fetch(url, {
        method: "HEAD",
        signal: controller.signal,
        redirect: "manual",
      });
      clearTimeout(timeoutId);
      if (response.status >= 400) {
        return err(`Server at ${url} returned status ${response.status}. Is it running?`);
      }
    } catch (e) {
      return err(`Cannot connect to ${url}. Start the server on port 3000 first. (${e instanceof Error ? e.message : e})`);
    }

    return renderResult({ url, static: isStatic, sourceDir });
  },
);

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error("UI MCP server running on stdio");
}

main().catch((error) => {
  console.error("Fatal error:", error);
  process.exit(1);
});
