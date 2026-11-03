from __future__ import annotations

import asyncio
import json
import logging
from typing import Any
from uuid import UUID, uuid4

from aiogram.exceptions import (
    TelegramBadRequest,
    TelegramForbiddenError,
    TelegramNetworkError,
    TelegramServerError,
)
from aiogram.types import InlineKeyboardButton, InlineKeyboardMarkup

from src.rabbit import RabbitBroker, shard_for
from src.rich_messages import question_rich_html
from src.sender import TelegramSender

logger = logging.getLogger(__name__)


class InvalidDelivery(Exception):
    pass


class TelegramDeliveryConsumer:
    def __init__(self, broker: RabbitBroker, sender: TelegramSender) -> None:
        self._broker = broker
        self._sender = sender

    async def run(self) -> None:
        await asyncio.gather(
            *(self._consume_shard(shard) for shard in range(self._broker.shard_count))
        )

    async def _consume_shard(self, shard: int) -> None:
        channel, queue = await self._broker.open_telegram_queue(shard)
        try:
            async with queue.iterator() as messages:
                async for message in messages:
                    try:
                        envelope, payload = parse_delivery(
                            message.body,
                            shard=shard,
                            shard_count=self._broker.shard_count,
                        )
                    except InvalidDelivery as error:
                        logger.error(
                            "Telegram delivery moved to DLQ",
                            extra={"shard": shard, "error": str(error)},
                        )
                        await message.reject(requeue=False)
                        continue

                    delivery_id = UUID(payload["delivery_id"])
                    chat_id = payload["chat_id"]
                    try:
                        reply_markup = build_keyboard(payload.get("buttons"))
                        if envelope["type"] == "telegram.question.send.v1":
                            sent = await self._sender.send_rich_message(
                                chat_id,
                                question_rich_html(
                                    text=payload["text"],
                                    answer_type=payload["answer_type"],
                                    question_number=payload["question_number"],
                                    question_total=payload["question_total"],
                                ),
                                reply_markup=reply_markup,
                            )
                        else:
                            sent = await self._sender.send_message(
                                chat_id,
                                payload["text"],
                                reply_markup=reply_markup,
                            )
                    except (TelegramBadRequest, TelegramForbiddenError) as error:
                        await self._publish_failed(
                            chat_id,
                            delivery_id,
                            code=type(error).__name__,
                            message=str(error),
                            correlation_id=UUID(envelope["correlation_id"]),
                        )
                        await message.ack()
                        continue
                    except (TelegramNetworkError, TelegramServerError) as error:
                        delivery_count = int(message.headers.get("x-delivery-count", 0))
                        if delivery_count >= 2:
                            await self._publish_failed(
                                chat_id,
                                delivery_id,
                                code="transient_retries_exhausted",
                                message=str(error),
                                correlation_id=UUID(envelope["correlation_id"]),
                            )
                            await message.reject(requeue=False)
                        else:
                            await message.nack(requeue=True)
                        continue

                    await self._broker.publish_candidate(
                        "candidate.delivery.succeeded.v1",
                        chat_id,
                        {
                            "delivery_id": str(delivery_id),
                            "telegram_message_id": sent.message_id,
                        },
                        message_id=uuid4(),
                        correlation_id=UUID(envelope["correlation_id"]),
                    )
                    await message.ack()
        finally:
            await channel.close()

    async def _publish_failed(
        self,
        chat_id: int,
        delivery_id: UUID,
        *,
        code: str,
        message: str,
        correlation_id: UUID,
    ) -> None:
        await self._broker.publish_candidate(
            "candidate.delivery.failed.v1",
            chat_id,
            {
                "delivery_id": str(delivery_id),
                "code": code,
                "message": message[:1000],
            },
            correlation_id=correlation_id,
        )


def parse_delivery(
    body: bytes,
    *,
    shard: int,
    shard_count: int,
) -> tuple[dict[str, Any], dict[str, Any]]:
    try:
        envelope = json.loads(body)
        UUID(envelope["message_id"])
        UUID(envelope["correlation_id"])
        if envelope["schema_version"] != 1:
            raise InvalidDelivery("unsupported schema version")
        if envelope["type"] not in {
            "telegram.question.send.v1",
            "telegram.system.send.v1",
            "telegram.session_conflict.send.v1",
        }:
            raise InvalidDelivery("unsupported delivery type")
        partition_key = envelope["partition_key"]
        if not isinstance(partition_key, int):
            raise InvalidDelivery("invalid partition key")
        if shard_for(partition_key, shard_count) != shard:
            raise InvalidDelivery("delivery belongs to another shard")
        payload = envelope["payload"]
        UUID(payload["delivery_id"])
        if payload["chat_id"] != partition_key:
            raise InvalidDelivery("chat id differs from partition key")
        if not isinstance(payload["text"], str) or not payload["text"]:
            raise InvalidDelivery("empty delivery text")
        if "rich_html" in payload:
            raise InvalidDelivery("delivery must contain raw content, not rich HTML")
        if envelope["type"] == "telegram.question.send.v1":
            answer_type = payload["answer_type"]
            question_number = payload["question_number"]
            question_total = payload["question_total"]
            if answer_type not in {"text", "voice"}:
                raise InvalidDelivery("invalid question answer type")
            if (
                not isinstance(question_number, int)
                or isinstance(question_number, bool)
                or not isinstance(question_total, int)
                or isinstance(question_total, bool)
                or question_number < 1
                or question_total < question_number
            ):
                raise InvalidDelivery("invalid question counter")
        build_keyboard(payload.get("buttons"))
    except InvalidDelivery:
        raise
    except (KeyError, TypeError, ValueError, json.JSONDecodeError) as error:
        raise InvalidDelivery("invalid delivery JSON") from error
    return envelope, payload


def build_keyboard(buttons: Any) -> InlineKeyboardMarkup | None:
    if buttons is None or buttons == []:
        return None
    if not isinstance(buttons, list):
        raise InvalidDelivery("invalid inline buttons")
    rows: list[list[InlineKeyboardButton]] = []
    for row in buttons:
        if not isinstance(row, list) or not row:
            raise InvalidDelivery("invalid inline button row")
        parsed_row: list[InlineKeyboardButton] = []
        for button in row:
            if not isinstance(button, dict):
                raise InvalidDelivery("invalid inline button")
            text = button.get("text")
            callback_data = button.get("callback_data")
            if not isinstance(text, str) or not isinstance(callback_data, str):
                raise InvalidDelivery("invalid inline button fields")
            parsed_row.append(
                InlineKeyboardButton(text=text, callback_data=callback_data)
            )
        rows.append(parsed_row)
    return InlineKeyboardMarkup(inline_keyboard=rows)
