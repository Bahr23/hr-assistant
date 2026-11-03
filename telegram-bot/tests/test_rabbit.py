import json
import unittest
from uuid import uuid4

from src.delivery import InvalidDelivery, build_keyboard, parse_delivery
from src.rabbit import shard_for


class RabbitRoutingTests(unittest.TestCase):
    def test_chat_always_uses_same_shard(self) -> None:
        self.assertEqual(5, shard_for(13, 8))
        self.assertEqual(shard_for(987654321, 8), shard_for(987654321, 8))

    def test_parses_valid_telegram_delivery(self) -> None:
        message_id = uuid4()
        correlation_id = uuid4()
        delivery_id = uuid4()
        chat_id = 13
        body = json.dumps(
            {
                "message_id": str(message_id),
                "type": "telegram.question.send.v1",
                "schema_version": 1,
                "occurred_at": "2026-08-31T12:00:00Z",
                "correlation_id": str(correlation_id),
                "partition_key": chat_id,
                "payload": {
                    "delivery_id": str(delivery_id),
                    "chat_id": chat_id,
                    "text": "Вопрос",
                    "answer_type": "text",
                    "question_number": 1,
                    "question_total": 1,
                },
            }
        ).encode()

        _, payload = parse_delivery(body, shard=5, shard_count=8)

        self.assertEqual(chat_id, payload["chat_id"])
        self.assertEqual("Вопрос", payload["text"])
        self.assertNotIn("rich_html", payload)

    def test_rejects_delivery_for_another_shard(self) -> None:
        body = json.dumps(
            {
                "message_id": str(uuid4()),
                "type": "telegram.system.send.v1",
                "schema_version": 1,
                "correlation_id": str(uuid4()),
                "partition_key": 13,
                "payload": {
                    "delivery_id": str(uuid4()),
                    "chat_id": 13,
                    "text": "test",
                },
            }
        ).encode()

        with self.assertRaises(InvalidDelivery):
            parse_delivery(body, shard=4, shard_count=8)

    def test_builds_inline_keyboard_from_delivery(self) -> None:
        keyboard = build_keyboard(
            [[{"text": "Продолжить", "callback_data": "interview:continue"}]]
        )

        self.assertIsNotNone(keyboard)
        assert keyboard is not None
        self.assertEqual(
            "interview:continue",
            keyboard.inline_keyboard[0][0].callback_data,
        )


if __name__ == "__main__":
    unittest.main()
