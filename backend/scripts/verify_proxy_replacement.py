"""Verify that the running web proxy follows a replaced backend container."""

import json
import os
import subprocess
import time
import urllib.error
import urllib.request


def compose_output(*arguments: str) -> str:
    return subprocess.check_output(["docker", "compose", *arguments], text=True, timeout=10).strip()


def http_status(url: str) -> int:
    try:
        with urllib.request.urlopen(url, timeout=3) as response:
            return int(response.status)
    except urllib.error.HTTPError as error:
        return error.code


def main() -> None:
    web_before = compose_output("ps", "-q", "web")
    backend_before = compose_output("ps", "-q", "backend")
    assert web_before and backend_before, "web and backend must be running"

    subprocess.run(
        ["docker", "compose", "up", "-d", "--force-recreate", "--no-deps", "backend"],
        check=True,
        timeout=30,
    )
    backend_after = compose_output("ps", "-q", "backend")
    web_after = compose_output("ps", "-q", "web")
    assert web_after == web_before, "web must stay running during backend replacement"
    assert backend_after != backend_before, "backend container must be replaced"

    backend_port = os.getenv("HOTKEY_BACKEND_PORT", "8867")
    deadline = time.monotonic() + 45
    while time.monotonic() < deadline:
        try:
            if http_status(f"http://127.0.0.1:{backend_port}/health/ready") == 200:
                break
        except (OSError, urllib.error.URLError):
            pass
        time.sleep(1)
    else:
        raise RuntimeError("replacement backend did not become ready")

    web_port = os.getenv("HOTKEY_WEB_PORT", "8010")
    status = http_status(f"http://127.0.0.1:{web_port}/api/v1/session")
    assert status == 401, f"web proxy did not reach replacement backend: HTTP {status}"
    print(
        json.dumps(
            {
                "scope": "dynamic_proxy_backend_replacement",
                "web_replaced": False,
                "backend_replaced": True,
                "proxied_status": status,
            }
        )
    )


if __name__ == "__main__":
    main()
