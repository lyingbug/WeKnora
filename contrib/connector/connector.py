#!/usr/bin/env python3
"""
Service connector — manages service integrations for agent sandboxes.

Adapted for WeKnora: can be used to manage external service credentials
and skill/plugin toggling in WeKnora's agent environment.

Usage:
  python3 connector.py '<json>'

JSON format:
  {
    "action": "connect" | "disconnect" | "refresh" | "sync",
    "services": {
      "<service>": { "credentials": { ... } },
      ...
    }
  }

Actions:

  +--------------+------------+---------------+
  | action       | env (.env) | skill toggle  |
  +--------------+------------+---------------+
  | connect      | write      | enable        |
  | disconnect   | clear      | disable       |
  | refresh      | write      | (unchanged)   |
  | sync         | write      | enable        |
  |  (unlisted)  | clear      | disable       |
  +--------------+------------+---------------+

All service definitions live in connectors/env_map.json.  Each entry maps
env var names to value expressions and may declare:

  map           - { "ENV_VAR": "<value>" } where <value> can be:
                    "${cred_key}" - resolved from the credentials payload
                    "literal"    - written as-is (fixed value)
                  Multiple refs are supported: "https://${HOST}:${PORT}"
  env_path      - relative path to .env inside the skill dir (default ".env")
  executables   - list of files to chmod +x after connect
  remove_keys   - env vars to remove (for legacy key migration)
  delete_env    - if true, delete the entire .env file on disconnect
                  (default false: only unset individual keys)
  sync_skills   - list of skill names to enable / disable together with this
                  service.  Skills listed here are skipped if they appear
                  explicitly in the payload (payload takes priority).

Skills are pre-installed under the configured SKILLS_DIR.  Enabling /
disabling is done by renaming SKILL.md <-> _SKILL.md.
"""

from __future__ import annotations

import json
import os
import re
import sys

from dotenv import set_key as dotenv_set_key
from dotenv import unset_key as dotenv_unset_key
from dotenv.main import StrPath

STAGING = os.environ.get("CONNECTOR_STAGING_DIR", "./connectors")
LIVE = os.environ.get("SKILLS_DIR", "./skills")
ENV_MAP_FILE = os.environ.get("ENV_MAP_FILE", "./connectors/env_map.json")

env_map: list[dict] = []

# ---------------------------------------------------------------------------
# env_map.json helpers
# ---------------------------------------------------------------------------

# The target .env file is created if it doesn't exist.
def set_key(
    dotenv_path: StrPath,
    key_to_set: str,
    value_to_set: str,
    **kwargs
):
    dotenv_set_key(dotenv_path=dotenv_path, key_to_set=key_to_set, value_to_set=value_to_set, **kwargs)


def unset_key(
    dotenv_path: StrPath,
    key_to_unset: str,
    **kwargs
):
    if os.path.isfile(dotenv_path):
        dotenv_unset_key(dotenv_path=dotenv_path, key_to_unset=key_to_unset, **kwargs)

def _load_env_map() -> list[dict]:
    """Load connectors/env_map.json. Returns empty list if file not found."""
    if not os.path.isfile(ENV_MAP_FILE):
        return []
    with open(ENV_MAP_FILE) as f:
        return json.load(f)

def _skill_names(entry: dict, service: str) -> list[str]:
    """Return the list of skill directory names for an env_map entry."""
    skill = entry.get("skill", service)
    return skill if isinstance(skill, list) else [skill]


def _entries_for(service: str, *, warn: bool = False) -> list[dict]:
    """Return env_map entries matching *service*, optionally warn if none."""
    entries = [e for e in env_map if e.get("service") == service]
    if warn and not entries:
        print(f"WARN: no env_map entry for service '{service}'", file=sys.stderr)
    return entries


def _all_env_map_services() -> set[str]:
    """Return the set of all service names declared in env_map.json."""
    return {e["service"] for e in env_map if "service" in e}


# ---------------------------------------------------------------------------
# Skill enable / disable  (rename SKILL.md <-> _SKILL.md)
# ---------------------------------------------------------------------------

