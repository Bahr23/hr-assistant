from __future__ import annotations

import json
from datetime import UTC, datetime
from typing import Any
from uuid import UUID, uuid4

import aio_pika
from aio_pika import DeliveryMode, ExchangeType, Message

CANDIDATE_EXCHANGE = "candidate.commands"
TELEGRAM_EXCHANGE = "telegram.commands"
INTERVIEW_EXCHANGE = "interview.events"
DEAD_LETTER_EXCHANGE = "dead.letters"


def shard_for(chat_id: int, shard_count: int) -> int:
    return chat_id % shard_count


class RabbitBroker:
    def __init__(self, url: str, shard_count: int) -> None:
        self._url = url
        self.shard_count = shard_count
        self.connection: aio_pika.abc.AbstractRobustConnection | None = None
        self._publisher_channel: aio_pika.abc.AbstractRobustChannel | None = None
        self._candidate_exchange: aio_pika.abc.AbstractRobustExchange | None = None

    async def connect(self) -> None:
        self.connection = await aio_pika.connect_robust(self._url)
        topology_channel = await self.connection.channel()
        try:
            exchanges = await self._declare_exchanges(topology_channel)
            for shard in range(self.shard_count):
                await self._declare_sharded_queue(
                    topology_channel,
                    exchange=exchanges[CANDIDATE_EXCHANGE],
                    queue_prefix="candidate.updates",
                    routing_prefix="candidate",
                    shard=shard,
                )
                await self._declare_sharded_queue(
                    topology_channel,
                    exchange=exchanges[TELEGRAM_EXCHANGE],
                    queue_prefix="telegram.delivery",
                    routing_prefix="telegram",
                    shard=shard,
                )
        finally:
            await topology_channel.close()

        self._publisher_channel = await self.connection.channel(
            publisher_confirms=True,
        )
        self._candidate_exchange = await self._publisher_channel.declare_exchange(
            CANDIDATE_EXCHANGE,
            ExchangeType.TOPIC,
            durable=True,
        )

    async def close(self) -> None:
        if self.connection is not None:
            await self.connection.close()

    async def publish_candidate(
        self,
        message_type: str,
        chat_id: int,
        payload: dict[str, Any],
        *,
        message_id: UUID | None = None,
        correlation_id: UUID | None = None,
    ) -> UUID:
        if self._candidate_exchange is None:
            raise RuntimeError("RabbitMQ broker is not connected")
        message_id = message_id or uuid4()
        correlation_id = correlation_id or message_id
        shard = shard_for(chat_id, self.shard_count)
        routing_suffix = message_type.removeprefix("candidate.")
        envelope = {
            "message_id": str(message_id),
            "type": message_type,
            "schema_version": 1,
            "occurred_at": datetime.now(UTC).isoformat().replace("+00:00", "Z"),
            "correlation_id": str(correlation_id),
            "partition_key": chat_id,
            "payload": payload,
        }
        await self._candidate_exchange.publish(
            Message(
                body=json.dumps(envelope, ensure_ascii=False).encode(),
                content_type="application/json",
                delivery_mode=DeliveryMode.PERSISTENT,
                message_id=str(message_id),
                correlation_id=str(correlation_id),
                timestamp=datetime.now(UTC),
            ),
            routing_key=f"candidate.{shard}.{routing_suffix}",
        )
        return message_id

    async def open_telegram_queue(
        self,
        shard: int,
    ) -> tuple[
        aio_pika.abc.AbstractRobustChannel,
        aio_pika.abc.AbstractRobustQueue,
    ]:
        if self.connection is None:
            raise RuntimeError("RabbitMQ broker is not connected")
        channel = await self.connection.channel()
        await channel.set_qos(prefetch_count=1)
        exchange = await channel.declare_exchange(
            TELEGRAM_EXCHANGE,
            ExchangeType.TOPIC,
            durable=True,
        )
        queue = await self._declare_sharded_queue(
            channel,
            exchange=exchange,
            queue_prefix="telegram.delivery",
            routing_prefix="telegram",
            shard=shard,
        )
        return channel, queue

    async def _declare_exchanges(
        self,
        channel: aio_pika.abc.AbstractChannel,
    ) -> dict[str, aio_pika.abc.AbstractExchange]:
        result: dict[str, aio_pika.abc.AbstractExchange] = {}
        for name in (
            CANDIDATE_EXCHANGE,
            TELEGRAM_EXCHANGE,
            INTERVIEW_EXCHANGE,
            DEAD_LETTER_EXCHANGE,
        ):
            result[name] = await channel.declare_exchange(
                name,
                ExchangeType.TOPIC,
                durable=True,
            )
        return result

    async def _declare_sharded_queue(
        self,
        channel: aio_pika.abc.AbstractChannel,
        *,
        exchange: aio_pika.abc.AbstractExchange,
        queue_prefix: str,
        routing_prefix: str,
        shard: int,
    ) -> aio_pika.abc.AbstractQueue:
        dead_letter_exchange = await channel.declare_exchange(
            DEAD_LETTER_EXCHANGE,
            ExchangeType.TOPIC,
            durable=True,
        )
        dead_letter_routing_key = f"{routing_prefix}.{shard}"
        queue = await channel.declare_queue(
            f"{queue_prefix}.q.{shard}",
            durable=True,
            arguments={
                "x-queue-type": "quorum",
                "x-single-active-consumer": True,
                "x-dead-letter-exchange": DEAD_LETTER_EXCHANGE,
                "x-dead-letter-routing-key": dead_letter_routing_key,
            },
        )
        await queue.bind(exchange, routing_key=f"{routing_prefix}.{shard}.#")
        dead_letter_queue = await channel.declare_queue(
            f"{queue_prefix}.dlq.{shard}",
            durable=True,
            arguments={"x-queue-type": "quorum"},
        )
        await dead_letter_queue.bind(
            dead_letter_exchange,
            routing_key=dead_letter_routing_key,
        )
        return queue
