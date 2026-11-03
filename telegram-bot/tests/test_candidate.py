import unittest

from aiogram.types import Chat, User

from src.candidate import candidate_identity


class CandidateIdentityTests(unittest.TestCase):
    def test_serializes_only_selected_candidate_fields(self) -> None:
        identity = candidate_identity(
            User(
                id=13,
                is_bot=False,
                first_name="Candidate",
                last_name="Test",
                username="candidate",
                language_code="ru",
                is_premium=True,
            ),
            Chat(
                id=13,
                type="private",
                first_name="Candidate",
                last_name="Test",
                username="candidate",
            ),
        )

        self.assertEqual({
            "telegram_user_id": 13,
            "telegram_chat_id": 13,
            "username": "candidate",
            "first_name": "Candidate",
            "last_name": "Test",
            "is_premium": True,
            "language_code": "ru",
        }, identity)


if __name__ == "__main__":
    unittest.main()
