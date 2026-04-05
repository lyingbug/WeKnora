---
name: skill-finder
Proactively find, recommend and install specialized skills or workflow knowledge that can better fulfill the user's task. Use this when the user's request could benefit from specialized capabilities not already available in the session. Use when: (1) the user describes a task, workflow, or deliverable that could involve domain-specific capabilities (e.g., "generate a finance report");  (2) the user asks to find, discover, or search for skills or knowledge; (3) you are unsure whether an existing skill covers the user's need and want to check for better options. Returns a ranked list of Skills and, when relevant, a matching workflow knowledge.
---

# Skill Finder

## Overview

This skill helps users discover relevant skills and knowledge templates by analyzing their requirements and recommending the most appropriate ones. It uses a 3-step workflow: recall candidates, select and score, then recommend and install.

## Setup

Before running any commands, source the environment configuration:

```bash
source .claude/skills/skill-finder/.env
```

This sets up the necessary environment variables for API access and configuration.

## Workflow

1. **Recall candidates** - Retrieve initial skill and knowledge candidates based on user query
2. **Select and score** - Pick the most relevant skills and knowledge, calculate coverage scores (0-100)
3. **Recommend and install** - Generate final recommendations and optionally install skills and knowledge

## Step 1: Recall Candidates

Retrieve initial candidates from the repository.

```bash
python .claude/skills/skill-finder/scripts/skill_finder.py recall --query "<user's request>"
```

**Parameters:**
- `--query`: Search query (required)

**Note:** By default, the recall command filters out skills already installed in `.claude/skills/` and knowledge templates already installed in `.mule_knowledge/`, as well as previously failed installations.

**Output:**
```
Recalled Skills (20 candidates):

1. pdfsed [verified] (74728f16-388d-49da-ad5e-06785e0a1075)
   Replace text in PDF files while preserving fonts. Use when asked to find and
   replace text, update dates, or edit text in PDF documents.

2. pdf-tools (a3b4c5d6-e7f8-9012-3456-789abcdef012)
   Comprehensive PDF manipulation toolkit for merging, splitting, and rotating.

... (20 total results)

Recalled Knowledge (3 candidates):

1. pdf-processing-guide (t-001-abc)
   Comprehensive guide for PDF processing workflows including OCR, text
   extraction, and document transformation.
   Prompt: This template provides a structured approach to PDF processing...

... (3 total results)
```

## Step 2: Select and Score

### 2.1: Select Most Relevant Items

Review recalled candidates and pick the most relevant skills and knowledge items based on name, description, and capability alignment. Aim for roughly **5 skills** and **3 knowledge** items, but adjust based on your relevance judgment — select fewer if few candidates are truly relevant, or more if many are strong matches.

When two skills have similar relevance, **prefer the one marked `[verified]`**.

### 2.2: Identify Requirements

Extract all requirements from the user's query:
- **Main Actions**: Core tasks ("edit PDF", "fill forms")
- **Constraints**: Technical/domain requirements ("preserve fonts", "tax forms")
- **Features**: Additional capabilities ("batch processing", "preview")

**IMPORTANT**: Only include requirements that are **explicitly stated or directly implied** by clues in the user's query. You may make mild inferences based on contextual hints, but avoid excessive guessing about what the user might want if not clearly indicated.

**Example:**
```
Query: "edit PDF forms for tax documents with font preservation"

Requirements (5 total):
1. Edit PDF files
2. Fill form fields
3. Handle tax documents
4. Preserve fonts
5. Support form-specific features
```

### 2.3: Calculate Coverage Score

Count requirements satisfied by each skill or knowledge item. **Mark as non-satisfied ([✗]) if the requirement is not explicitly mentioned in the description.**

**Coverage Score = (Requirements Satisfied / Total Requirements) x 100**

**Example:**
```
Skill: pdfsed
- [✓] Edit PDF files (description mentions "Replace text in PDF files")
- [✓] Fill form fields (description mentions "edit text in PDF documents")
- [✓] Handle tax documents (description mentions precision editing)
- [✓] Preserve fonts (explicitly mentioned: "preserving fonts")
- [✓] Support form-specific features (implied by "PDF documents")
Coverage Score: (5/5) x 100 = 100
```

### 2.4: Prepare Scored Items

```json
Skills:
[
  {"skill_id": "74728f16-388d-49da-ad5e-06785e0a1075", "score": 100},
  {"skill_id": "a3b4c5d6-e7f8-9012-3456-789abcdef012", "score": 80}
]

Knowledge:
[
  {"template_id": "t-001-abc", "score": 90}
]
```

