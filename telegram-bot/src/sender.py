from __future__ import annotations

import asyncio
import random
from collections import defaultdict
from collections.abc import Awaitable, Callable

from aiogram import Bot
from aiogram.exceptions import (
    TelegramNetworkError,
    TelegramRetryAfter,
    TelegramServerError,
)
from aiogram.types import InlineKeyboardMarkup, InputRichMessage, Message

from src.rabbit import shard_for


class TelegramSender:
    def __init__(
        self,
        bot: Bot,
        *,
        shard_count: int,
        shard_messages_per_second: float,
        chat_messages_per_second: float,
        max_retries: int,
    ) -> None:
        self._bot = bot
        self._shard_count = shard_count
        self._shard_interval = 1 / shard_messages_per_second
        self._chat_interval = 1 / chat_messages_per_second
        self._max_retries = max_retries
        self._shard_locks = [asyncio.Lock() for _ in range(shard_count)]
        self._chat_locks: defaultdict[int, asyncio.Lock] = defaultdict(asyncio.Lock)
        self._next_shard_send = [0.0] * shard_count
        self._next_chat_send: dict[int, float] = {}

    async def send_message(
        self,
        chat_id: int,
        text: str,
        *,
        reply_markup: InlineKeyboardMarkup | None = None,
    ) -> Message:
        return await self._send(
            chat_id,
            lambda: self._bot.send_message(
                chat_id=chat_id,
                text=text,
                reply_markup=reply_markup,
            ),
        )

    async def send_rich_message(
        self,
        chat_id: int,
        html: str,
        *,
        reply_markup: InlineKeyboardMarkup | None = None,
    ) -> Message:
        return await self._send(
            chat_id,
            lambda: self._bot.send_rich_message(
                chat_id=chat_id,
                rich_message=InputRichMessage(html=html),
                reply_markup=reply_markup,
            ),
        )

    async def _send(
        self,
        chat_id: int,
        operation: Callable[[], Awaitable[Message]],
    ) -> Message:
        shard = shard_for(chat_id, self._shard_count)
        async with self._shard_locks[shard]:
            async with self._chat_locks[chat_id]:
                transient_attempt = 0
                while True:
                    await self._wait_for_slot(shard, chat_id)
                    try:
                        return await operation()
                    except TelegramRetryAfter as error:
                        await asyncio.sleep(error.retry_after + random.uniform(0.1, 0.3))
                    except (TelegramNetworkError, TelegramServerError):
                        transient_attempt += 1
                        if transient_attempt >= self._max_retries:
                            raise
                        await asyncio.sleep(min(2 ** (transient_attempt - 1), 30))

    async def _wait_for_slot(self, shard: int, chat_id: int) -> None:
        loop = asyncio.get_running_loop()
        now = loop.time()
        next_allowed = max(
            self._next_shard_send[shard],
            self._next_chat_send.get(chat_id, 0.0),
        )
        if next_allowed > now:
            await asyncio.sleep(next_allowed - now)
        sent_at = loop.time()
        self._next_shard_send[shard] = sent_at + self._shard_interval
        self._next_chat_send[chat_id] = sent_at + self._chat_interval
