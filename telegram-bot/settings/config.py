from pydantic import Field, SecretStr
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    bot_token: SecretStr = Field(alias="BOT_TOKEN")
    master_api_token: SecretStr = Field(alias="MASTER_API_TOKEN")
    backend_api_url: str = Field(
        default="http://localhost:8080/api/v1",
        alias="BACKEND_API_URL",
    )
    backend_timeout_seconds: float = Field(
        default=5.0,
        alias="BACKEND_TIMEOUT_SECONDS",
        gt=0,
    )
    rabbitmq_url: str = Field(alias="RABBITMQ_URL")
    rabbitmq_shards: int = Field(
        default=8,
        alias="RABBITMQ_SHARDS",
        ge=8,
        le=8,
    )
    telegram_shard_messages_per_second: float = Field(
        default=2.0,
        alias="TELEGRAM_SHARD_MESSAGES_PER_SECOND",
        gt=0,
        le=2,
    )
    telegram_chat_messages_per_second: float = Field(
        default=1.0,
        alias="TELEGRAM_CHAT_MESSAGES_PER_SECOND",
        gt=0,
        le=1,
    )
    telegram_send_max_retries: int = Field(
        default=8,
        alias="TELEGRAM_SEND_MAX_RETRIES",
        ge=1,
    )

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
    )


settings = Settings()  # type: ignore[call-arg]
