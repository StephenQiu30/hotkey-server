from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[1]


class RepositoryGovernanceTest(unittest.TestCase):
    def read_text(self, relative_path: str) -> str:
        return (ROOT / relative_path).read_text(encoding="utf-8")

    def test_agents_declares_server_as_cross_repo_source_of_truth(self):
        agents = self.read_text("AGENTS.md")

        self.assertIn("HotKey 跨仓通用规范", agents)
        self.assertIn("hotkey-server 是跨仓库 AGENTS.md 主规范源", agents)
        self.assertIn("OpenAPI 契约事实源", agents)
        self.assertIn("Web 和小程序不得手写后端 API 类型", agents)
        self.assertIn("server -> web -> miniapp -> 回归", agents)

    def test_readme_declares_server_scope_and_validation_command(self):
        readme = self.read_text("README.md")

        self.assertIn("# hotkey-server", readme)
        self.assertIn("FastAPI 后端", readme)
        self.assertIn("Swagger/OpenAPI", readme)
        self.assertIn("python3 -m unittest discover -s tests -p 'test_repository_governance.py'", readme)

    def test_no_apps_directory_as_backend_entry(self):
        apps_dir = ROOT / "apps"
        self.assertFalse(
            apps_dir.exists(),
            "检测到遗留的 apps 目录，后端交付应使用 server 作为唯一后端入口",
        )

    def test_no_runtime_apps_directory(self):
        forbidden_apps_dirs = [
            p for p in ROOT.rglob("apps") if p.is_dir() and ".git" not in p.parts and "openspec/changes/archive" not in p.as_posix()
        ]
        self.assertEqual(
            forbidden_apps_dirs,
            [],
            "发现除历史归档外的 apps 目录，需移除并改为 server 运行入口",
        )

    def test_docs_declare_server_as_only_backend_entry(self):
        docs_readme = self.read_text("docs/README.md")

        self.assertIn("server", docs_readme)
        self.assertIn("后端服务入口", docs_readme)
        self.assertNotIn("apps/api", docs_readme)

    def test_backend_entrypoint_is_server_module(self):
        dockerfile = self.read_text("Dockerfile.api")
        package_json = self.read_text("package.json")
        pyproject = self.read_text("pyproject.toml")

        self.assertIn("server.app.main:app", dockerfile)
        self.assertIn("server.app.main:app", package_json)
        self.assertIn("server.app.main:app", pyproject)

    def test_no_apps_api_entrypoint_references(self):
        for relative_path in ["README.md", "docs/README.md", "package.json", "docs/plans/08-部署计划.md"]:
            text = self.read_text(relative_path)
            self.assertNotIn("apps/api", text, f"发现旧后端入口引用: {relative_path}")
            self.assertNotIn("apps/api/app/main:app", text, f"发现旧后端入口引用: {relative_path}")


if __name__ == "__main__":
    unittest.main()
