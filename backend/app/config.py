from functools import lru_cache

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", env_file_encoding="utf-8", extra="ignore")

    postgres_url: str = "postgresql+psycopg://matrix:matrix@localhost:5432/matrix"

    oracle_user: str = ""
    oracle_password: str = ""
    oracle_dsn: str = ""

    import_batch_size: int = 5000

    cors_origins: list[str] = ["http://localhost:5173"]


@lru_cache
def get_settings() -> Settings:
    return Settings()