def _enable_skill(skill_dir: str) -> None:
    """_SKILL.md -> SKILL.md (idempotent)."""
    hidden = os.path.join(skill_dir, "_SKILL.md")
    visible = os.path.join(skill_dir, "SKILL.md")
    if os.path.isfile(hidden) and not os.path.isfile(visible):
        os.rename(hidden, visible)


def _disable_skill(skill_dir: str) -> None:
    """SKILL.md -> _SKILL.md (idempotent)."""
    visible = os.path.join(skill_dir, "SKILL.md")
    hidden = os.path.join(skill_dir, "_SKILL.md")
    if os.path.isfile(visible) and not os.path.isfile(hidden):
        os.rename(visible, hidden)


# ---------------------------------------------------------------------------
# Env operations  (write / clear credentials in skill .env files)
# ---------------------------------------------------------------------------

_VAR_RE = re.compile(r"\$\{([^}]+)\}")


def _resolve_value(value_expr: str, credentials: dict) -> str | None:
    """Resolve a map value expression.

    * ``${CRED_KEY}`` — substitute from *credentials*; returns ``None`` when
      the key is missing or its value is ``None``.
    * Any other string — treated as a literal fixed value.

    Multiple ``${…}`` references within a single value are supported (e.g.
    ``"https://${HOST}:${PORT}"``), and mixing literal text with references is
    allowed.
    """
    refs = _VAR_RE.findall(value_expr)
    if not refs:
        # No ${…} at all → literal fixed value.
        return value_expr

    # Pure reference: value is exactly "${KEY}" (most common case).
    if value_expr == f"${{{refs[0]}}}" and len(refs) == 1:
        v = credentials.get(refs[0])
        return str(v) if v is not None else None

    # Mixed / multiple references — substitute each occurrence.
    def _replacer(m: re.Match) -> str:
        v = credentials.get(m.group(1))
        if v is None:
            return m.group(0)  # leave unresolved placeholder as-is
        return str(v)

    resolved = _VAR_RE.sub(_replacer, value_expr)
    # If nothing was resolved (all refs missing), treat as unresolvable.
    if resolved == value_expr:
        return None
    return resolved


def _write_env(service: str, credentials: dict) -> None:
    """Write credentials into skill .env files and chmod executables.

    Map format (env_map.json):
        { "ENV_VAR": "${credential_key}" }   — resolved from credentials
        { "ENV_VAR": "literal value" }        — written as-is
    """
    for entry in _entries_for(service):
        mapping: dict[str, str] = entry.get("map", {})
        remove_keys: list[str] = entry.get("remove_keys", [])
        env_rel: str = entry.get("env_path", ".env")
        executables: list[str] = entry.get("executables", [])

        for skill_name in _skill_names(entry, service):
            skill_dir = os.path.join(LIVE, skill_name)
            if not os.path.isdir(skill_dir):
                print(f"ERR {skill_dir} is not exists", file=sys.stderr)
                continue

            env_path = os.path.join(skill_dir, env_rel)
            os.makedirs(os.path.dirname(env_path), exist_ok=True)

            for env_var, value_expr in mapping.items():
                resolved = _resolve_value(value_expr, credentials)
                if resolved is not None:
                    set_key(env_path, env_var, resolved)
            for env_var in remove_keys:
                unset_key(env_path, env_var)
            # Ensure .env is readable by non-root users (connector runs as
            # root, but the agent process runs as 'user').
            if os.path.isfile(env_path):
                os.chmod(env_path, 0o644)
            for rel in executables:
                path = os.path.join(skill_dir, rel)
                if os.path.isfile(path):
                    os.chmod(path, 0o755)


def _clear_env(service: str) -> None:
    """Remove mapped env vars (or delete .env) for *service*."""
    for entry in _entries_for(service):
        mapping: dict[str, str] = entry.get("map", {})
        remove_keys: list[str] = entry.get("remove_keys", [])
        env_rel: str = entry.get("env_path", ".env")
        delete_env: bool = entry.get("delete_env", False)

        for skill_name in _skill_names(entry, service):
            skill_dir = os.path.join(LIVE, skill_name)
            env_path = os.path.join(skill_dir, env_rel)
            if os.path.isfile(env_path):
                if delete_env:
                    os.remove(env_path)
                else:
                    for env_var in list(mapping.keys()) + remove_keys:
                        unset_key(env_path, env_var)


