import unittest
from datetime import datetime, timezone
from types import SimpleNamespace
from unittest.mock import AsyncMock, patch

from aiogram.types import CallbackQuery, Chat, Message, User, Voice

from src.backend import VacancyEntry
from src.handlers.start import start_command, start_interview, voice_answer


def make_callback() -> CallbackQuery:
    user = User(id=13, is_bot=False, first_name="Candidate")
    message = Message(
        message_id=42,
        date=datetime.now(timezone.utc),
        chat=Chat(id=13, type="private"),
        from_user=user,
        text="Welcome",
    )
    return CallbackQuery(
        id="callback-id",
        from_user=user,
        chat_instance="chat-instance",
        message=message,
        data="interview:start:vacancy-token",
    )


class StartHandlerTests(unittest.IsolatedAsyncioTestCase):
    async def test_full_description_precedes_start_button(self) -> None:
        message = make_callback().message
        entry = VacancyEntry(welcome_message="Приветствие", already_attempted=False, description="я" * 10000)
        vacancies = SimpleNamespace(get_entry=AsyncMock(return_value=entry))
        sender = SimpleNamespace(send_message=AsyncMock())
        await start_command(message, SimpleNamespace(args="token"), vacancies, sender)
        calls = sender.send_message.await_args_list
        self.assertEqual(calls[0].args[1], entry.welcome_message)
        self.assertEqual("".join(call.args[1] for call in calls[1:]), entry.description)
        self.assertTrue(all(len(call.args[1]) <= 4096 for call in calls))
        self.assertTrue(all(call.kwargs["reply_markup"] is None for call in calls[:-1]))
        self.assertIsNotNone(calls[-1].kwargs["reply_markup"])

    async def test_repeated_attempt_is_rejected_before_welcome_message(self) -> None:
        user = User(id=13, is_bot=False, first_name="Candidate")
        message = Message(
            message_id=41,
            date=datetime.now(timezone.utc),
            chat=Chat(id=13, type="private"),
            from_user=user,
            text="/start vacancy-token",
        )
        command = SimpleNamespace(args="vacancy-token")
        vacancies = SimpleNamespace(get_entry=AsyncMock(return_value=VacancyEntry(
            welcome_message="Welcome",
            already_attempted=True,
        )))
        sender = SimpleNamespace(send_message=AsyncMock())

        await start_command(message, command, vacancies, sender)

        vacancies.get_entry.assert_awaited_once_with("vacancy-token", 13)
        sender.send_message.assert_awaited_once_with(
            13,
            "Вы уже проходили интервью по этой вакансии. "
            "Повторное прохождение недоступно.",
        )

    async def test_removes_start_button_after_command_is_published(self) -> None:
        callback = make_callback()
        publisher = SimpleNamespace(publish_candidate=AsyncMock())
        sender = SimpleNamespace(send_message=AsyncMock())

        with (
            patch.object(CallbackQuery, "answer", new=AsyncMock()) as answer,
            patch.object(Message, "edit_reply_markup", new=AsyncMock()) as edit,
        ):
            await start_interview(callback, publisher, sender)

        answer.assert_awaited_once()
        publisher.publish_candidate.assert_awaited_once()
        payload = publisher.publish_candidate.await_args.args[2]
        self.assertEqual("Candidate", payload["first_name"])
        self.assertNotIn("telegram_user", payload)
        self.assertNotIn("telegram_chat", payload)
        edit.assert_awaited_once_with(reply_markup=None)

    async def test_keeps_start_button_when_publish_fails(self) -> None:
        callback = make_callback()
        publisher = SimpleNamespace(
            publish_candidate=AsyncMock(side_effect=RuntimeError("RabbitMQ unavailable"))
        )
        sender = SimpleNamespace(send_message=AsyncMock())

        with (
            patch.object(CallbackQuery, "answer", new=AsyncMock()),
            patch.object(Message, "edit_reply_markup", new=AsyncMock()) as edit,
        ):
            await start_interview(callback, publisher, sender)

        edit.assert_not_awaited()
        sender.send_message.assert_awaited_once()

    async def test_voice_answer_publishes_telegram_file_metadata(self) -> None:
        user = User(id=13, is_bot=False, first_name="Candidate", language_code="ru")
        message = Message(
            message_id=44,
            date=datetime.now(timezone.utc),
            chat=Chat(id=13, type="private"),
            from_user=user,
            voice=Voice(
                file_id="telegram-file-id",
                file_unique_id="telegram-unique-id",
                duration=17,
                mime_type="audio/ogg",
                file_size=12345,
            ),
        )
        publisher = SimpleNamespace(publish_candidate=AsyncMock())
        sender = SimpleNamespace(send_message=AsyncMock())

        await voice_answer(message, publisher, sender)

        publisher.publish_candidate.assert_awaited_once()
        payload = publisher.publish_candidate.await_args.args[2]
        self.assertEqual("telegram-file-id", payload["voice_file_id"])
        self.assertEqual("audio/ogg", payload["voice_mime_type"])
        self.assertEqual(12345, payload["voice_file_size"])
        self.assertEqual(17, payload["voice_duration_seconds"])


if __name__ == "__main__":
    unittest.main()
