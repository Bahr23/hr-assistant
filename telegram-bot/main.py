import asyncio
import logging

from aiohttp import ClientSession, ClientTimeout
from aiogram import Bot, Dispatcher

from settings import settings
from src.backend import VacancyBackend
from src.delivery import TelegramDeliveryConsumer
from src.handlers import router
from src.rabbit import RabbitBroker
from src.sender import TelegramSender


async def main() -> None:
    logging.basicConfig(level=logging.INFO)
    dispatcher = Dispatcher()
    dispatcher.include_router(router)

    timeout = ClientTimeout(total=settings.backend_timeout_seconds)
    broker = RabbitBroker(settings.rabbitmq_url, settings.rabbitmq_shards)
    await broker.connect()
    try:
        async with ClientSession(timeout=timeout) as http, Bot(
            token=settings.bot_token.get_secret_value()
        ) as bot:
            sender = TelegramSender(
                bot,
                shard_count=settings.rabbitmq_shards,
                shard_messages_per_second=(
                    settings.telegram_shard_messages_per_second
                ),
                chat_messages_per_second=(
                    settings.telegram_chat_messages_per_second
                ),
                max_retries=settings.telegram_send_max_retries,
            )
            dispatcher["vacancies"] = VacancyBackend(
                base_url=settings.backend_api_url,
                api_token=settings.master_api_token.get_secret_value(),
                http=http,
            )
            dispatcher["publisher"] = broker
            dispatcher["sender"] = sender

            polling = asyncio.create_task(
                dispatcher.start_polling(
                    bot,
                    allowed_updates=dispatcher.resolve_used_update_types(),
                )
            )
            deliveries = asyncio.create_task(
                TelegramDeliveryConsumer(broker, sender).run()
            )
            tasks = {polling, deliveries}
            try:
                done, _ = await asyncio.wait(
                    tasks,
                    return_when=asyncio.FIRST_COMPLETED,
                )
                for task in done:
                    task.result()
            finally:
                for task in tasks:
                    task.cancel()
                await asyncio.gather(*tasks, return_exceptions=True)
    finally:
        await broker.close()


if __name__ == "__main__":
    asyncio.run(main())
