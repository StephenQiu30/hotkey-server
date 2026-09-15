import pytest
from pydantic import ValidationError

from jobs.contracts import Dispatch, Lease


def test_message_rejects_secrets_and_invalid_epoch():
    with pytest.raises(ValidationError):
        Dispatch(job_id="not-a-uuid", epoch=0, cookie="secret")


def test_lease_only_accepts_registered_job_kinds():
    lease = Lease(job_id="00000000-0000-0000-0000-000000000001", epoch=1, fencing_token=1)
    assert lease.kind == "verify_pipeline"
    with pytest.raises(ValidationError):
        Lease(
            job_id="00000000-0000-0000-0000-000000000001",
            epoch=1,
            fencing_token=1,
            kind="arbitrary_function",
        )
