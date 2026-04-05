#!/usr/bin/env python3
"""Publish a directory to Mule Pages.

Usage:
    publish.py <local_dir> [--category static|dynamic] [--command JSON_ARRAY] [--port PORT]

Page name is derived from MULE_SESSION_ID (one page per session).

Requires environment variables:
    MULE_AGENT_TOKEN, MULE_PAGE_URL, MULE_BARN_URL, MULE_SESSION_ID

Exit codes:
    0 = success
    1 = error
    2 = missing environment variables (fatal, do not retry)
"""

import argparse
import json
import mimetypes
import os
import sys
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from typing import NoReturn
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------

MIN_CHUNK = 1 * 1024 * 1024    # 1 MB  (Barn minimum)
MAX_CHUNK = 100 * 1024 * 1024  # 100 MB (Barn maximum)
MAX_CHUNKS = 10                # aim for ≤10 chunks per file
MAX_RETRIES = 3
MAX_PARALLEL = 5
AGENT_NAME = "Mule Super Agent"
MAGIC_SUFFIX = "4ed153f9"


def fatal(msg: str, code: int = 1) -> NoReturn:
    print(f"FATAL: {msg}", file=sys.stderr)
    sys.exit(code)


def info(msg: str) -> None:
    print(msg, file=sys.stderr)


# ---------------------------------------------------------------------------
# Environment
# ---------------------------------------------------------------------------

def load_env() -> dict:
    required = {
        "MULE_PAGE_URL": "Page API base URL",
        "MULE_BARN_URL": "Barn API base URL",
        "MULE_AGENT_TOKEN": "Authentication token",
        "MULE_SESSION_ID": "Session identifier",
    }
    env = {}
    missing = []
    for key, desc in required.items():
        val = os.environ.get(key, "").strip()
        if not val:
            missing.append(f"  {key} ({desc})")
        else:
            env[key] = val
    if missing:
        fatal(
            "Missing required environment variables:\n"
            + "\n".join(missing)
            + "\nPublishing is not available.",
            code=2,
        )
    # Strip trailing slashes from URLs
    env["MULE_PAGE_URL"] = env["MULE_PAGE_URL"].rstrip("/")
    env["MULE_BARN_URL"] = env["MULE_BARN_URL"].rstrip("/")
    return env


# ---------------------------------------------------------------------------
# HTTP helpers
# ---------------------------------------------------------------------------

def api_request(
    method: str,
    url: str,
    token: str,
    data: dict | None = None,
    body: bytes | None = None,
    content_type: str = "application/json",
    timeout: int = 30,
) -> tuple[int, bytes]:
    """Make an HTTP request. Returns (status_code, response_body)."""
    if data is not None:
        body = json.dumps(data).encode()
    req = Request(url, method=method, data=body)
    req.add_header("Authorization", f"Bearer {token}")
    req.add_header("Content-Type", content_type)
    req.add_header("User-Agent", "MulePages/1.0")
    try:
        with urlopen(req, timeout=timeout) as resp:
            return resp.status, resp.read()
    except HTTPError as e:
        return e.code, e.read()
    except URLError as e:
        fatal(f"Network error: {e.reason}")


def api_json(method: str, url: str, token: str, data: dict | None = None) -> dict:
    """Make an API call and return parsed JSON."""
    status, body = api_request(method, url, token, data=data)
    try:
        return json.loads(body)
    except json.JSONDecodeError:
        fatal(f"Invalid JSON response (HTTP {status}): {body[:200]}")


# ---------------------------------------------------------------------------
# Page API (uses MULE_PAGE_URL)
# ---------------------------------------------------------------------------

def page_api(env: dict, method: str, path: str, data: dict | None = None) -> dict:
    url = f"{env['MULE_PAGE_URL']}{path}"
    return api_json(method, url, env["MULE_AGENT_TOKEN"], data)


def find_existing_page(env: dict, page_name: str) -> str | None:
    """Return domain if a page with this name exists, else None."""
    resp = page_api(env, "GET", "/api/page?page_number=1&page_size=100")
    if resp.get("code") != "ok":
        return None
    for page in resp.get("data", {}).get("pages", []):
        if page.get("name") == page_name:
            return page["domain"]
    return None


