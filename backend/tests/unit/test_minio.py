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
