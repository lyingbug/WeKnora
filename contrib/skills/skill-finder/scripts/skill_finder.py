#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Skill Finder - Discover and recommend skills based on user needs.

Usage:
    python skill_finder.py analyze --query "user's request"
"""

import argparse
import json
import math
import os
import re
import shutil
import sys
import tempfile
import time
import traceback
import zipfile
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime, timezone
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode
from urllib.request import HTTPRedirectHandler, Request, build_opener, urlopen

# Configure UTF-8 encoding for console output (Windows compatibility)
if sys.platform == "win32":
    import io

    sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")
    sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding="utf-8", errors="replace")

API_BASE_URL = os.environ.get(
    "APP_SKILL_FINDER_BASE_URL", "http://skill-finder.mulerun.com/api/v1"
)
if API_BASE_URL is None:
    print("Error: Skill finder API endpoint is not set", file=sys.stderr)
    sys.exit(1)

SESSION_ID = os.environ.get("MULE_SESSION_ID", None)

# Debug mode: set to True to print full stack traces on errors
DEBUG_MODE = os.environ.get("SKILL_FINDER_DEBUG", "").lower() in ("1", "true", "yes")


# HTTP wrapper functions for easy library switching
class HTTPPostRedirectHandler(HTTPRedirectHandler):
    """Custom handler to allow POST redirects (307/308)"""

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        """Return a Request or None in response to a redirect."""
        m = req.get_method()
        if code in (307, 308) and m == "POST":
            # Preserve POST data and method on 307/308 redirects
            return Request(
                newurl,
                data=req.data,
                headers=req.headers,
                origin_req_host=req.origin_req_host,
                unverifiable=req.unverifiable,
            )
        else:
            # Default behavior for other codes
            return HTTPRedirectHandler.redirect_request(
                self, req, fp, code, msg, headers, newurl
            )


def http_get(url, timeout=30):
    """
    Perform HTTP GET request.

    Args:
        url: URL to fetch
        timeout: Request timeout in seconds

    Returns:
        dict with keys: status_code, headers, content (bytes)

    Raises:
        HTTPError: For HTTP errors
        URLError: For connection errors
    """
    req = Request(url)
    req.add_header("User-Agent", "skill-finder/1.0")
    if SESSION_ID is not None:
        req.add_header("X-Session-Id", SESSION_ID)

    with urlopen(req, timeout=timeout) as response:
        return {
            "status_code": response.status,
            "headers": dict(response.headers),
            "content": response.read(),
        }


def http_post_json(url, data, timeout=30):
    """
    Perform HTTP POST request with JSON data.

    Args:
        url: URL to post to
        data: Dictionary to send as JSON
        timeout: Request timeout in seconds

    Returns:
        dict with keys: status_code, headers, content (bytes)

    Raises:
        HTTPError: For HTTP errors
        URLError: For connection errors
    """
    json_data = json.dumps(data).encode("utf-8")
    req = Request(url, data=json_data, method="POST")
    req.add_header("Content-Type", "application/json")
    req.add_header("User-Agent", "skill-finder/1.0")
    if SESSION_ID is not None:
        req.add_header("X-Session-Id", SESSION_ID)

    # Use custom opener that handles POST redirects
    opener = build_opener(HTTPPostRedirectHandler)
    with opener.open(req, timeout=timeout) as response:
        return {
            "status_code": response.status,
            "headers": dict(response.headers),
            "content": response.read(),
        }


def _sanitize_filename(filename):
    """
    Sanitize filename for Windows compatibility by removing invalid characters.

    Windows disallows: < > : " / \\ | ? *
    Also removes control characters and handles reserved names.
    """
    # Remove or replace invalid Windows filename characters
    sanitized = re.sub(r'[<>:"/\\|?*\x00-\x1f]', "_", filename)

    # Remove leading/trailing spaces and periods
    sanitized = sanitized.strip(". ")

    # Handle reserved Windows names (CON, PRN, AUX, NUL, COM1-9, LPT1-9)
    reserved_names = {
        "CON",
        "PRN",
        "AUX",
        "NUL",
        "COM1",
        "COM2",
        "COM3",
        "COM4",
        "COM5",
        "COM6",
        "COM7",
        "COM8",
        "COM9",
        "LPT1",
        "LPT2",
        "LPT3",
        "LPT4",
        "LPT5",
        "LPT6",
        "LPT7",
        "LPT8",
        "LPT9",
    }
    name_without_ext = Path(sanitized).stem.upper()
    if name_without_ext in reserved_names:
        sanitized = f"_{sanitized}"

    # Ensure we have a valid filename
    if not sanitized or sanitized == ".":
        sanitized = "unnamed_skill"

    return sanitized


def _safe_extract_zip(zip_ref, extract_path, max_size=100 * 1024 * 1024):
    """
    Safely extract zip file with path traversal and zip bomb protection.

    Args:
        zip_ref: ZipFile object
        extract_path: Target extraction directory
        max_size: Maximum allowed total uncompressed size (default 100MB)

    Raises:
        ValueError: If path traversal detected or size limit exceeded
    """
    extract_path = Path(extract_path).resolve()
    total_size = 0

    for member in zip_ref.namelist():
        # Validate path - prevent path traversal (zip slip)
        member_path = (extract_path / member).resolve()
        try:
            member_path.relative_to(extract_path)
        except ValueError:
            raise ValueError(f"Path traversal attempt detected: {member}")

        # Check size - prevent zip bombs
        info = zip_ref.getinfo(member)
        total_size += info.file_size
        if total_size > max_size:
            raise ValueError(
                f"Extracted size exceeds limit ({max_size} bytes): {total_size} bytes"
            )

        # Extract individual member
        zip_ref.extract(member, extract_path)


def _get_cache_dir():
    """
    Get the cache directory for tracking installation attempts.
    Returns None if SESSION_ID is not set.

    Returns:
        Path object or None
    """
    if SESSION_ID is None:
        return None

    cache_dir = Path(tempfile.gettempdir()) / f"skill_finder_cache_{SESSION_ID}"
    cache_dir.mkdir(parents=True, exist_ok=True)
    return cache_dir


def _read_failed_cache():
    """
    Read failed installation cache from disk (no locking, best effort).

    Returns:
        dict mapping skill_name to {"timestamp": str, "attempts": int, "error": str}
    """
    cache_dir = _get_cache_dir()
    if cache_dir is None:
        return {}

    cache_file = cache_dir / "failed_installs.json"
    if not cache_file.exists():
        return {}

    # No locking on read - just read what's there
    try:
        with cache_file.open("r", encoding="utf-8") as f:
            return json.load(f)
    except (json.JSONDecodeError, IOError):
        return {}  # Fail silently on read errors


def _write_failed_cache(cache_data):
    """
    Write failed installation cache to disk with write locking only.

    Args:
        cache_data: dict mapping skill_name to failure info
    """
    cache_dir = _get_cache_dir()
    if cache_dir is None:
        return

    cache_file = cache_dir / "failed_installs.json"
    lock_file = cache_dir / "failed_installs.lock"

    # Retry logic for write lock conflicts only
    max_retries = 3
    for attempt in range(max_retries):
        try:
            # Simple lock mechanism using lock file
            lock_file.touch()

            # Read existing data to merge (no retry, best effort)
            existing_data = {}
            if cache_file.exists():
                try:
                    with cache_file.open("r", encoding="utf-8") as f:
                        existing_data = json.load(f)
                except (json.JSONDecodeError, IOError):
                    pass  # If read fails, start fresh

            # Merge new data
            existing_data.update(cache_data)

            # Write atomically using temp file + rename
            temp_file = cache_file.with_suffix(".tmp")
            with temp_file.open("w", encoding="utf-8") as f:
                json.dump(existing_data, f, indent=2)

            # Atomic rename
            temp_file.replace(cache_file)

            # Remove lock
            try:
                lock_file.unlink()
            except Exception:
                pass

            return
        except Exception:
            if attempt < max_retries - 1:
                time.sleep(0.05 * (attempt + 1))  # Short backoff
            else:
                # Clean up lock file on final failure
                try:
                    lock_file.unlink()
                except Exception:
                    pass
                return  # Fail silently


def _track_failed_installs_batch(failures):
    """
    Track multiple failed skill installation attempts in a single write.

    Args:
        failures: dict mapping skill_name to error_msg
    """
    if not failures:
        return

    cache = _read_failed_cache()
    timestamp = datetime.now(timezone.utc).isoformat()

    updates = {}
    for skill_name, error_msg in failures.items():
        # Get existing entry or create new one
        entry = cache.get(skill_name, {"attempts": 0})
        entry["timestamp"] = timestamp
        entry["attempts"] = entry.get("attempts", 0) + 1
        entry["error"] = error_msg
        updates[skill_name] = entry

    # Single write for all failures
    _write_failed_cache(updates)


def _download_skill_from_api(skill_id, temp_dir, is_knowledge=False):
    """
    Download a single skill or knowledge template from API.

    Args:
        skill_id: ID of the skill or template to download
        temp_dir: Temporary directory for downloads
        is_knowledge: If True, use template_id query param

    Returns:
        Dict with keys: skill_id, skill_name, status, temp_path/error
    """
    # Initialize variables at the top to avoid fragile locals() checks
    metadata = None
    skill_name = skill_id  # Default fallback

    try:
        # Step 1: Get metadata (download_url, name) from API
        if is_knowledge:
            query_params = urlencode({"template_id": skill_id})
        elif skill_id.startswith("@"):
            query_params = urlencode({"ref_key": skill_id})
        else:
            query_params = urlencode({"hash_aggregation_id": skill_id})
        metadata_url = f"{API_BASE_URL}/skills/download?{query_params}"
        metadata_response = http_get(metadata_url, timeout=30)
        metadata = json.loads(metadata_response["content"].decode("utf-8"))

        download_url = metadata.get("download_url")
        # Knowledge uses "name" field; skills use "skill_name" with "name" fallback
        if is_knowledge:
            raw_name = metadata.get("name") or skill_id
        else:
            raw_name = metadata.get("skill_name") or metadata.get("name") or skill_id
        skill_name = _sanitize_filename(raw_name)

        if not download_url or not skill_name:
            return {
                "skill_id": skill_id,
                "skill_name": skill_name or skill_id,
                "status": "failed",
                "error": "Missing download_url or name in API response",
            }

        # Step 2: Download the package from the download_url
        download_response = http_get(download_url, timeout=60)

        # Save to temporary location with sanitized filename
        filename = f"{_sanitize_filename(skill_name)}.zip"
        temp_file = temp_dir / f"{_sanitize_filename(skill_id)}_{filename}"
        temp_file.write_bytes(download_response["content"])

        return {
            "skill_id": skill_id,
            "skill_name": skill_name,
            "status": "success",
            "temp_path": temp_file,
            "filename": filename,
        }
    except (HTTPError, URLError) as e:
        return {
            "skill_id": skill_id,
            "skill_name": skill_name,
            "status": "failed",
            "error": str(e),
        }
    except Exception as e:
        return {
            "skill_id": skill_id,
            "skill_name": skill_name,
            "status": "failed",
            "error": f"Unexpected error: {str(e)}",
        }


def _download_skills_concurrently(
    skill_ids, temp_dir, status_dict, failed_installs, is_knowledge=False
):
    """
    Download multiple skills or knowledge templates concurrently.

    Args:
        skill_ids: List of skill/template IDs to download
        temp_dir: Temporary directory for downloads
        status_dict: Dict to track skill status
        failed_installs: Dict to track failures by skill_name
        is_knowledge: If True, download as knowledge templates

    Returns:
        List of download results
    """
    entity_label = "templates" if is_knowledge else "skills"
    download_results = []
    print(f"Downloading {entity_label}...")
    with ThreadPoolExecutor(max_workers=5) as executor:
        future_to_skill = {
            executor.submit(
                _download_skill_from_api, skill_id, temp_dir, is_knowledge
            ): skill_id
            for skill_id in skill_ids
        }

        for future in as_completed(future_to_skill):
            result = future.result()
            download_results.append(result)

            if result["status"] == "success":
                print(f"[OK] Downloaded {result['skill_id']}")
            else:
                print(
                    f"[FAIL] Failed to download {result['skill_id']}: {result['error']}",
                    file=sys.stderr,
                )
                status_dict[result["skill_id"]] = "failed"
                failed_installs[result["skill_name"]] = result["error"]

    return download_results


def _deduplicate_downloads(download_results, status_dict):
    """
    Deduplicate download results by skill_name.

    Args:
        download_results: List of download results
        status_dict: Dict to track skill status

    Returns:
        List of unique download results
    """
    seen_skill_names = {}
    unique_downloads = []
    for result in download_results:
        if result["status"] == "success":
            skill_name = result["skill_name"]
            if skill_name not in seen_skill_names:
                seen_skill_names[skill_name] = result
                unique_downloads.append(result)
            else:
                print(
                    f"[SKIP] Skipping duplicate {result['skill_id']} (same skill_name as {seen_skill_names[skill_name]['skill_id']})"
                )
                status_dict[result["skill_id"]] = "skipped"

    return unique_downloads


def _extract_and_install_skills(
    unique_downloads,
    skills_dir,
    temp_dir,
    status_dict,
    failed_installs,
    entity_label="skill",
):
    """
    Extract and install skills/templates from downloaded zip files.

    Args:
        unique_downloads: List of unique download results
        skills_dir: Target directory for installation
        temp_dir: Temporary directory for extraction
        status_dict: Dict to track skill status
        failed_installs: Dict to track failures by skill_name
        entity_label: Label for print messages ("skill" or "template")

    Returns:
        List of installation results
    """
    print(f"\nExtracting and installing {entity_label}s...")
    install_results = []
    for result in unique_downloads:
        skill_name = result["skill_name"]

        try:
            temp_zip = result["temp_path"]

            # Create temporary extraction directory
            extract_temp = temp_dir / f"extract_{skill_name}"
            extract_temp.mkdir(exist_ok=True)

            # Extract zip file safely (with path traversal and zip bomb protection)
            with zipfile.ZipFile(temp_zip, "r") as zip_ref:
                _safe_extract_zip(zip_ref, extract_temp)

            # Post-extraction detection: if zip contains template.md, it's a knowledge template
            actual_dir = skills_dir
            actual_label = entity_label
            if entity_label != "template":
                # Check if this is actually a template (template_skill fallthrough)
                has_template_md = any(
                    (extract_temp / f).exists() for f in ["template.md", "TEMPLATE.md"]
                )
                if has_template_md:
                    actual_dir = Path(".mule_knowledge")
                    actual_dir.mkdir(parents=True, exist_ok=True)
                    actual_label = "template"

            # Give Windows a moment to release file handles
            time.sleep(0.1)

            # Move extracted directory to final location
            final_path = actual_dir / skill_name
            if final_path.exists():
                print(
                    f"[SKIP] {actual_label.capitalize()} already installed: {skill_name}"
                )
                status_dict[result["skill_id"]] = "skipped"
                continue

            shutil.move(str(extract_temp), str(final_path))

            install_results.append(
                {
                    "skill_id": result["skill_id"],
                    "status": "success",
                    "path": str(final_path),
                }
            )
            status_dict[result["skill_id"]] = "installed"
            print(f"[OK] Installed {result['skill_id']} to {final_path}")

        except zipfile.BadZipFile as e:
            error_msg = f"Invalid zip file: {str(e)}"
            install_results.append(
                {"skill_id": result["skill_id"], "status": "failed", "error": error_msg}
            )
            status_dict[result["skill_id"]] = "failed"
            failed_installs[skill_name] = error_msg
            print(
                f"[FAIL] Failed to install {result['skill_id']}: {error_msg}",
                file=sys.stderr,
            )
        except Exception as e:
            error_msg = f"Installation error: {str(e)}"
            install_results.append(
                {"skill_id": result["skill_id"], "status": "failed", "error": error_msg}
            )
            status_dict[result["skill_id"]] = "failed"
            failed_installs[skill_name] = error_msg
            print(
                f"[FAIL] Failed to install {result['skill_id']}: {error_msg}",
                file=sys.stderr,
            )

    return install_results


def _install_skills(skill_ids, install_dir=".claude/skills", is_knowledge=False):
    """
    Utility function to download and install skills or knowledge from a list of IDs.

    Args:
        skill_ids: List of skill/template IDs to install
        install_dir: Target directory for installation (default: ".claude/skills")
        is_knowledge: If True, download as knowledge templates

    Returns:
        Tuple of (success_count, failed_count, skipped_count, status_dict)
        status_dict maps skill_id to status: "installed", "failed", or "skipped"
    """
    entity_label = "template" if is_knowledge else "skill"

    # Create install directory if it doesn't exist
    skills_dir = Path(install_dir)
    skills_dir.mkdir(parents=True, exist_ok=True)

    # Create temporary directory for downloads
    temp_dir = Path(tempfile.mkdtemp(prefix="skill_install_"))

    status_dict = {}
    failed_installs = {}  # Track failures by skill_name for batch write

    # Download skills concurrently
    download_results = _download_skills_concurrently(
        skill_ids, temp_dir, status_dict, failed_installs, is_knowledge
    )

    # Deduplicate by skill_name
    unique_downloads = _deduplicate_downloads(download_results, status_dict)

    # Extract and install unique zip files (results tracked in status_dict)
    _extract_and_install_skills(
        unique_downloads,
        skills_dir,
        temp_dir,
        status_dict,
        failed_installs,
        entity_label,
    )

    # Cleanup temporary directory
    try:
        # Give Windows extra time to release all file handles
        time.sleep(0.2)
        shutil.rmtree(temp_dir)
    except Exception as e:
        print(
            f"Warning: Failed to cleanup temporary directory {temp_dir}: {e}",
            file=sys.stderr,
        )
        print("You may need to manually delete it later.", file=sys.stderr)

    # Calculate summary from status_dict (covers both download and install failures)
    success_count = sum(1 for s in status_dict.values() if s == "installed")
    failed_count = sum(1 for s in status_dict.values() if s == "failed")
    skipped_count = sum(1 for s in status_dict.values() if s == "skipped")

    # Track all failed installations in a single batch write
    _track_failed_installs_batch(failed_installs)

    return success_count, failed_count, skipped_count, status_dict


def recall_handler(args):
    url = f"{API_BASE_URL}/recall"
    if not args.query.strip():
        print("Error: Query cannot be empty", file=sys.stderr)
        sys.exit(1)

    # Build exclusion list from both failed and installed skills (unless disabled)
    exclude_skills = []
    exclude_knowledge = []

    if not args.no_filter:
        # Read failed installations cache
        failed_cache = _read_failed_cache()
        if failed_cache:
            exclude_skills.extend(failed_cache.keys())

        # Read installed skills from .claude/skills directory
        skills_dir = Path(".claude/skills")
        if skills_dir.exists() and skills_dir.is_dir():
            installed_skills = [
                item.name for item in skills_dir.iterdir() if item.is_dir()
            ]
            exclude_skills.extend(installed_skills)

        # Read installed knowledge templates from .mule_knowledge directory
        knowledge_dir = Path(".mule_knowledge")
        if knowledge_dir.exists() and knowledge_dir.is_dir():
            installed_knowledge = [
                item.name for item in knowledge_dir.iterdir() if item.is_dir()
            ]
            exclude_knowledge.extend(installed_knowledge)

    # Remove duplicates
    exclude_skills = list(set(exclude_skills)) if exclude_skills else None
    exclude_knowledge = list(set(exclude_knowledge)) if exclude_knowledge else None

    request_data = {
        "query": args.query,
        "topk": args.topk,
    }

    if args.topk_knowledge is not None:
        request_data["topk_knowledge"] = args.topk_knowledge

    # Add exclude parameters if we have items to exclude
    if exclude_skills:
        request_data["exclude"] = exclude_skills
    if exclude_knowledge:
        request_data["exclude_knowledge"] = exclude_knowledge

    try:
        response = http_post_json(url, request_data)
        results = json.loads(response["content"].decode("utf-8"))
    except json.JSONDecodeError as e:
        print(f"Error: Failed to parse API response as JSON: {e}", file=sys.stderr)
        sys.exit(1)
    except (HTTPError, URLError) as e:
        print(f"Error: API request failed: {e}", file=sys.stderr)
        sys.exit(1)

    # Format skill results
    if "results" in results and isinstance(results["results"], list):
        skills = results["results"]
        print(f"\nRecalled Skills ({len(skills)} candidates):\n")

        for idx, skill in enumerate(skills, 1):
            skill_id = skill.get("skill_id", "unknown")
            skill_name = skill.get("skill_name", "Unknown")
            description = skill.get("description", "No description available")
            is_verified = skill.get("is_verified", False)

            # Format with number, name, verified tag, and ID
            verified_tag = " [verified]" if is_verified else ""
            print(f"{idx}. {skill_name}{verified_tag} ({skill_id})")

            # Wrap description to ~80 characters for readability
            desc_lines = []
            words = description.split()
            current_line = "   "
            for word in words:
                if len(current_line) + len(word) + 1 <= 83:
                    current_line += word + " "
                else:
                    desc_lines.append(current_line.rstrip())
                    current_line = "   " + word + " "
            if current_line.strip():
                desc_lines.append(current_line.rstrip())

            print("\n".join(desc_lines))
            print()  # Blank line between entries
    else:
        # Fallback to JSON if structure is unexpected
        print(json.dumps(results, indent=2))

    # Format knowledge results
    knowledge = results.get("knowledge", [])
    if knowledge and isinstance(knowledge, list):
        print(f"\nRecalled Knowledge ({len(knowledge)} candidates):\n")

        for idx, item in enumerate(knowledge, 1):
            name = item.get("name", "Unknown")
            template_id = item.get("template_id", "unknown")
            description = item.get("description", "No description available")
            initial_prompt = item.get("initial_prompt", "")

            print(f"{idx}. {name} ({template_id})")

            # Wrap description
            desc_lines = []
            words = description.split()
            current_line = "   "
            for word in words:
                if len(current_line) + len(word) + 1 <= 83:
                    current_line += word + " "
                else:
                    desc_lines.append(current_line.rstrip())
                    current_line = "   " + word + " "
            if current_line.strip():
                desc_lines.append(current_line.rstrip())
            print("\n".join(desc_lines))

            # Show truncated initial_prompt (first 3 lines or ~200 chars)
            if initial_prompt:
                prompt_lines = initial_prompt.strip().splitlines()[:3]
                truncated = "\n".join(prompt_lines)
                if len(truncated) > 200:
                    truncated = truncated[:200]
                if (
                    len(prompt_lines) < len(initial_prompt.strip().splitlines())
                    or len(truncated) >= 200
                ):
                    truncated += "..."
                print(f"   Prompt: {truncated}")

            print()  # Blank line between entries


def _parse_and_validate_knowledge(knowledge_strings):
    """
    Parse and validate knowledge JSON strings with score normalization.

    Args:
        knowledge_strings: List of knowledge JSON strings

    Returns:
        List of parsed knowledge dicts with normalized scores

    Raises:
        SystemExit: If no valid knowledge items provided
    """
    items = []
    for item_str in knowledge_strings:
        try:
            item = json.loads(item_str)

            if "template_id" not in item:
                print(
                    f"Warning: Missing template_id in knowledge JSON: {item_str}",
                    file=sys.stderr,
                )
                continue

            # Validate and normalize score
            if "score" in item:
                score = item["score"]
                score = int(round(float(score)))
                score = max(0, min(100, score))
                item["score"] = score

            items.append(item)
        except (json.JSONDecodeError, ValueError, TypeError) as e:
            print(
                f"Warning: Failed to parse knowledge JSON: {item_str}", file=sys.stderr
            )
            print(f"  Error: {e}", file=sys.stderr)
            continue

    if not items:
        print("Error: No valid knowledge items provided", file=sys.stderr)
        sys.exit(1)

    return items


def _print_recommended_knowledge(knowledge_list, install_status):
    """
    Format and print recommended knowledge with install status.

    Args:
        knowledge_list: List of knowledge dicts from recommend API
        install_status: Dict mapping template_id to install status
    """
    print(f"Recommended Knowledge ({len(knowledge_list)}):\n")

    for idx, item in enumerate(knowledge_list, 1):
        name = _safe_string(item.get("name"), "Unknown")
        template_id = _safe_string(item.get("template_id"), "unknown")
        score = _safe_float(item.get("recommend_score"), 0.0)

        # Header with install status
        status = install_status.get(template_id)
        status_icon = {
            "installed": "\u2713",
            "failed": "\u2717",
            "skipped": "\u2298",
        }.get(status, "")
        status_text = f" [{status_icon} {status}]" if status else ""
        print(f"{idx}. {name} (Score: {score:.3f}){status_text}")
        print(f"   ID: {template_id}")
        print()


def _parse_and_validate_skills(skill_strings):
    """
    Parse and validate skill JSON strings with score normalization.

    Args:
        skill_strings: List of skill JSON strings

    Returns:
        List of parsed skill dicts with normalized scores

    Raises:
        SystemExit: If no valid skills provided
    """
    skills = []
    for skill_str in skill_strings:
        try:
            skill = json.loads(skill_str)

            # Validate and normalize score
            if "score" in skill:
                score = skill["score"]
                # Convert to int and clamp to 0-100 range
                score = int(round(float(score)))
                score = max(0, min(100, score))
                skill["score"] = score

            skills.append(skill)
        except (json.JSONDecodeError, ValueError, TypeError) as e:
            print(f"Warning: Failed to parse skill JSON: {skill_str}", file=sys.stderr)
            print(f"  Error: {e}", file=sys.stderr)
            continue

    if not skills:
        print("Error: No valid skills provided", file=sys.stderr)
        sys.exit(1)

    return skills


def _deduplicate_skills_for_install(skills_list):
    """
    Deduplicate recommended skills based on skill_name, keeping highest scored.

    Args:
        skills_list: List of skill dicts from recommend API

    Returns:
        List of skill_ids to install
    """
    skill_map = {}
    for skill in skills_list:
        if not isinstance(skill, dict):
            continue
        skill_name = skill.get("skill_name")
        skill_id = skill.get("skill_id")
        recommend_score = skill.get("recommend_score", 0)
        if not skill_name or not skill_id:
            continue
        if (
            skill_name not in skill_map
            or recommend_score > skill_map[skill_name]["recommend_score"]
        ):
            skill_map[skill_name] = {
                "skill_id": skill_id,
                "skill_name": skill_name,
                "recommend_score": recommend_score,
            }

    return [skill["skill_id"] for skill in skill_map.values()]


def _safe_float(value, default=0.0):
    """
    Safely convert a value to float, handling None, NaN, and invalid values.

    Args:
        value: Value to convert
        default: Default value if conversion fails or value is NaN/None

    Returns:
        Float value or default
    """
    if value is None:
        return default
    try:
        result = float(value)
        if math.isnan(result) or math.isinf(result):
            return default
        return result
    except (ValueError, TypeError):
        return default


def _safe_string(value, default=""):
    """
    Safely get a string value, handling None, NaN, and invalid values.

    Args:
        value: Value to convert
        default: Default value if conversion fails or value is invalid

    Returns:
        String value or default
    """
    if value is None:
        return default
    if isinstance(value, float) and math.isnan(value):
        return default
    if isinstance(value, str):
        return value
    # Convert other types to string
    try:
        return str(value)
    except (ValueError, TypeError):
        return default


def _safe_list(value, default=None):
    """
    Safely get a list value, handling None, NaN, and invalid values.

    Args:
        value: Value to convert
        default: Default value if conversion fails or value is invalid

    Returns:
        List value or default
    """
    if default is None:
        default = []
    if value is None:
        return default
    if isinstance(value, float) and math.isnan(value):
        return default
    if isinstance(value, str):
        return [value]
    if isinstance(value, list):
        return value
    return default


def _safe_dict(value, default=None):
    """
    Safely get a dict value, handling None, NaN, and invalid values.

    Args:
        value: Value to convert
        default: Default value if conversion fails or value is invalid

    Returns:
        Dict value or default
    """
    if default is None:
        default = {}
    if value is None:
        return default
    if isinstance(value, float) and math.isnan(value):
        return default
    if isinstance(value, dict):
        return value
    return default


def _print_recommended_skills(skills_list, install_status):
    """
    Format and print recommended skills with install status.

    Args:
        skills_list: List of skill dicts from recommend API
        install_status: Dict mapping skill_id to install status
    """
    print(f"Recommended Skills ({len(skills_list)}):\n")

    for idx, skill in enumerate(skills_list, 1):
        skill_name = _safe_string(skill.get("skill_name"), "Unknown")
        skill_id = _safe_string(skill.get("skill_id"), "unknown")
        score = _safe_float(skill.get("recommend_score"), 0.0)
        breakdown = _safe_dict(skill.get("breakdown"))
        strengths = _safe_list(skill.get("eval_strengths"))
        weaknesses = _safe_list(skill.get("eval_weaknesses"))
        is_verified = skill.get("is_verified", False)

        # Header with verified tag and install status
        verified_tag = " [verified]" if is_verified else ""
        status = install_status.get(skill_id)
        status_icon = {"installed": "✓", "failed": "✗", "skipped": "⊘"}.get(status, "")
        status_text = f" [{status_icon} {status}]" if status else ""
        print(f"{idx}. {skill_name}{verified_tag} (Score: {score:.3f}){status_text}")
        print(f"   ID: {skill_id}")

        # Breakdown
        if breakdown:
            q = _safe_float(breakdown.get("quality_score"), 0.0)
            r = _safe_float(breakdown.get("relevance_score"), 0.0)
            v = _safe_float(breakdown.get("verified_score"), 0.0)
            print(f"   Quality: {q:.3f} | Relevance: {r:.3f} | Verified: {v:.3f}")

        # Strengths/Weaknesses
        if strengths:
            print("   Strengths:")
            for strength in strengths:
                print(f"     + {strength}")
        if weaknesses:
            print("   Weaknesses:")
            for weakness in weaknesses:
                print(f"     - {weakness}")
        print()


def recommend_handler(args):
    url = f"{API_BASE_URL}/recommend"

    # Parse and validate inputs
    skills = []
    templates = []

    if args.skills:
        skills = _parse_and_validate_skills(args.skills)
    if args.knowledge:
        templates = _parse_and_validate_knowledge(args.knowledge)

    if not skills and not templates:
        print(
            "Error: At least one of --skills or --knowledge is required",
            file=sys.stderr,
        )
        sys.exit(1)

    # Call recommend API
    request_data = {"skills": skills, "templates": templates}
    try:
        response = http_post_json(url, request_data)
        results = json.loads(response["content"].decode("utf-8"))
    except json.JSONDecodeError as e:
        print(f"Error: Failed to parse API response as JSON: {e}", file=sys.stderr)
        sys.exit(1)
    except (HTTPError, URLError) as e:
        print(f"Error: API request failed: {e}", file=sys.stderr)
        sys.exit(1)

    # Handle installation if requested
    install_status = {}
    knowledge_install_status = {}
    template_count = 0

    if args.install:
        # Install ALL skills from results
        skills_list = results.get("results", [])
        if isinstance(skills_list, list) and skills_list:
            skills_to_install = _deduplicate_skills_for_install(skills_list)
            if skills_to_install:
                print("\n")
                _, _, _, install_status = _install_skills(skills_to_install)
                print()

        # Install TOP 1 knowledge from knowledge (sorted by recommend_score desc)
        knowledge_list = results.get("knowledge", [])
        if isinstance(knowledge_list, list) and knowledge_list:
            sorted_knowledge = sorted(
                knowledge_list,
                key=lambda x: _safe_float(x.get("recommend_score"), 0.0),
                reverse=True,
            )
            top_template_id = sorted_knowledge[0].get("template_id")
            if top_template_id:
                print()
                k_success, k_failed, k_skipped, knowledge_install_status = (
                    _install_skills(
                        [top_template_id],
                        install_dir=".mule_knowledge",
                        is_knowledge=True,
                    )
                )
                template_count = k_success
                print()

    # Format and print skill results
    num_submitted = len(skills)
    if "results" in results and isinstance(results["results"], list):
        skills_list = results["results"]
        _print_recommended_skills(skills_list, install_status)

        # Note when verified-skill filtering reduced the result set
        if (
            skills_list
            and len(skills_list) < num_submitted
            and all(s.get("is_verified", False) for s in skills_list)
        ):
            print(
                f"Note: {num_submitted} submitted, {len(skills_list)} returned "
                f"due to server-side filtering rules. The result is expected.\n"
            )
    else:
        print(json.dumps(results, indent=2))

    # Format and print knowledge results
    knowledge_list = results.get("knowledge", [])
    if knowledge_list and isinstance(knowledge_list, list):
        _print_recommended_knowledge(knowledge_list, knowledge_install_status)

    # Summary
    all_status = {**install_status, **knowledge_install_status}
    if all_status:
        installed = sum(1 for s in all_status.values() if s == "installed")
        failed = sum(1 for s in all_status.values() if s == "failed")
        skipped = sum(1 for s in all_status.values() if s == "skipped")
        template_note = f" ({template_count} template)" if template_count > 0 else ""
        print(
            f"Summary: {installed} installed{template_note}, {failed} failed, {skipped} skipped\n"
        )

        # Strong instruction: read CLAUDE.md of installed knowledge
        installed_knowledge_dirs = []
        for tid, st in knowledge_install_status.items():
            if st == "installed":
                knowledge_dir = Path(".mule_knowledge")
                if knowledge_dir.exists():
                    for d in knowledge_dir.iterdir():
                        if d.is_dir():
                            claude_md = d / "CLAUDE.md"
                            if claude_md.exists():
                                installed_knowledge_dirs.append(str(claude_md))
        if installed_knowledge_dirs:
            paths_list = "\n".join(f"    -> {p}" for p in installed_knowledge_dirs)
            print(
                f"IMPORTANT - Post-install actions:\n"
                f"  You MUST read the CLAUDE.md of each installed knowledge template.\n"
                f"  It contains setup instructions and may reference knowledge-specific\n"
                f"  skills that are NOT auto-installed. Read the referenced SKILL.md\n"
                f"  files manually if you need those capabilities.\n"
                f"{paths_list}\n"
            )

        if failed > 0:
            sys.exit(1)


def install_handler(args):
    """Handler for the install command"""
    skills_list = args.skills or []
    knowledge_list = args.knowledge or []

    if not skills_list and not knowledge_list:
        print(
            "Error: At least one of --skills or --knowledge is required",
            file=sys.stderr,
        )
        sys.exit(1)

    total_success = 0
    total_failed = 0
    total_skipped = 0
    template_count = 0

    # Install skills
    if skills_list:
        s, f, sk, _ = _install_skills(skills_list)
        total_success += s
        total_failed += f
        total_skipped += sk

    # Install knowledge templates
    if knowledge_list:
        s, f, sk, _ = _install_skills(
            knowledge_list, install_dir=".mule_knowledge", is_knowledge=True
        )
        total_success += s
        total_failed += f
        total_skipped += sk
        template_count = s

    template_note = f" ({template_count} template)" if template_count > 0 else ""
    print(
        f"\nSummary: {total_success} installed{template_note}, {total_failed} failed, {total_skipped} skipped\n"
    )
    if total_failed > 0:
        sys.exit(1)


def main():
    try:
        _main_impl()
    except KeyboardInterrupt:
        print("\nOperation cancelled by user.", file=sys.stderr)
        sys.exit(130)
    except Exception as e:
        print(f"Error: An unexpected error occurred: {e}", file=sys.stderr)
        if DEBUG_MODE:
            print("\n--- Debug Stack Trace ---", file=sys.stderr)
            traceback.print_exc(file=sys.stderr)
            print("--- End Stack Trace ---\n", file=sys.stderr)
        else:
            print(
                "Hint: Set SKILL_FINDER_DEBUG=1 for detailed error information.",
                file=sys.stderr,
            )
        sys.exit(1)


def _main_impl():
    """Main implementation - separated for clean exception handling."""
    parser = argparse.ArgumentParser(
        description="Skill Finder - Discover and recommend skills based on user needs",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )

    subparsers = parser.add_subparsers(dest="command", help="Available commands")

    # Recall command
    recall_parser = subparsers.add_parser(
        "recall", help="Recall skills and knowledge based on user requirements"
    )
    recall_parser.add_argument(
        "--query", required=True, help="Search query for skill recall"
    )
    recall_parser.add_argument(
        "--topk", type=int, default=20, help="Number of skills to recall (default: 20)"
    )
    recall_parser.add_argument(
        "--topk-knowledge",
        type=int,
        default=None,
        help="Number of knowledge results to recall (default: server-side 3)",
    )
    recall_parser.add_argument(
        "--no-filter",
        action="store_true",
        help="Disable filtering of already installed or failed skills and knowledge",
    )

    recommend_parser = subparsers.add_parser(
        "recommend", help="Recommend skills and knowledge based on user requirements"
    )
    recommend_parser.add_argument(
        "--skills",
        type=str,
        nargs="+",
        default=[],
        help="List of skills with their relevance scores (JSON string)",
    )
    recommend_parser.add_argument(
        "--knowledge",
        type=str,
        nargs="+",
        default=[],
        help="List of knowledge templates with their relevance scores (JSON string)",
    )
    recommend_parser.add_argument(
        "--install-all",
        action="store_true",
        dest="install",
        help="Install all recommended skills and top knowledge template",
    )
    recommend_parser.add_argument(
        "--no-install",
        action="store_false",
        dest="install",
        help="Do not install recommended skills (default)",
    )
    recommend_parser.set_defaults(install=False)

    install_parser = subparsers.add_parser(
        "install", help="Install skills and knowledge templates"
    )
    install_parser.add_argument(
        "--skills",
        type=str,
        nargs="+",
        default=[],
        help="List of skill IDs to install",
    )
    install_parser.add_argument(
        "--knowledge",
        type=str,
        nargs="+",
        default=[],
        help="List of template IDs to install to .mule_knowledge/",
    )

    args = parser.parse_args()

    if not args.command:
        parser.print_help()
        sys.exit(1)

    handlers = {
        "recall": recall_handler,
        "recommend": recommend_handler,
        "install": install_handler,
    }

    handler = handlers.get(args.command)
    if handler is None:
        parser.print_help()
        sys.exit(1)
    handler(args)


if __name__ == "__main__":
    main()
