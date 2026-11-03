from aiogram.types import InlineKeyboardButton, InlineKeyboardMarkup

from src.custom_emoji import CustomEmoji

START_INTERVIEW_CALLBACK = "interview:start"


def start_interview_keyboard(public_token: str) -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(
        inline_keyboard=[
            [
                InlineKeyboardButton(
                    text="Начать",
                    icon_custom_emoji_id=CustomEmoji.TICK.value,
                    callback_data=f"{START_INTERVIEW_CALLBACK}:{public_token}",
                )
            ]
        ]
    )
