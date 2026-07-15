---
name: code-review
description: Perform an automated code review of a ticket and PR during the Agent Review phase.
---

# Code Review

This skill is invoked when a Linear issue transitions into the `Agent Review` state.
Your role as the reviewing agent is to rigorously evaluate the submitted work for completeness, correctness, and adherence to acceptance criteria before it reaches a human.

## Review Process

1. **Understand Context:**
   - Read the Linear issue details and thoroughly review the `## Codex Workpad` checklist.
   - Identify the stated Acceptance Criteria and Validation requirements.

2. **Review the Implementation:**
   - Inspect the code, diff, commit history, and tests against the ticket requirements.
   - Run the validation commands listed in the Workpad and record the actual results.
   - If there is UI or application-level behavior, consider using `harness-local-server` or `harness-playwright-evidence` if applicable.

3. **Verify Evidence:**
   - Look at the git diff and commit history.
   - Verify that test cases exist and that the code explicitly meets the requirements defined in the ticket.

4. **Make a Decision:**
   - **Reject (Rework):** If you find issues (failing tests, missed criteria, unresolved PR feedback, poor design, etc.):
     1. Leave clear and actionable feedback in the Workpad or as PR comments.
     2. Move the issue state to `Rework`.
     3. Restore the original developer's `agent:*` label using the `linear` skill.
   - **Approve:** If all requirements are completely satisfied and verified:
     1. Move the issue state to `Human Review` for final human approval.

## Guardrails
- **Do not write or refactor the code yourself** during this review. Your job is to verify and send it back to the implementer if incomplete.
- **Trust but verify**: Do not take checked boxes in the Workpad at face value. Inspect the code or test execution evidence.
