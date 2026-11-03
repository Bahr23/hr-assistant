from dataclasses import dataclass
from urllib.parse import quote

from aiohttp import ClientError, ClientSession


class VacancyNotFound(Exception):
    pass


class BackendUnavailable(Exception):
    pass


@dataclass(frozen=True)
class VacancyEntry:
    welcome_message: str
    already_attempted: bool
    description: str | None = None


class VacancyBackend:
    def __init__(self, base_url: str, api_token: str, http: ClientSession) -> None:
        self._base_url = base_url.rstrip("/")
        self._api_token = api_token
        self._http = http

    async def get_entry(
        self,
        public_token: str,
        telegram_user_id: int,
    ) -> VacancyEntry:
        token = quote(public_token, safe="")
        url = (
            f"{self._base_url}/vacancies/public/{token}"
            f"?telegram_user_id={telegram_user_id}"
        )
        try:
            async with self._http.get(
                url,
                headers={"Authorization": f"Bearer {self._api_token}"},
            ) as response:
                if response.status == 404:
                    raise VacancyNotFound
                if response.status != 200:
                    raise BackendUnavailable(f"backend returned HTTP {response.status}")
                payload = await response.json()
        except VacancyNotFound:
            raise
        except (ClientError, TimeoutError, ValueError) as error:
            raise BackendUnavailable("backend request failed") from error

        description = payload.get("description")
        welcome_message = payload.get("welcome_message")
        already_attempted = payload.get("already_attempted")
        if (
            not isinstance(welcome_message, str)
            or not welcome_message
            or not isinstance(already_attempted, bool)
            or (description is not None and not isinstance(description, str))
        ):
            raise BackendUnavailable("backend returned an invalid response")
        return VacancyEntry(
            welcome_message=welcome_message,
            already_attempted=already_attempted,
            description=description,
        )
