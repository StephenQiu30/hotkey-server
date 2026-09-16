import stat

import pytest

from cli.deletions import _read_manifest, _write_private


def test_manifest_file_is_private_and_exclusive(tmp_path):
    path = tmp_path / "withdrawals.json"
    _write_private(path, b'{"safe":true}')

    assert stat.S_IMODE(path.stat().st_mode) == 0o600
    assert path.read_bytes() == b'{"safe":true}'
    with pytest.raises(FileExistsError):
        _write_private(path, b"replacement")
    assert path.read_bytes() == b'{"safe":true}'


def test_manifest_reader_rejects_unknown_fields(tmp_path):
    path = tmp_path / "withdrawals.json"
    path.write_text(
        '{"schema_version":"content-withdrawal-manifest-v1",'
        '"generated_at":"2026-09-16T00:00:00Z","entries":[],"unexpected":true}'
    )

    with pytest.raises(RuntimeError, match="withdrawal_manifest_invalid"):
        _read_manifest(path)
