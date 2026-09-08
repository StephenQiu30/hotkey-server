import pytest
from pydantic import ValidationError

from jobs.contracts import Dispatch


def test_message_rejects_secrets_and_invalid_epoch():
    with pytest.raises(ValidationError):
        Dispatch(job_id="not-a-uuid", epoch=0, cookie="secret")
