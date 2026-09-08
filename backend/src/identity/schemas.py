from pydantic import BaseModel, Field, SecretStr

from core.schemas import Input


class LoginInput(Input):
    username: str = Field(min_length=3, max_length=50, pattern=r"^[a-zA-Z0-9_.-]+$")
    password: SecretStr = Field(min_length=12, max_length=128)


class Principal(BaseModel):
    username: str
