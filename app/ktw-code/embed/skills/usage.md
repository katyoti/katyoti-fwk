---
name: usage
description: Getting started guide for ktw-code — code analysis and review sidecar
category: system
advertised: false
---

# ktw-code — Usage Guide

ktw-code analyzes and reviews source code. It provides structured code explanation and multi-dimensional code review.

## Skills

| Skill | When to use |
|-------|------------|
| `ktw-code:code-explain` | Explain code architecture, configuration, security, tooling |
| `ktw-code:code-review` | Multi-dimensional code review (quality, security, performance, architecture) |

## Getting Started

```bash
# List available skills
ktw-code skills list

# Load code review instructions
ktw-code llm resources read ktw-code:/skill/code-review

# Load a specific review reference
ktw-code llm resources read ktw-code:/reference/configuration.md
```
