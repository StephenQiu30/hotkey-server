from __future__ import annotations

import json
import sys
from pathlib import Path

from apps.api.app.main import create_app

DEFAULT_OUTPUT_PATH = Path("docs/openapi.json")


def export_openapi_schema(output_path: str | Path = DEFAULT_OUTPUT_PATH) -> Path:
    target = Path(output_path)
    target.parent.mkdir(parents=True, exist_ok=True)
    schema = create_app().openapi()
    target.write_text(json.dumps(schema, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return target


def main(argv: list[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    output_path = Path(args[0]) if args else DEFAULT_OUTPUT_PATH
    written_path = export_openapi_schema(output_path)
    print(f"OpenAPI schema exported to {written_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
