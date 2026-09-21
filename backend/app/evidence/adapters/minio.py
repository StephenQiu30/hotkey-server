from minio import Minio


class VersionedBucketUnsupportedError(RuntimeError):
    """Physical cleanup needs explicit version deletion for a versioned bucket."""


class MinioObjectCleanup:
    def __init__(self, client: Minio, bucket: str) -> None:
        self._client = client
        self._bucket = bucket

    def __call__(self, reference: str) -> None:
        versioning = self._client.get_bucket_versioning(self._bucket)
        if versioning.status in {"Enabled", "Suspended"}:
            raise VersionedBucketUnsupportedError(
                "versioned MinIO buckets require version-aware cleanup"
            )
        self._client.remove_object(self._bucket, reference)
