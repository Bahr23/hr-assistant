import unittest

from src.custom_emoji import CustomEmoji
from src.keyboards import START_INTERVIEW_CALLBACK, start_interview_keyboard


class StartInterviewKeyboardTests(unittest.TestCase):
    def test_start_button_uses_tick_custom_emoji(self) -> None:
        keyboard = start_interview_keyboard("public_token")
        button = keyboard.inline_keyboard[0][0]

        self.assertEqual("Начать", button.text)
        self.assertEqual(CustomEmoji.TICK.value, button.icon_custom_emoji_id)
        self.assertEqual(
            f"{START_INTERVIEW_CALLBACK}:public_token",
            button.callback_data,
        )


if __name__ == "__main__":
    unittest.main()
