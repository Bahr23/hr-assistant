from aiogram.types import Chat, User


def candidate_identity(user: User, chat: Chat) -> dict[str, object]:
    identity: dict[str, object] = {
        "telegram_user_id": user.id,
        "telegram_chat_id": chat.id,
        "first_name": user.first_name,
        "is_premium": bool(user.is_premium),
    }
    if user.username is not None:
        identity["username"] = user.username
    if user.last_name is not None:
        identity["last_name"] = user.last_name
    if user.language_code is not None:
        identity["language_code"] = user.language_code
    return identity
