import asyncio
import logging
from uuid import NAMESPACE_URL, UUID, uuid5

from aiogram import F, Router
from aiogram.exceptions import TelegramBadRequest, TelegramRetryAfter
from aiogram.filters import Command, CommandObject, CommandStart
from aiogram.types import CallbackQuery, Message

from src.backend import BackendUnavailable, VacancyBackend, VacancyNotFound
from src.candidate import candidate_identity
from src.keyboards import START_INTERVIEW_CALLBACK, start_interview_keyboard
from src.rabbit import RabbitBroker
from src.sender import TelegramSender

router = Router()
router.message.filter(F.chat.type == "private")
router.callback_query.filter(F.message.chat.type == "private")
logger = logging.getLogger(__name__)


@router.message(CommandStart())
async def start_command(
    message: Message,
    command: CommandObject,
    vacancies: VacancyBackend,
    sender: TelegramSender,
) -> None:
    public_token = (command.args or "").strip()
    if not public_token:
        await sender.send_message(
            message.chat.id,
            "Откройте ссылку конкретной вакансии, чтобы начать.",
        )
        return

    try:
        entry = await vacancies.get_entry(public_token, message.chat.id)
    except VacancyNotFound:
        await sender.send_message(
            message.chat.id,
            "Вакансия не найдена или больше не активна.",
        )
        return
    except BackendUnavailable:
        logger.exception("Failed to load vacancy from backend")
        await sender.send_message(
            message.chat.id,
            "Не удалось загрузить вакансию. Попробуйте ещё раз позже.",
        )
        return

    if entry.already_attempted:
        await sender.send_message(
            message.chat.id,
            "Вы уже проходили интервью по этой вакансии. "
            "Повторное прохождение недоступно.",
        )
        return

    chunks = [
        text[offset : offset + 4096]
        for text in (entry.welcome_message, entry.description or "")
        for offset in range(0, len(text), 4096)
    ]
    for index, chunk in enumerate(chunks):
        reply_markup = (
            start_interview_keyboard(public_token)
            if index == len(chunks) - 1
            else None
        )
        await sender.send_message(
            message.chat.id,
            chunk,
            reply_markup=reply_markup,
        )


@router.callback_query(F.data.startswith(f"{START_INTERVIEW_CALLBACK}:"))
async def start_interview(
    callback: CallbackQuery,
    publisher: RabbitBroker,
    sender: TelegramSender,
) -> None:
    await answer_callback(callback)
    public_token = (callback.data or "").removeprefix(
        f"{START_INTERVIEW_CALLBACK}:"
    )
    published = await publish_start_action(
        callback,
        publisher,
        sender,
        "candidate.start.requested.v1",
        public_token,
    )
    if published:
        await remove_inline_keyboard(callback)


@router.callback_query(F.data.startswith("interview:switch:"))
async def switch_interview(
    callback: CallbackQuery,
    publisher: RabbitBroker,
    sender: TelegramSender,
) -> None:
    await answer_callback(callback)
    public_token = (callback.data or "").removeprefix("interview:switch:")
    await publish_start_action(
        callback,
        publisher,
        sender,
        "candidate.switch.requested.v1",
        public_token,
    )


@router.callback_query(F.data == "interview:continue")
async def continue_interview(
    callback: CallbackQuery,
    publisher: RabbitBroker,
    sender: TelegramSender,
) -> None:
    await answer_callback(callback)
    identity = callback_identity(callback)
    if identity is None:
        return
    chat_id = identity["telegram_chat_id"]
    assert isinstance(chat_id, int)
    await publish_or_report(
        publisher,
        sender,
        message_type="candidate.continue.requested.v1",
        chat_id=chat_id,
        payload=identity,
        event_key=f"callback:{callback.id}:continue",
    )