def create_page(env: dict, page_name: str, category: str) -> str:
    """Create a new page. Returns domain."""
    resp = page_api(env, "POST", "/api/page", {
        "name": page_name,
        "agent": AGENT_NAME,
        "category": category,
    })
    if resp.get("code") != "ok":
        fatal(f"Error creating page: {json.dumps(resp)}")
    return resp["data"]["domain"]


def list_versions(env: dict, domain: str) -> list[str]:
    """Return list of version strings, e.g. ['V1', 'V2']."""
    resp = page_api(env, "GET", f"/api/page/{domain}/artifact?page_number=1&page_size=10")
    if resp.get("code") != "ok":
        return []
    return [a["version"] for a in resp.get("data", {}).get("artifacts", [])]


def delete_version(env: dict, domain: str, version: str) -> None:
    page_api(env, "DELETE", f"/api/page/{domain}/artifact/{version}")


def create_version(env: dict, domain: str, version: str) -> None:
    resp = page_api(env, "POST", f"/api/page/{domain}/artifact", {"version": version})
    if resp.get("code") != "ok":
        fatal(f"Error creating version: {json.dumps(resp)}")


def publish_version(env: dict, domain: str, version: str, category: str,
                    command: list[str] | None, port: int | None) -> None:
    body: dict = {"version": version}
    if category == "dynamic":
        body["option"] = {
            "dynamic_page_command": command or ["npm", "start"],
            "dynamic_page_port": port or 8080,
        }
    resp = page_api(env, "POST", f"/api/page/{domain}/publish", body)
    if resp.get("code") != "ok":
        fatal(f"Error publishing: {json.dumps(resp)}")


# ---------------------------------------------------------------------------
# Barn upload (uses MULE_BARN_URL)
# ---------------------------------------------------------------------------

