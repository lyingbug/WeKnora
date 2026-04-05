#!/usr/bin/env python3
"""Template script for compressing template/ and collecting skills list."""

import argparse
import json
import os
import shutil
import sys

# Platform-aware paths (driven by AGENT_PLATFORM env var)
CONFIG_DIR = os.environ.get("CONFIG_DIR", ".claude")
PROMPT_FILE = os.environ.get("PROMPT_FILE", "CLAUDE.md")

# Skills whitelist: loaded from build-time snapshot, fallback to default
try:
    with open(f"/workspace/{CONFIG_DIR}/.skills_whitelist_init", "r") as f:
        SKILLS_WHITELIST: list[str] = [line.strip() for line in f if line.strip()]
except (FileNotFoundError, PermissionError):
    SKILLS_WHITELIST: list[str] = ["template"]

# Working directory (always /workspace, regardless of cwd)
WORKSPACE_DIR = "/workspace"

# Paths relative to working directory
TEMPLATE_SOURCE_DIR = os.path.join(WORKSPACE_DIR, "template")
TEMPLATE_SKILLS_DIR = os.path.join(TEMPLATE_SOURCE_DIR, "skills")
TEMPLATE_ZIP_PATH = os.path.join(WORKSPACE_DIR, "template.zip")
SKILLS_DIR = os.path.join(WORKSPACE_DIR, CONFIG_DIR, "skills")


def get_skills_list() -> list[str]:
    """Get list of skill directory names, excluding whitelist."""
    skills = []
    if os.path.isdir(SKILLS_DIR):
        for name in os.listdir(SKILLS_DIR):
            full_path = os.path.join(SKILLS_DIR, name)
            if os.path.isdir(full_path) and name not in SKILLS_WHITELIST:
                skills.append(name)
    return sorted(skills)


def copy_skills_to_template(skills_list: list[str]) -> None:
    """Copy skills from config dir skills/ to template/skills/."""
    # Clear existing template/skills directory
    if os.path.exists(TEMPLATE_SKILLS_DIR):
        shutil.rmtree(TEMPLATE_SKILLS_DIR)

    # Create fresh template/skills directory
    os.makedirs(TEMPLATE_SKILLS_DIR, exist_ok=True)

    # Copy each skill directory
    for skill_name in skills_list:
        src_path = os.path.join(SKILLS_DIR, skill_name)
        dst_path = os.path.join(TEMPLATE_SKILLS_DIR, skill_name)
        if os.path.isdir(src_path):
            shutil.copytree(src_path, dst_path)


def compress_template() -> str:
    """Compress template/ to template.zip in working directory."""
    if not os.path.isdir(TEMPLATE_SOURCE_DIR):
        raise FileNotFoundError(f"Template directory not found: {TEMPLATE_SOURCE_DIR}")

    # Remove existing zip if exists
    if os.path.exists(TEMPLATE_ZIP_PATH):
        os.remove(TEMPLATE_ZIP_PATH)

    # Create zip (shutil.make_archive adds .zip automatically, so we strip it)
    zip_base = TEMPLATE_ZIP_PATH.removesuffix(".zip")
    shutil.make_archive(zip_base, "zip", TEMPLATE_SOURCE_DIR)

    return TEMPLATE_ZIP_PATH


def save() -> dict:
    """Main save function: compress template and return file_path and skills_list."""
    skills_list = get_skills_list()
    copy_skills_to_template(skills_list)
    file_path = compress_template()

    return {
        "file_path": file_path,
        "skills_list": skills_list,
    }


def main():
    parser = argparse.ArgumentParser(description="Template management script")
    parser.add_argument("--save", action="store_true", help="Save template")
    args = parser.parse_args()

    if args.save:
        try:
            result = save()
            print(json.dumps(result, indent=2))
        except Exception as e:
            print(json.dumps({"error": str(e)}), file=sys.stderr)
            sys.exit(1)
    else:
        parser.print_help()
        sys.exit(1)


if __name__ == "__main__":
    main()
