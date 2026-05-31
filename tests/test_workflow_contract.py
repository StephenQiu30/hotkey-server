import pathlib
import re
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]
WORKFLOW = ROOT / "WORKFLOW.md"
PRD_DIR = ROOT / "docs" / "product" / "prd"
PLAN_DIR = ROOT / "docs" / "plans"
SYMPHONY_SKILLS_DIR = ROOT / ".codex" / "skills"
REQUIRED_SYMPHONY_SKILLS = ("commit", "debug", "land", "linear", "pull", "push")
LAND_SKILL = ROOT / ".codex" / "skills" / "land" / "SKILL.md"
LAND_WATCH = ROOT / ".codex" / "skills" / "land" / "land_watch.py"


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
        self.assertNotIn("- Blocked", front_matter)
        self.assertIn("interval_ms: 5000", front_matter)
        self.assertIn("max_concurrent_agents: 4", front_matter)
        self.assertIn('git clone --depth 1 "$SOURCE_REPO_URL" .', front_matter)
        self.assertIn("approval_policy: never", front_matter)
        self.assertIn("thread_sandbox: danger-full-access", front_matter)
        self.assertIn("type: dangerFullAccess", front_matter)
        self.assertNotIn("thread_sandbox: workspace-write", front_matter)
        self.assertNotIn("type: workspaceWrite", front_matter)
        self.assertIn("read_timeout_ms: 30000", front_matter)
        self.assertIn("{{ issue.identifier }}", body)
        self.assertIn("hotkey-server", body)
        self.assertIn("## Status map", body)
        self.assertIn("## Step 0: Determine current ticket state and route", body)
        self.assertIn('update_issue(..., state: "In Progress")', body)
        self.assertIn("## Codex Workpad", body)
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
        self.assertIn(".codex/skills/land/SKILL.md", body)
        self.assertIn("## GitHub automation contract", body)
        self.assertIn("Use the authenticated `gh` CLI", body)
        self.assertIn("Do not use GitHub MCP/Connector tools", body)
        self.assertIn("interactive connector approval prompts", body)
        self.assertIn("gh pr create --repo StephenQiu30/hotkey-server", body)
        self.assertIn("gh pr edit <number> --repo StephenQiu30/hotkey-server --add-label symphony", body)
        self.assertIn("gh pr view <number> --repo StephenQiu30/hotkey-server --json", body)
        self.assertIn("gh pr checks <number> --repo StephenQiu30/hotkey-server", body)
        self.assertIn("gh api repos/StephenQiu30/hotkey-server/pulls/<number>/comments", body)

    def test_prd_and_plan_numbers_are_contiguous_and_paired(self):
        prds = numbered_markdown_files(PRD_DIR)
        plans = numbered_markdown_files(PLAN_DIR)

        self.assertGreater(len(prds), 0)
        self.assertEqual(len(prds), len(plans))

        expected = list(range(1, len(prds) + 1))
        self.assertEqual([number for number, _ in prds], expected)
        self.assertEqual([number for number, _ in plans], expected)

    def test_land_skill_is_available_for_merging_state(self):
        self.assertTrue(LAND_SKILL.exists())
        self.assertTrue(LAND_WATCH.exists())

        skill = LAND_SKILL.read_text(encoding="utf-8")
        watch = LAND_WATCH.read_text(encoding="utf-8")

        self.assertIn("name: land", skill)
        self.assertIn("gh pr view --json number", skill)
        self.assertIn("gh pr checks --watch", skill)
        self.assertIn("gh pr merge --squash", skill)
        self.assertIn("python3 .codex/skills/land/land_watch.py", skill)
        self.assertIn("async def get_pr_info", watch)
        self.assertIn("async def get_check_runs", watch)

    def test_required_symphony_skills_are_available(self):
        for skill_name in REQUIRED_SYMPHONY_SKILLS:
            with self.subTest(skill=skill_name):
                skill_file = SYMPHONY_SKILLS_DIR / skill_name / "SKILL.md"
                self.assertTrue(skill_file.exists())
                skill = skill_file.read_text(encoding="utf-8")
                self.assertIn(f"name: {skill_name}", skill)

        push_skill = (SYMPHONY_SKILLS_DIR / "push" / "SKILL.md").read_text(
            encoding="utf-8"
        )
        self.assertIn("make test", push_skill)
        self.assertIn("Test-first Evidence", push_skill)
        self.assertNotIn("make -C elixir all", push_skill)
        self.assertNotIn("mix pr_body.check", push_skill)

    def test_legacy_codex_agent_roles_are_not_committed(self):
        legacy_agents_dir = ROOT / ".codex" / "agents"
        self.assertEqual(list(legacy_agents_dir.glob("*.toml")), [])


if __name__ == "__main__":
    unittest.main()
