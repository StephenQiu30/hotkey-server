"""Exercise the real Compose API/scheduler/prefork worker; no source data or secrets."""

import json
import os
import subprocess
import time
import urllib.error
import urllib.request
from uuid import uuid4


def main() -> None:
    deadline = time.monotonic() + 45
    while True:
        try:
            with urllib.request.urlopen(
                f"http://127.0.0.1:{os.getenv('HOTKEY_BACKEND_PORT', '8867')}/health/ready",
                timeout=3,
            ) as response:
                assert json.load(response)["status"] == "ready"
            break
        except (OSError, urllib.error.URLError):
            if time.monotonic() >= deadline:
                raise
            time.sleep(1)
    base = ["docker", "compose", "exec", "-T", "backend", "python", "-m", "cli"]
    key = "compose-smoke-" + uuid4().hex
    job = json.loads(subprocess.check_output([*base, "enqueue", key], timeout=10))
    replay = json.loads(subprocess.check_output([*base, "enqueue", key], timeout=10))
    assert replay == job
    while time.monotonic() < deadline:
        state = json.loads(subprocess.check_output([*base, "show", job["job_id"]], timeout=10))
        if state["status"] == "succeeded":
            assert state["attempts"] == 1
            print(json.dumps({"scope": "compose_prefork_pipeline", **state}))
            return
        time.sleep(1)
    raise RuntimeError("prefork worker did not commit the diagnostic job")


if __name__ == "__main__":
    main()
