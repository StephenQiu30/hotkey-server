import pathlib
import re
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]
WORKFLOW = ROOT / "WORKFLOW.md"
PRD_DIR = ROOT / "docs" / "product" / "prd"
PLAN_DIR = ROOT / "docs" / "plans"


def numbered_markdown_files(path):
    files = sorted(path.glob("*.md"))
    pairs = []
    for file in files:
        match = re.match(r"^(\d+)-", file.name)
        if match:
            pairs.append((int(match.group(1)), file.name))
    return pairs


class WorkflowContractTest(unittest.TestCase):
    def test_workflow_file_uses_symphony_front_matter(self):
        text = WORKFLOW.read_text(encoding="utf-8")
        self.assertTrue(text.startswith("---\n"))
        front_matter = text.split("---\n", 2)[1]
        body = text.split("---\n", 2)[2].strip()

        for key in ["tracker:", "polling:", "workspace:", "hooks:", "agent:", "codex:"]:
            self.assertIn(key, front_matter)

        self.assertIn("kind: linear", front_matter)
        self.assertIn('project_slug: "$SYMPHONY_LINEAR_PROJECT_SLUG"', front_matter)
        self.assertRegex(front_matter, re.compile(r"active_states:\n(\s+- .+\n)+"))
        self.assertRegex(front_matter, re.compile(r"terminal_states:\n(\s+- .+\n)+"))
        self.assertIn("- Merging", front_matter)
        self.assertIn("- Rework", front_matter)
        self.assertIn("interval_ms: 5000", front_matter)
        self.assertIn("max_concurrent_agents: 4", front_matter)
        self.assertIn('git clone --depth 1 "$SOURCE_REPO_URL" .', front_matter)
        self.assertIn("approval_policy: never", front_matter)
        self.assertIn("{{ issue.identifier }}", body)
        self.assertIn("hotkey-server", body)
        self.assertIn("## Status map", body)
        self.assertIn("## Step 0: Determine current ticket state and route", body)
        self.assertIn('update_issue(..., state: "In Progress")', body)
        self.assertIn("## Codex Workpad", body)
        self.assertIn("PR feedback sweep protocol", body)
        self.assertIn("Completion bar before Human Review", body)
        self.assertIn(".codex/skills/land/SKILL.md", body)
        self.assertIn("## GitHub automation contract", body)
        self.assertIn("Use the authenticated `gh` CLI", body)
        self.assertIn("Do not use GitHub MCP/Connector tools", body)
        self.assertIn("interactive connector approval prompts", body)
        self.assertIn("gh pr create --repo StephenQiu30/hotkey-server", body)
        self.assertIn("gh pr checks <number> --repo StephenQiu30/hotkey-server", body)

    def test_prd_and_plan_numbers_are_contiguous_and_paired(self):
        prds = numbered_markdown_files(PRD_DIR)
        plans = numbered_markdown_files(PLAN_DIR)

        self.assertGreater(len(prds), 0)
        self.assertEqual(len(prds), len(plans))

        expected = list(range(1, len(prds) + 1))
        self.assertEqual([number for number, _ in prds], expected)
        self.assertEqual([number for number, _ in plans], expected)


if __name__ == "__main__":
    unittest.main()
