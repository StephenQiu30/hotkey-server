"""Read MinIO bucket retention controls without exposing connection identifiers."""

import json

from core.config import Settings
from evidence.adapters.minio import MinioEvidenceStore
from evidence.audit import audit_bucket


def main() -> None:
    store = MinioEvidenceStore.from_settings(Settings())
    print(json.dumps(audit_bucket(store.client, store.bucket), sort_keys=True))


if __name__ == "__main__":
    main()
