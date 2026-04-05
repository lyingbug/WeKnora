#!/usr/bin/env node

// Set process title so MCP servers are distinguishable from user node processes.
// Without this, "killall node" (to stop a user's app) would also kill MCP servers.
process.title = "weknora-mcp-template";

import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { z } from "zod";
import * as fs from "fs";
import * as path from "path";
import * as crypto from "crypto";

/**
 * MCP Server providing a save_template() tool.
 *
 * Flow:
 * 1. Read the file from sandbox filesystem
 * 2. Upload to Barn via chunked upload protocol
 * 3. Call session-mgmt create_template API to persist and get template_id
 * 4. Return structured result including template_id
 */

const BARN_URL = process.env.WEKNORA_STORAGE_URL || process.env.BARN_URL || "";
const AGENT_TOKEN = process.env.WEKNORA_AGENT_TOKEN || process.env.AGENT_TOKEN || "";
const AGENT_ID = process.env.WEKNORA_AGENT_ID || process.env.AGENT_ID || "";
const SESSION_ID = process.env.WEKNORA_SESSION_ID || process.env.SESSION_ID || "";

/**
 * Get agent_id and session_id from environment variables.
 */
function getWorkspaceIds(): { agentId: string; sessionId: string } {
  return { agentId: AGENT_ID, sessionId: SESSION_ID };
}

/**
 * Derive SESSION_MGMT_URL: prefer explicit env var, fallback to WEKNORA_GATEWAY_URL origin
 * (session-mgmt and agent-gateway share the same ingress host).
 */
function getSessionMgmtUrl(): string {
  if (process.env.WEKNORA_SESSION_MGMT_URL) return process.env.WEKNORA_SESSION_MGMT_URL;
  const gwUrl = process.env.WEKNORA_GATEWAY_URL || "";
  if (gwUrl) {
    try {
      const u = new URL(gwUrl);
      return u.origin;
    } catch { /* ignore */ }
  }
  return "";
}
const SESSION_MGMT_URL = getSessionMgmtUrl();
const CHUNK_SIZE = 5 * 1024 * 1024; // 5 MB

function barnHeaders(): Record<string, string> {
  return {
    Authorization: `Bearer ${AGENT_TOKEN}`,
    Cookie: `token=${AGENT_TOKEN}`,
  };
}

/**
 * Chunked upload to Barn:
 * 1. POST /api/barn/v1/upload → create upload session
 * 2. POST /api/barn/v1/upload/{sid}/chunks?chunk_number=N → upload chunks
 * 3. POST /api/barn/v1/upload/{sid} → complete upload, get oss_id
 */
async function uploadToBarn(
  filePath: string,
  fileName: string
): Promise<string> {
  const fileBuffer = fs.readFileSync(filePath);
  const fileSize = fileBuffer.length;
  const mimeType = "application/octet-stream";
  const uniqueId = crypto.randomUUID().replace(/-/g, "").slice(0, 8);
  const { agentId, sessionId } = getWorkspaceIds();
  const relativePath = `template/${uniqueId}/${fileName}`;

  // Step 1: Create upload session — use relativePath only;
  // barn internally stores under the agent workspace prefix.
  const createResp = await fetch(`${BARN_URL}/api/barn/v1/upload`, {
    method: "POST",
    headers: { ...barnHeaders(), "Content-Type": "application/json" },
    body: JSON.stringify({
      path: relativePath,
      size: fileSize,
      mime_type: mimeType,
      chunk_size: CHUNK_SIZE,
    }),
  });
  if (!createResp.ok) {
    const text = await createResp.text();
    throw new Error(
      `Barn create upload failed: ${createResp.status} ${text}`
    );
  }
  const createData = (await createResp.json()) as Record<string, unknown>;
  const uploadSessionId =
    (createData.session_id as string) || (createData.id as string);
  if (!uploadSessionId) {
    throw new Error(
      `No session_id in barn upload response: ${JSON.stringify(createData)}`
    );
  }

  // Step 2: Upload chunks
  let chunkNumber = 0;
  for (let offset = 0; offset < fileSize; offset += CHUNK_SIZE) {
    const chunk = fileBuffer.subarray(offset, offset + CHUNK_SIZE);
    const chunkResp = await fetch(
      `${BARN_URL}/api/barn/v1/upload/${uploadSessionId}/chunks?chunk_number=${chunkNumber}`,
      {
        method: "POST",
        headers: {
          ...barnHeaders(),
          "Content-Type": "application/octet-stream",
        },
        body: chunk,
      }
    );
    if (!chunkResp.ok) {
      const text = await chunkResp.text();
      throw new Error(
        `Barn chunk upload failed: ${chunkResp.status} ${text}`
      );
    }
    chunkNumber++;
  }

  // Step 3: Complete upload
  const completeResp = await fetch(
    `${BARN_URL}/api/barn/v1/upload/${uploadSessionId}`,
    {
      method: "POST",
      headers: barnHeaders(),
    }
  );
  if (!completeResp.ok) {
    const text = await completeResp.text();
    throw new Error(
      `Barn complete upload failed: ${completeResp.status} ${text}`
    );
  }
  const completeData = (await completeResp.json()) as Record<string, unknown>;
  // Barn stores files under the agent workspace internally. The returned
  // id/barn_id is the relative path without the workspace prefix.
  // Prepend __agent-workspace__/{agentId}/{sessionId}/ so downstream
  // get_file_info calls can locate the file.
  const barnPath =
    (completeData.id as string) ||
    (completeData.barn_id as string) ||
    relativePath;
  const ossId = agentId && sessionId
    ? `__agent-workspace__/${agentId}/${sessionId}/${barnPath}`
    : barnPath;

  console.error(
    `Barn upload complete: ${filePath} -> ${ossId} (${chunkNumber} chunks)`
  );
  return ossId;
}

