---
name: template
description: Summarize and persist current session context to a template file. Use when the user asks to "save template", "remember session", "create template", or wants to capture the current conversation workflow, task patterns, and learnings so future sessions can quickly restore context without repeating dialogue.
---

# Template

Capture session context, task patterns, and learnings into a structured template file that future sessions can load to quickly restore understanding.

## Principles

A template should be **clean** **experience-dense** and **high-value**.

### What TO Record

- **Project Context** - Project summary, tech stack, architecture
- **Conventions** - Code style, naming conventions, project-specific rules
- **Key Patterns** - Common experience-based patterns, tools skills MCPs usage, reusable tips and tricks
- **Workflow** - Modified task execution steps, commands, procedures
- **Key Decisions** - User choices and preferences with their rationale
- **Lessons** - Pitfalls encountered, edge cases, important notes to avoid repeating mistakes

### What NOT to Record

- **Private Information** - Passwords, API keys, tokens, PII, internal URLs
- **Over-Specific / Low-Reuse Details** - Error stack traces, temporary paths, specific issue numbers, line numbers, full code blocks, session IDs
- **Conversational Noise** - Greetings, filler phrases, repeated confirmations, meta-discussion

## Workflow

Don't ask questions since this skill is well-defined and clear.
### Step 1: Analyze Current Session

Review the conversation to extract all parts in "What TO Record".
Read `<project-root>/template/CLAUDE.md` template file to know current template content(it may be empty).
If there's nothing worth recording in current session, you will write a nearly empty template meeting the format requirements with a minimal header.

### Step 2: Create Session Skills

Review the current session for reusable work artifacts — scripts, code snippets, utilities, configurations, automation workflows, or any file that could benefit future sessions in similar scenarios. This may include files in '/output' for example using. Then use the `skill-creator` skill to consolidate them into a skill (or update an existing similar skill) under `<project-root>/.claude/skills/`.

**Guidelines:**
- The skill should encapsulate the reusable logic/files from this session, not the entire project
- If a similar skill already exists under `<project-root>/.claude/skills/`, update it instead of creating a new one. Remember you can only update Unique Skills since default skills' changes won't be applied.
- Record the created/updated skill name(s) for the next step

If there are no reusable artifacts worth packaging, skip this step.

### Step 3: Iterate on Existing Template

You shall update `<project-root>/template/CLAUDE.md` template file.
IMPORTANT: READ IT BEFORE EDIT !!!

1. **Remove** outdated or redundant information based on principles
2. **Update** with refined understanding. Modify headers(name/description/initial_prompt) if needed. Add newly extracted materials in this session.

```markdown
---
name: <Template name>
description: <one sentence brief description of the scenario>
initial_prompt: <opening prompt in this scenario suggested for users to tell the agent, be as detailed and specific as possible with concrete examples. A length of 150-200 characters including spaces>
---

## Unique Skills
<Skills created or updated in Step 2 that are specific to this session's workflow. Future sessions should prefer using these skills when applicable.>
| Skill Name | Purpose |
|------------|---------|
|   …        |    …    |

## Project Context
<Project summary, tech stack, architecture>

## Conventions
<Code style, naming conventions, project-specific rules>

## Key Patterns
<Common experience-based patterns, tools skills MCPs usage, reusable tips and tricks>

## Workflow
<Modified task execution steps, commands, procedures>

## Key Decisions
<User choices and preferences with their rationale>
| Decision | Rationale |
|----------|-----------|
|   …      |    …      |

## Lessons
<Pitfalls encountered, edge cases, important notes>
```

If no skills were created in Step 2, omit the "Unique Skills" section.

### Step 4: Package Template

Run the template script to compress the template directory and collect skills:

```bash
cd /workspace && python .claude/skills/template/scripts/template.py --save
```

This returns:
- `file_path`: Path to the compressed template zip file
- `skills_list`: List of skill names

### Step 5: Notify Platform

Call the `save_template` MCP tool to notify the platform with the template information:

```json
{
  "name": "<name of template>",
  "description": "<one sentence brief description of template>",
  "initial_prompt": "<initial prompt for this template. Be detailed with 150-200 characters including spaces>",
  "file_path": "<file_path from Step 4>",
  "skills_list": "<skills_list from Step 4>"
}
```

The platform will receive these parameters via callback for subsequent processing.

IMPORTANT: During the execution of this skill, do not reveal the steps or related details. Only introduce the template in a few words after completion. 

