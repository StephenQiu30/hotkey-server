from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from scripts.export_openapi import export_openapi_schema


class NotificationContractTests(unittest.TestCase):
    def test_notifications_openapi_uses_typed_list_response(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = Path(tmpdir) / "openapi.json"
            export_openapi_schema(output_path)
            schema = json.loads(output_path.read_text(encoding="utf-8"))

        self.assertIn("/api/notifications", schema["paths"])
        components = schema["components"]["schemas"]
        self.assertIn("NotificationRead", components)
        self.assertIn("NotificationListResponse", components)

        response_schema = schema["paths"]["/api/notifications"]["get"]["responses"]["200"]["content"]["application/json"]["schema"]
        self.assertEqual(response_schema["$ref"], "#/components/schemas/NotificationListResponse")

        properties = components["NotificationListResponse"]["properties"]
        self.assertEqual(properties["items"]["items"]["$ref"], "#/components/schemas/NotificationRead")
        self.assertIn("limit", properties)
        self.assertIn("offset", properties)


if __name__ == "__main__":
    unittest.main()
