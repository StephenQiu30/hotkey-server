import pathlib
import re
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]
WORKFLOW = ROOT / "WORKFLOW.md"


class WorkflowContractTest(unittest.TestCase):
    def test_workflow_file_uses_symphony_front_matter(self):
        text = WORKFLOW.read_text(encoding="utf-8")
        self.assertTrue(text.startswith("---\n"))
        front_matter = text.split("---\n", 2)[1]
        body = text.split("---\n", 2)[2].strip()

        for key in [
            "tracker:",
            "polling:",
            "workspace:",
            "hooks:",
            "agent:",
            "codex:",
            "claude:",
            "cursor:",
        ]:
            self.assertIn(key, front_matter)

        self.assertIn("kind: linear", front_matter)
        self.assertIn('project_slug: "$SYMPHONY_LINEAR_PROJECT_SLUG"', front_matter)
        self.assertRegex(front_matter, re.compile(r"active_states:\n(\s+- .+\n)+"))
        self.assertRegex(front_matter, re.compile(r"terminal_states:\n(\s+- .+\n)+"))
        self.assertIn("- Merging", front_matter)
        self.assertIn("- Rework", front_matter)
        self.assertIn("- Blocked", front_matter)
        self.assertIn("interval_ms: 5000", front_matter)
        self.assertIn("max_concurrent_agents: 4", front_matter)
        self.assertIn('git clone --depth 1 "$SOURCE_REPO_URL" .', front_matter)
        self.assertIn("default_runtime: codex", front_matter)
        self.assertIn("agent:codex: codex", front_matter)
        self.assertIn("command: codex app-server", front_matter)
        self.assertIn("command: claude -p --dangerously-skip-permissions", front_matter)
        self.assertIn("{{ issue.identifier }}", body)
        self.assertIn("hotkey-server", body)
        self.assertIn("## Status map", body)
        self.assertIn("## Step 0: Determine current ticket state and route", body)
        self.assertIn('update_issue(..., state: "In Progress")', body)
        self.assertIn("## Claude Workpad", body)
        self.assertIn("`Blocked` -> non-active blocked state", body)
        self.assertIn("move the ticket to `Blocked`", body)
        self.assertIn("If issue state is `Blocked`", body)
        self.assertIn("PR feedback sweep protocol", body)
        self.assertIn("Completion bar before Human Review", body)
        self.assertIn("zero unchecked required items", body)
        self.assertIn("issue description checkbox", body)
        self.assertIn("issue Plan checkbox", body)
        self.assertIn("If any required checkbox remains unchecked", body)
        self.assertIn("### Remaining Items", body)
        self.assertIn("Blocked access never bypasses the completion bar", body)
        self.assertIn("There is no blocker exception for incomplete work", body)
        self.assertIn("move it to `Blocked`", body)
        self.assertIn(".claude/skills/land/SKILL.md", body)
        self.assertIn("## GitHub automation contract", body)
        self.assertIn("Use the authenticated `gh` CLI", body)
        self.assertIn("Do not use GitHub MCP/Connector tools", body)
        self.assertIn("interactive connector approval prompts", body)
        self.assertIn("gh pr create --repo StephenQiu30/hotkey-server", body)
        self.assertIn("gh pr edit <number> --repo StephenQiu30/hotkey-server --add-label symphony", body)
        self.assertIn("gh pr view <number> --repo StephenQiu30/hotkey-server --json", body)
        self.assertIn("gh pr checks <number> --repo StephenQiu30/hotkey-server", body)
        self.assertIn("gh api repos/StephenQiu30/hotkey-server/pulls/<number>/comments", body)


if __name__ == "__main__":
    unittest.main()