## Step 3: Recommend and Install

You **MUST** call the recommend command after recall. Never skip this step — it applies verification logic and quality scoring that you cannot replicate.

**Note:** The recommend step may return fewer skills than you submitted due to server-side filtering rules. If there are no explicit error messages, this is expected — do not retry or treat it as a failure. You are suggested to trust the recommend result over the raw recall output.

```bash
python .claude/skills/skill-finder/scripts/skill_finder.py recommend \
  --skills '{"skill_id":"<id>","score":100}' '{"skill_id":"<id>","score":80}' \
  --knowledge '{"template_id":"<id>","score":90}' \
  --install-all
```

**Parameters:**
- `--skills`: JSON string for each skill with skill_id and score (can be repeated)
- `--knowledge`: JSON string for each knowledge item with template_id and score (can be repeated)
- `--install-all`: Install all recommended skills to `.claude/skills/` and top 1 knowledge template to `.mule_knowledge/`
- `--no-install`: Skip installation (default)

At least one of `--skills` or `--knowledge` is required.

**Output:**
```
Downloading skills...
[OK] Downloaded 74728f16-388d-49da-ad5e-06785e0a1075
[OK] Installed 74728f16-388d-49da-ad5e-06785e0a1075 to .claude/skills/pdfsed

Downloading templates...
[OK] Downloaded t-001-abc
[OK] Installed t-001-abc to .mule_knowledge/pdf-processing-guide

Recommended Skills (5):

1. pdfsed [verified] (Score: 0.920) [✓ installed]
   ID: 74728f16-388d-49da-ad5e-06785e0a1075
   Quality: 0.380 | Relevance: 0.540 | Verified: 0.100
   Strengths:
     + Comprehensive PDF editing capabilities
     + Form field support with validation
   Weaknesses:
     - Large file size (~50MB)

2. pdf-tools (Score: 0.850) [✓ installed]
   ID: a3b4c5d6-e7f8-9012-3456-789abcdef012
   Quality: 0.340 | Relevance: 0.510 | Verified: 0.000

... (5 total)

Recommended Knowledge (1):

1. pdf-processing-guide (Score: 0.870) [✓ installed]
   ID: t-001-abc

Summary: 3 installed (1 template), 0 failed, 0 skipped

IMPORTANT - Post-install actions:
  You MUST read the CLAUDE.md of each installed knowledge template.
  It contains setup instructions and may reference knowledge-specific
  skills that are NOT auto-installed. Read the referenced SKILL.md
  files manually if you need those capabilities.
    -> .mule_knowledge/pdf-processing-guide/CLAUDE.md
```

**After installation**: When the output includes the `IMPORTANT - Post-install actions` block, you **MUST** follow it — read the listed `CLAUDE.md` files and check for any referenced knowledge-specific skills that are **not** auto-installed. Read their `SKILL.md` files manually if you need those capabilities.

## Direct Installation (Optional)

Install skills or knowledge directly without recommendation:

```bash
# Install skills
python .claude/skills/skill-finder/scripts/skill_finder.py install --skills <skill_id_1> <skill_id_2>

# Install knowledge templates
python .claude/skills/skill-finder/scripts/skill_finder.py install --knowledge <template_id_1>

# Install both
python .claude/skills/skill-finder/scripts/skill_finder.py install --skills <skill_id> --knowledge <template_id>
```

At least one of `--skills` or `--knowledge` is required.

- Skills are installed to `.claude/skills/`
- Knowledge templates are installed to `.mule_knowledge/`

**Accepted `skill_id` formats:**
- **UUID**: `74728f16-388d-49da-ad5e-06785e0a1075`
- **ref_key**: `@openai/skills#playwright:20260207151718` (format: `@org/repo#skill_name:version`)

## Error Handling

- **Connection errors**: Verify `APP_SKILL_FINDER_BASE_URL` is set and reachable
- **Invalid skill IDs**: Use IDs from recall output; ref keys use `@` prefix (e.g., `@org/repo#skill:version`) — do not strip it
- **Installation failures**: Check `.claude/skills/` and `.mule_knowledge/` are writable; retry for corrupted downloads
- **JSON errors**: Verify JSON format for --skills and --knowledge

## Tips

- Use broad queries for recall, then narrow down with scoring
- Coverage score = percentage of requirements satisfied
- Installation deduplicates automatically by name
- Failed installation attempts are tracked per session to avoid repeated failures
- When two skills have similar relevance, prefer the one marked `[verified]`
