"""Generate the foundation contract without connecting to runtime dependencies."""

import argparse
import json
from pathlib import Path

from main import create_app


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    parser.add_argument(
        "--output",
        type=Path,
        default=Path(__file__).resolve().parents[3] / "docs/openapi/openapi.json",
    )
    args = parser.parse_args()
    path = args.output
    expected = (
        json.dumps(create_app().openapi(), ensure_ascii=False, indent=2, sort_keys=True) + "\n"
    )
    if args.check:
        if not path.exists() or path.read_text() != expected:
            raise SystemExit("Python OpenAPI is stale; run python -m tools.export_openapi")
    else:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(expected)


if __name__ == "__main__":
    main()
