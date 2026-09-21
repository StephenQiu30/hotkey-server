from redis import Redis


class RedisCacheCleanup:
    def __init__(self, client: Redis) -> None:
        self._client = client

    def __call__(self, reference: str) -> None:
        self._client.unlink(reference)
