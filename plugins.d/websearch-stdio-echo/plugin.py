#!/usr/bin/env python3
"""WeKnora stdio plugin: JSON-RPC 2.0, one message per line, logs on stderr."""

from __future__ import annotations

import json
import sys


def handle(msg: dict) -> dict | None:
    method = msg.get("method")
    if method == "shutdown":
        return None
    if method != "websearch.search":
        return {
            "jsonrpc": "2.0",
            "id": msg.get("id"),
            "error": {"code": -32601, "message": "method not found"},
        }
    params = msg.get("params") or {}
    query = str(params.get("query") or "").strip()
    results = []
    if query:
        results.append(
            {
                "title": "stdio-echo",
                "url": "https://weknora.local/plugin/stdio-echo",
                "snippet": query,
                "content": query,
                "source": "stdio-echo",
            }
        )
    return {"jsonrpc": "2.0", "id": msg.get("id"), "result": {"results": results}}


def main() -> None:
    for raw in sys.stdin:
        line = raw.strip()
        if not line:
            continue
        try:
            msg = json.loads(line)
        except json.JSONDecodeError:
            continue
        resp = handle(msg)
        if resp is None:
            return
        sys.stdout.write(json.dumps(resp, ensure_ascii=False) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    main()