def upload_file(env: dict, local_path: Path, barn_path: str) -> bool:
    """Upload a single file to Barn. Returns True on success."""
    token = env["MULE_AGENT_TOKEN"]
    barn_url = env["MULE_BARN_URL"]
    file_size = local_path.stat().st_size
    mime_type = mimetypes.guess_type(str(local_path))[0] or "application/octet-stream"

    # Dynamic chunk size: ceil(file_size / MAX_CHUNKS), clamped to [MIN_CHUNK, MAX_CHUNK]
    chunk_size = max(MIN_CHUNK, min(MAX_CHUNK, -(-file_size // MAX_CHUNKS)))

    for attempt in range(1, MAX_RETRIES + 1):
        session_id = None
        try:
            # Step 1: Create upload session
            status, body = api_request("POST", f"{barn_url}/api/barn/v1/upload", token, data={
                "path": barn_path,
                "size": file_size,
                "mime_type": mime_type,
                "metadata": {},
                "chunk_size": chunk_size,
            })
            resp = json.loads(body)
            session_id = resp.get("session_id")
            if not session_id:
                raise ValueError(f"No session_id: {resp}")

            # Step 2: Upload chunks
            with open(local_path, "rb") as f:
                chunk_num = 0
                while True:
                    chunk = f.read(chunk_size)
                    if not chunk:
                        break
                    status, _ = api_request(
                        "POST",
                        f"{barn_url}/api/barn/v1/upload/{session_id}/chunks?chunk_number={chunk_num}",
                        token,
                        body=chunk,
                        content_type="application/octet-stream",
                        timeout=60,
                    )
                    if status not in (200, 201, 204):
                        raise ValueError(f"Chunk {chunk_num} failed: HTTP {status}")
                    chunk_num += 1

            # Step 3: Complete upload
            status, _ = api_request("POST", f"{barn_url}/api/barn/v1/upload/{session_id}", token)
            if status not in (200, 201):
                raise ValueError(f"Complete failed: HTTP {status}")

            return True

        except Exception as e:
            # Cancel session to avoid orphans
            if session_id:
                try:
                    api_request("DELETE", f"{barn_url}/api/barn/v1/upload/{session_id}", token)
                except Exception:
                    pass
            if attempt < MAX_RETRIES:
                time.sleep(attempt * 2)
                continue
            print(f"FAIL  {barn_path} ({e})", file=sys.stderr)
            return False

    return False  # unreachable, satisfies type checker


def upload_directory(env: dict, local_dir: Path, barn_prefix: str) -> tuple[int, int]:
    """Upload all files in a directory to Barn. Returns (succeeded, failed)."""
    files = [p for p in local_dir.rglob("*") if p.is_file()]
    info(f"Found {len(files)} files. Uploading with {MAX_PARALLEL} parallel workers...")

    succeeded = 0
    failed = 0

    def _upload(file_path: Path) -> tuple[Path, bool]:
        relative = file_path.relative_to(local_dir)
        barn_path = f"{barn_prefix}/{relative}"
        ok = upload_file(env, file_path, barn_path)
        return file_path, ok

    with ThreadPoolExecutor(max_workers=MAX_PARALLEL) as pool:
        futures = {pool.submit(_upload, f): f for f in files}
        for future in as_completed(futures):
            file_path, ok = future.result()
            relative = file_path.relative_to(local_dir)
            if ok:
                size = file_path.stat().st_size
                print(f"OK    {relative} ({size} bytes)", file=sys.stderr)
                succeeded += 1
            else:
                failed += 1

    return succeeded, failed


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def derive_page_name(session_id: str) -> str:
    return f"app-{session_id[:8]}-{MAGIC_SUFFIX}"


def next_version(versions: list[str]) -> str:
    if not versions:
        return "V1"
    max_v = max(int(v[1:]) for v in versions)
    return f"V{max_v + 1}"


def cleanup_old_versions(env: dict, domain: str, versions: list[str]) -> None:
    """Delete old versions to stay within the 3-version limit."""
    if len(versions) >= 3:
        to_delete = versions
    elif len(versions) >= 2:
        to_delete = versions[1:]  # keep newest
    else:
        to_delete = []
    for v in to_delete:
        info(f"Deleting old version {v}...")
        delete_version(env, domain, v)


def main():
    parser = argparse.ArgumentParser(description="Publish a directory to Mule Pages.")
    parser.add_argument("local_dir", help="Directory to publish")
    parser.add_argument("--category", default="static", choices=["static", "dynamic"])
    parser.add_argument("--command", help='Dynamic page command as JSON array, e.g. \'["node","server.js"]\'')
    parser.add_argument("--port", type=int, help="Dynamic page port (default: 8080)")
    args = parser.parse_args()

    local_dir = Path(args.local_dir)
    if not local_dir.is_dir():
        fatal(f"'{args.local_dir}' is not a directory")

    if args.category == "static" and not (local_dir / "index.html").is_file():
        fatal(f"Static pages require an index.html at the root of '{args.local_dir}'")

    dyn_command = json.loads(args.command) if args.command else None

    env = load_env()
    page_name = derive_page_name(env["MULE_SESSION_ID"])

    info(f"Publishing '{local_dir}' as '{page_name}' ({args.category})...")

    # Step 1: Find or create page
    domain = find_existing_page(env, page_name)
    if domain:
        info(f"Found existing page: {domain}")
        versions = list_versions(env, domain)
        version = next_version(versions)
        cleanup_old_versions(env, domain, versions)
    else:
        info("Creating new page...")
        domain = create_page(env, page_name, args.category)
        version = "V1"
        info(f"Created page: {domain}")

    # Step 2: Create version
    info(f"Creating version {version}...")
    create_version(env, domain, version)

    # Step 3: Upload files
    info("Uploading files...")
    barn_prefix = f"/__pages__/{domain}/{version}"
    succeeded, failed = upload_directory(env, local_dir, barn_prefix)
    info(f"\nDone: {succeeded} succeeded, {failed} failed.")

    if failed > 0:
        fatal("Some files failed to upload")

    # Step 4: Publish
    info(f"Publishing {version}...")
    publish_version(env, domain, version, args.category, dyn_command, args.port)

    # Step 5: Output
    page_url = f"https://{domain}/"
    info("")
    info("=== Published ===")
    info(f"URL:      {page_url}")
    info(f"Domain:   {domain}")
    info(f"Name:     {page_name}")
    info(f"Category: {args.category}")
    if args.category == "dynamic":
        info(f"Command:  {dyn_command or ['npm', 'start']}")
        info(f"Port:     {args.port or 8080}")
        info("Note:     First request may take 10-30s (cold start)")

    # Machine-readable output on stdout
    print(page_url)


if __name__ == "__main__":
    main()
