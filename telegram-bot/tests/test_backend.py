import unittest

from src.backend import VacancyBackend, VacancyNotFound


class FakeResponse:
    def __init__(self, status: int, payload: dict | None = None) -> None:
        self.status = status
        self._payload = payload or {}

    async def __aenter__(self):
        return self

    async def __aexit__(self, *_args):
        return None

    async def json(self) -> dict:
        return self._payload


class FakeSession:
    def __init__(self, response: FakeResponse) -> None:
        self.response = response
        self.url = ""
        self.headers: dict[str, str] = {}

    def get(self, url: str, headers: dict[str, str]) -> FakeResponse:
        self.url = url
        self.headers = headers
        return self.response


class VacancyBackendTests(unittest.IsolatedAsyncioTestCase):
    async def test_sends_service_token_and_returns_welcome_message(self) -> None:
        session = FakeSession(FakeResponse(200, {
            "welcome_message": "Добро пожаловать!",
            "already_attempted": False,
        }))
        backend = VacancyBackend("http://backend/api/v1/", "service-secret", session)  # type: ignore[arg-type]

        entry = await backend.get_entry("deep-link_token", 13)

        self.assertEqual("Добро пожаловать!", entry.welcome_message)
        self.assertFalse(entry.already_attempted)
        self.assertEqual(
            "http://backend/api/v1/vacancies/public/deep-link_token?telegram_user_id=13",
            session.url,
        )
        self.assertEqual("Bearer service-secret", session.headers["Authorization"])

    async def test_maps_missing_vacancy(self) -> None:
        backend = VacancyBackend(
            "http://backend/api/v1",
            "service-secret",
            FakeSession(FakeResponse(404)),  # type: ignore[arg-type]
        )

        with self.assertRaises(VacancyNotFound):
            await backend.get_entry("missing", 13)


if __name__ == "__main__":
    unittest.main()
