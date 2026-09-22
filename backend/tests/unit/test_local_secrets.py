from __future__ import annotations

import os
from pathlib import Path
from uuid import uuid4

import pytest

from connections.adapters.local_secrets import BrowserStateError, BrowserStateStore


def test_browser_state_is_immutable_and_scoped_by_owner_connection_version(
    tmp_path: Path,
) -> None:
    root = tmp_path / "browser-states"
    root.mkdir(mode=0o700)
    store = BrowserStateStore(root)
    owner_id, connection_id = uuid4(), uuid4()
    state = {"cookies": [], "origins": []}

    reference = store.save(owner_id=owner_id, connection_id=connection_id, version=1, state=state)

    assert reference == f"browser-state:{owner_id.hex}/{connection_id.hex}/1"
    assert (
        store.load(owner_id=owner_id, connection_id=connection_id, version=1, reference=reference)
        == state
    )
    assert (root / owner_id.hex).stat().st_mode & 0o777 == 0o700
    assert (root / owner_id.hex / connection_id.hex).stat().st_mode & 0o777 == 0o700
    assert (root / owner_id.hex / connection_id.hex / "1.json").stat().st_mode & 0o777 == 0o600
    with pytest.raises(BrowserStateError, match="already_exists"):
        store.save(owner_id=owner_id, connection_id=connection_id, version=1, state=state)
    assert (
        store.load(owner_id=owner_id, connection_id=connection_id, version=1, reference=reference)
        == state
    )
    with pytest.raises(BrowserStateError, match="reference_invalid"):
        store.load(owner_id=uuid4(), connection_id=connection_id, version=1, reference=reference)


def test_browser_state_rejects_weak_roots_symlinks_and_file_permissions(
    tmp_path: Path,
) -> None:
    root = tmp_path / "browser-states"
    root.mkdir(mode=0o700)
    alias = tmp_path / "alias"
    alias.symlink_to(root, target_is_directory=True)
    with pytest.raises(BrowserStateError, match="directory_invalid"):
        BrowserStateStore(alias)

    os.chmod(root, 0o755)
    with pytest.raises(BrowserStateError, match="directory_invalid"):
        BrowserStateStore(root)
    os.chmod(root, 0o700)
    store = BrowserStateStore(root)
    owner_id, connection_id = uuid4(), uuid4()
    outside = tmp_path / "outside"
    outside.mkdir(mode=0o700)
    (root / owner_id.hex).symlink_to(outside, target_is_directory=True)
    with pytest.raises(BrowserStateError, match="directory_invalid"):
        store.save(
            owner_id=owner_id,
            connection_id=connection_id,
            version=1,
            state={"cookies": [], "origins": []},
        )

    (root / owner_id.hex).unlink()
    reference = store.save(
        owner_id=owner_id,
        connection_id=connection_id,
        version=1,
        state={"cookies": [], "origins": []},
    )
    state_file = root / owner_id.hex / connection_id.hex / "1.json"
    os.chmod(state_file, 0o644)
    with pytest.raises(BrowserStateError, match="file_invalid"):
        store.load(owner_id=owner_id, connection_id=connection_id, version=1, reference=reference)
    state_file.unlink()
    state_file.symlink_to(tmp_path / "outside.json")
    with pytest.raises(BrowserStateError, match="file_invalid"):
        store.load(owner_id=owner_id, connection_id=connection_id, version=1, reference=reference)


def test_browser_state_rejects_invalid_content_and_versions(tmp_path: Path) -> None:
    root = tmp_path / "browser-states"
    root.mkdir(mode=0o700)
    store = BrowserStateStore(root)
    owner_id, connection_id = uuid4(), uuid4()
    with pytest.raises(BrowserStateError, match="state_invalid"):
        store.save(owner_id=owner_id, connection_id=connection_id, version=1, state={})
    with pytest.raises(BrowserStateError, match="version_invalid"):
        store.save(
            owner_id=owner_id,
            connection_id=connection_id,
            version=0,
            state={"cookies": [], "origins": []},
        )

    reference = store.save(
        owner_id=owner_id,
        connection_id=connection_id,
        version=1,
        state={"cookies": [], "origins": []},
    )
    state_file = root / owner_id.hex / connection_id.hex / "1.json"
    state_file.write_text('query-token {"cookies": [], "origins": []}')
    os.chmod(state_file, 0o600)
    with pytest.raises(BrowserStateError, match="state_invalid") as error:
        store.load(owner_id=owner_id, connection_id=connection_id, version=1, reference=reference)
    assert "query-token" not in str(error.value)

    state_file.write_bytes(b"x" * 1_048_577)
    with pytest.raises(BrowserStateError, match="state_invalid"):
        store.load(owner_id=owner_id, connection_id=connection_id, version=1, reference=reference)