# ---------------------------------------------------------------------------
# Skill toggle  (enable / disable service skills + sync_skills)
# ---------------------------------------------------------------------------

def _enable_service(service: str, payload_services: set[str]) -> None:
    """Enable the primary skill(s) and sync_skills for *service*."""
    for entry in _entries_for(service):
        for skill_name in _skill_names(entry, service):
            _enable_skill(os.path.join(LIVE, skill_name))
        for name in entry.get("sync_skills", []):
            if name not in payload_services:
                _enable_skill(os.path.join(LIVE, name))


def _disable_service(service: str, payload_services: set[str]) -> None:
    """Disable the primary skill(s) and sync_skills for *service*."""
    for entry in _entries_for(service):
        for skill_name in _skill_names(entry, service):
            _disable_skill(os.path.join(LIVE, skill_name))
        for name in entry.get("sync_skills", []):
            if name not in payload_services:
                _disable_skill(os.path.join(LIVE, name))


# ---------------------------------------------------------------------------
# Actions
# ---------------------------------------------------------------------------

def _do_connect(services: dict, payload_services: set[str], *, label: str = "connect") -> None:
    """Write credentials + enable skills."""
    for svc in sorted(services):
        creds = services[svc].get("credentials", {})
        try:
            if not _entries_for(svc, warn=True):
                continue
            _write_env(svc, creds)
            _enable_service(svc, payload_services)
            print(f"OK {label} {svc}")
        except Exception as e:
            print(f"ERR {label} {svc}: {e}", file=sys.stderr)


def _do_disconnect(services: dict, payload_services: set[str], *, label: str = "disconnect") -> None:
    """Clear credentials + disable skills."""
    for svc in sorted(services):
        try:
            if not _entries_for(svc, warn=True):
                continue
            _clear_env(svc)
            _disable_service(svc, payload_services)
            print(f"OK {label} {svc}")
        except Exception as e:
            print(f"ERR {label} {svc}: {e}", file=sys.stderr)


def _do_refresh(services: dict) -> None:
    """Update credentials only (skill toggle unchanged)."""
    for svc in sorted(services):
        creds = services[svc].get("credentials", {})
        try:
            if not _entries_for(svc, warn=True):
                continue
            _write_env(svc, creds)
            print(f"OK refresh {svc}")
        except Exception as e:
            print(f"ERR refresh {svc}: {e}", file=sys.stderr)


def _do_sync(services: dict) -> None:
    """Connect listed services, disconnect everything else."""
    payload_services = set(services)
    _do_connect(services, payload_services, label="sync")
    others = {svc: {} for svc in _all_env_map_services() - payload_services}
    if others:
        _do_disconnect(others, payload_services)


# ---------------------------------------------------------------------------
# Dispatch
# ---------------------------------------------------------------------------

_ACTIONS = {
    "connect": lambda s, ps: _do_connect(s, ps),
    "disconnect": lambda s, ps: _do_disconnect(s, ps),
    "refresh": lambda s, ps: _do_refresh(s),
    "sync": lambda s, ps: _do_sync(s),
}


def run(action: str, services: dict) -> None:
    """Execute a connector action for the given services."""
    handler = _ACTIONS.get(action)
    if handler is None:
        print(f"ERR: unknown action '{action}'", file=sys.stderr)
        return
    handler(services, set(services))


def main() -> None:
    if len(sys.argv) < 2:
        print("Usage: connector.py '<json>'", file=sys.stderr)
        sys.exit(1)

    os.makedirs(LIVE, exist_ok=True)

    payload = json.loads(sys.argv[1])
    run(payload["action"], payload.get("services", {}))


if __name__ == "__main__":
    env_map = _load_env_map()
    main()
