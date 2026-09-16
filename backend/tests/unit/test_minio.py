from hashlib import sha256
from io import BytesIO
from types import SimpleNamespace

import pytest
from minio.error import S3Error
from urllib3.response import HTTPResponse

from evidence.adapters.minio import MinioEvidenceStore


class ObjectResponse:
    def __init__(self, payload: bytes):
        self.payload = BytesIO(payload)
        self.closed = False
        self.released = False

    def read(self, amount: int) -> bytes:
        return self.payload.read(amount)

    def close(self) -> None:
        self.closed = True

    def release_conn(self) -> None:
        self.released = True


class ObjectClient:
    def __init__(self):
        self.objects: dict[tuple[str, str], bytes] = {}
        self.puts = 0
        self.corrupt_on_put = False

    def stat_object(self, bucket: str, key: str):
        identity = (bucket, key)
        if identity not in self.objects:
            raise S3Error(
                HTTPResponse(), "NoSuchKey", "missing", key, "request", "host", bucket, key
            )
        return SimpleNamespace(size=len(self.objects[identity]))

    def get_object(self, bucket: str, key: str):
        return ObjectResponse(self.objects[(bucket, key)])

    def put_object(
        self,
        bucket: str,
        key: str,
        stream: BytesIO,
        length: int,
        *,
        content_type: str,
        metadata: dict[str, str],
    ):
        payload = stream.read(length)
        self.objects[(bucket, key)] = payload + b"corrupt" if self.corrupt_on_put else payload
        self.puts += 1
        return SimpleNamespace(etag="not-used-as-sha256")

    def list_objects(
        self,
        bucket: str,
        *,
        prefix: str,
        recursive: bool,
        include_version: bool,
    ):
        assert recursive and include_version
        return [
            SimpleNamespace(object_name=key, version_id=None, is_delete_marker=False)
            for object_bucket, key in self.objects
            if object_bucket == bucket and key.startswith(prefix)
        ]

    def remove_objects(self, bucket: str, objects):
        for item in objects:
            self.objects.pop((bucket, item.name), None)
        return iter(())

    def remove_object(self, bucket: str, key: str, version_id: str | None = None):
        self.objects.pop((bucket, key), None)


def test_put_verifies_readback_and_reuses_identical_object():
    client = ObjectClient()
    store = MinioEvidenceStore(client, "hotkey-evidence-test")
    payload = b"deterministic gzip bytes"
    digest = sha256(payload).hexdigest()

    first = store.put("raw/test/object.json.gz", payload, digest)
    second = store.put("raw/test/object.json.gz", payload, digest)

    assert first == second
    assert first.sha256 == digest and first.size == len(payload)
    assert client.puts == 1


def test_put_never_overwrites_conflict_or_accepts_failed_readback():
    client = ObjectClient()
    store = MinioEvidenceStore(client, "hotkey-evidence-test")
    key = "raw/test/object.json.gz"
    payload = b"expected"
    digest = sha256(payload).hexdigest()
    client.objects[(store.bucket, key)] = b"different"

    with pytest.raises(RuntimeError, match="evidence_object_conflict"):
        store.put(key, payload, digest)
    assert client.objects[(store.bucket, key)] == b"different"
    assert client.puts == 0

    client.objects.clear()
    client.corrupt_on_put = True
    with pytest.raises(RuntimeError, match="evidence_readback_mismatch"):
        store.put(key, payload, digest)


def test_put_rejects_caller_hash_mismatch_before_network_io():
    client = ObjectClient()
    store = MinioEvidenceStore(client, "hotkey-evidence-test")

    with pytest.raises(ValueError, match="evidence_hash_mismatch"):
        store.put("raw/test/object.json.gz", b"payload", "0" * 64)

    assert client.objects == {} and client.puts == 0


def test_delete_verifies_current_object_and_removes_exact_key_idempotently():
    client = ObjectClient()
    store = MinioEvidenceStore(client, "hotkey-evidence-test")
    key = "raw/test/object.json.gz"
    sibling = key + ".sibling"
    payload = b"immutable evidence"
    digest = sha256(payload).hexdigest()
    client.objects[(store.bucket, key)] = payload
    client.objects[(store.bucket, sibling)] = b"keep"

    store.delete(key, digest)
    store.delete(key, digest)

    assert (store.bucket, key) not in client.objects
    assert client.objects[(store.bucket, sibling)] == b"keep"


def test_delete_refuses_to_remove_an_object_with_an_unexpected_hash():
    client = ObjectClient()
    store = MinioEvidenceStore(client, "hotkey-evidence-test")
    key = "raw/test/object.json.gz"
    client.objects[(store.bucket, key)] = b"different"

    with pytest.raises(RuntimeError, match="evidence_object_conflict"):
        store.delete(key, sha256(b"expected").hexdigest())

    assert client.objects[(store.bucket, key)] == b"different"