@router.message(Command("cancel"))
async def cancel_interview(
    message: Message,
    publisher: RabbitBroker,
    sender: TelegramSender,
) -> None:
    if message.from_user is None:
        return
    await publish_or_report(
        publisher,
        sender,
        message_type="candidate.cancel.requested.v1",
        chat_id=message.chat.id,
        payload=candidate_identity(message.from_user, message.chat),
        event_key=f"message:{message.chat.id}:{message.message_id}:cancel",
    )


@router.message(F.voice)
async def voice_answer(
    message: Message,
    publisher: RabbitBroker,
    sender: TelegramSender,
) -> None:
    if message.from_user is None or message.voice is None:
        return
    await publish_or_report(
        publisher,
        sender,
        message_type="candidate.answer.received.v1",
        chat_id=message.chat.id,
        payload={
            **candidate_identity(message.from_user, message.chat),
            "telegram_message_id": message.message_id,
            "answer_type": "voice",
            "voice_file_id": message.voice.file_id,
            "voice_mime_type": message.voice.mime_type or "audio/ogg",
            "voice_file_size": message.voice.file_size or 0,
            "voice_duration_seconds": message.voice.duration,
        },
        event_key=f"message:{message.chat.id}:{message.message_id}:answer",
    )


@router.message(F.text)
async def text_answer(
    message: Message,
    publisher: RabbitBroker,
    sender: TelegramSender,
) -> None:
    if (
        message.from_user is None
        or message.text is None
        or message.text.startswith("/")
    ):
        return
    await publish_or_report(
        publisher,
        sender,
        message_type="candidate.answer.received.v1",
        chat_id=message.chat.id,
        payload={
            **candidate_identity(message.from_user, message.chat),
            "telegram_message_id": message.message_id,
            "answer_type": "text",
            "text": message.text,
        },
        event_key=f"message:{message.chat.id}:{message.message_id}:answer",
    )


async def publish_start_action(
    callback: CallbackQuery,
    publisher: RabbitBroker,
    sender: TelegramSender,
    message_type: str,
    public_token: str,
) -> bool:
    identity = callback_identity(callback)
    if identity is None:
        return False
    chat_id = identity["telegram_chat_id"]
    assert isinstance(chat_id, int)
    return await publish_or_report(
        publisher,
        sender,
        message_type=message_type,
        chat_id=chat_id,
        payload={
            **identity,
            "public_token": public_token,
        },
        event_key=f"callback:{callback.id}:{message_type}",
    )


async def publish_or_report(
    publisher: RabbitBroker,
    sender: TelegramSender,
    *,
    message_type: str,
    chat_id: int,
    payload: dict[str, object],
    event_key: str,
) -> bool:
    message_id = deterministic_message_id(event_key)
    try:
        await publisher.publish_candidate(
            message_type,
            chat_id,
            payload,
            message_id=message_id,
            correlation_id=message_id,
        )
        return True
    except Exception:
        logger.exception("Failed to publish candidate command", extra={"type": message_type})
        try:
            await sender.send_message(
                chat_id,
                "Не удалось обработать сообщение. Пожалуйста, отправьте его ещё раз.",
            )
        except Exception:
            logger.exception("Failed to report RabbitMQ error to candidate")
        return False


async def answer_callback(callback: CallbackQuery) -> None:
    while True:
        try:
            await callback.answer()
            return
        except TelegramRetryAfter as error:
            await asyncio.sleep(error.retry_after + 0.1)


async def remove_inline_keyboard(callback: CallbackQuery) -> None:
    if not isinstance(callback.message, Message):
        return
    while True:
        try:
            await callback.message.edit_reply_markup(reply_markup=None)
            return
        except TelegramRetryAfter as error:
            await asyncio.sleep(error.retry_after + 0.1)
        except TelegramBadRequest:
            logger.warning("Failed to remove start keyboard", exc_info=True)
            return


def callback_identity(callback: CallbackQuery) -> dict[str, object] | None:
    chat = getattr(callback.message, "chat", None)
    if chat is None:
        return None
    return candidate_identity(callback.from_user, chat)


def deterministic_message_id(event_key: str) -> UUID:
    return uuid5(NAMESPACE_URL, f"hr-assistant:{event_key}")
