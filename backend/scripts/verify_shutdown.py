"""Verify normal Compose shutdown without Docker SIGKILL; run after E2E."""

import json
import subprocess


def main() -> None:
    roles = ("worker", "backend", "scheduler")
    identifiers = {
        role: subprocess.check_output(
            ["docker", "compose", "ps", "-q", role], text=True, timeout=5
        ).strip()
        for role in roles
    }
    assert all(identifiers.values()), "all application roles must be running before this check"
    subprocess.run(["docker", "compose", "stop", *roles], check=True, timeout=55)
    for role, identifier in identifiers.items():
        state = json.loads(
            subprocess.check_output(
                ["docker", "inspect", "--format", "{{json .State}}", identifier],
                text=True,
                timeout=5,
            )
        )
        assert not state["Running"] and not state["OOMKilled"]
        assert state["ExitCode"] in (0, 143), f"{role}: unexpected exit {state['ExitCode']}"
        print(json.dumps({"role": role, "exit_code": state["ExitCode"], "sigkill": False}))


if __name__ == "__main__":
    main()