/**
 * Call session-mgmt to create template record and get template_id.
 */
async function createTemplate(params: {
  name: string;
  description: string;
  initial_prompt: string;
  oss_id: string;
  skills: string[];
}): Promise<string> {
  const resp = await fetch(
    `${SESSION_MGMT_URL}/api/chat-sessions/public/templates`,
    {
      method: "POST",
      headers: {
        ...barnHeaders(),
        "Content-Type": "application/json",
      },
      body: JSON.stringify(params),
    }
  );
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(
      `Create template failed: ${resp.status} ${text}`
    );
  }
  const data = (await resp.json()) as Record<string, unknown>;
  const templateId = data.id as string;
  if (!templateId) {
    throw new Error(
      `No template_id in response: ${JSON.stringify(data)}`
    );
  }
  console.error(`Template created: ${templateId}`);
  return templateId;
}

const server = new McpServer({
  name: "template-server",
  version: "2.0.0",
});

// Register the save_template tool
server.registerTool(
  "save_template",
  {
    title: "Save Template",
    description:
      "Save a template: uploads the file to Barn, creates a template record, and returns the template_id.",
    inputSchema: {
      name: z.string().describe("Name of the template"),
      description: z.string().describe("Description of the template"),
      initial_prompt: z.string().describe("Initial prompt for the template"),
      file_path: z.string().describe("File path associated with the template"),
      skills_list: z
        .array(z.string())
        .describe("List of skills associated with the template"),
    },
    outputSchema: {
      template_id: z.string(),
      oss_id: z.string(),
      name: z.string(),
      description: z.string(),
      initial_prompt: z.string(),
      file_path: z.string(),
      skills_list: z.array(z.string()),
    },
  },
  async ({ name, description, initial_prompt, file_path, skills_list }) => {
    let template_id = "";
    let oss_id = "";

    try {
      // Step 1: Upload file to Barn
      const fileName = path.basename(file_path);
      oss_id = await uploadToBarn(file_path, fileName);

      // Step 2: Create template via session-mgmt API
      template_id = await createTemplate({
        name,
        description,
        initial_prompt,
        oss_id,
        skills: skills_list,
      });
    } catch (error) {
      console.error("save_template error:", error);
      // Return with empty template_id on failure so the caller knows it failed
      const errorOutput = {
        template_id: "",
        oss_id: "",
        name,
        description,
        initial_prompt,
        file_path,
        skills_list,
      };
      return {
        content: [
          {
            type: "text",
            text: `Error: ${error instanceof Error ? error.message : String(error)}`,
          },
        ],
        structuredContent: errorOutput,
        isError: true,
      };
    }

    const output = {
      template_id,
      oss_id,
      name,
      description,
      initial_prompt,
      file_path,
      skills_list,
    };

    return {
      content: [
        {
          type: "text",
          text: JSON.stringify(output),
        },
      ],
      structuredContent: output,
    };
  }
);

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error("Template MCP server running on stdio");
}

main().catch((error) => {
  console.error("Fatal error:", error);
  process.exit(1);
});
