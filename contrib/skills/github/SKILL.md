---
name: github
description: "GitHub: Authenticated gh CLI. Load this skill before running any gh command."
---

# GitHub CLI (gh)

The platform writes `GH_TOKEN` into `scripts/.env` before the agent starts. Use the `gh` wrapper in this skill's `scripts/gh` for all operations — it loads the token automatically. The agent does not manage credentials.

```bash
gh <command> <subcommand> [flags]
```

## Auth

`GH_TOKEN` is a GitHub Personal Access Token injected by the platform. It takes priority over any `gh auth login` state. If a command returns a 401 or 403, **stop trying** — the platform will handle token refresh.

## Security Rules

- **Never** output secrets (tokens, API keys) directly
- **Always** confirm with user before destructive operations (deleting repos, closing issues, force-pushing)
- Prefer `--dry-run` where available
- Never force-push to main/master without explicit user confirmation
