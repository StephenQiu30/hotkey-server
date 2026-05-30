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

        for key in ["tracker:", "polling:", "workspace:", "hooks:", "agent:", "codex:"]:
            self.assertIn(key, front_matter)

        self.assertIn("kind: linear", front_matter)
        self.assertIn('api_key: "$LINEAR_API_KEY"', front_matter)
        self.assertIn('project_slug: "$SYMPHONY_LINEAR_PROJECT_SLUG"', front_matter)
        self.assertRegex(front_matter, re.compile(r"active_states:\n(\s+- .+\n)+"))
        self.assertRegex(front_matter, re.compile(r"terminal_states:\n(\s+- .+\n)+"))
        self.assertIn("{{ issue.identifier }}", body)
        self.assertIn("hotkey-server", body)


if __name__ == "__main__":
    unittest.main()
