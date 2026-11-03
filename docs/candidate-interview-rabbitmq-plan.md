# Кандидатское интервью через RabbitMQ

## Цель и границы первой версии

Telegram-бот получает ответы кандидата и передаёт команды напрямую в RabbitMQ.
Backend остаётся владельцем состояния интервью и хранит его в PostgreSQL. RabbitMQ
используется только как транспорт: потеря или перезапуск брокера не должны менять
подтверждённое состояние интервью.

Поддерживаются текстовые и голосовые ответы. Для голоса сохраняются Telegram
`file_id`, MIME type, размер и длительность. После завершения интервью отдельный
`openrouter-worker` надёжно транскрибирует каждый голосовой ответ; дальнейший
AI-анализ и object storage пока не входят в версию.
Одновременно у Telegram-пользователя может быть только одно активное интервью.
Кандидатский flow работает только в приватном чате, где `telegram_chat_id` совпадает
с `telegram_user_id`.

## Поток данных

```text
Telegram update
      │
      ▼
aiogram bot ──publish──▶ candidate.commands
                              │ chat_id % 8
                  ┌───────────┼───────────┐
                  ▼           ▼           ▼
             shard q.0   ... shard q.7   DLQ
                  │ один active consumer на shard
                  ▼
          Go backend transaction
      inbox → session/answer → outbox
                              │
                              ▼
                       telegram.commands
                              │ chat_id % 8
                  ┌───────────┼───────────┐
                  ▼           ▼           ▼
             shard q.0   ... shard q.7   DLQ
                  │ 2 msg/s на shard, 1 msg/s на chat
                  ▼
             Telegram Bot API
                  │ success / failure event
                  └──────────▶ candidate.commands

completed transaction ──▶ interview.events ──▶ OpenRouter worker
                                                   │ Telegram getFile
                                                   │ OpenRouter STT
                                                   ▼
                         transcribed/failed ──▶ interview.events ──▶ backend
```

## Shards и порядок

Используются восемь фиксированных shards. Номер вычисляется как
`telegram_chat_id % 8`. Все команды одного приватного чата всегда попадают в одну
очередь и обрабатываются последовательно. Разные чаты могут обрабатываться
параллельно.

Очереди являются durable quorum queues и создаются с
`x-single-active-consumer=true`. При нескольких репликах только один consumer
обрабатывает конкретный shard; остальные остаются резервными. `prefetch=1` не даёт
consumer забрать следующую команду до подтверждения текущей.

Число shards зафиксировано в первой версии. Его изменение перераспределит чаты и
требует остановки publishers, опустошения старых очередей и контролируемой миграции.

## RabbitMQ topology

Durable topic exchanges:

- `candidate.commands` — команды от бота backend;
- `telegram.commands` — команды backend на отправку сообщений;
- `interview.events` — интеграционные события для будущих consumers;
- `dead.letters` — сообщения, которые нельзя обработать.

Для каждого `N` от `0` до `7`:

- `candidate.updates.q.N`, binding `candidate.N.#`;
- `telegram.delivery.q.N`, binding `telegram.N.#`;
- `candidate.updates.dlq.N` и `telegram.delivery.dlq.N`.

Основные routing keys:

- `candidate.<shard>.start.requested.v1`;
- `candidate.<shard>.switch.requested.v1`;
- `candidate.<shard>.continue.requested.v1`;
- `candidate.<shard>.cancel.requested.v1`;
- `candidate.<shard>.answer.received.v1`;
- `candidate.<shard>.delivery.succeeded.v1`;
- `candidate.<shard>.delivery.failed.v1`;
- `telegram.<shard>.question.send.v1`;
- `telegram.<shard>.system.send.v1`;
- `telegram.<shard>.session_conflict.send.v1`.

AI routing keys и очереди:

- `interview.completed.v1` → `openrouter.interview-completed.q`;
- `interview.voice.transcribed.v1` и
  `interview.voice.transcription_failed.v1` → `backend.ai-results.q`;
- обе очереди — durable quorum с single active consumer и отдельной DLQ.

Сообщение публикуется persistent, publisher ждёт RabbitMQ publisher confirm.

## Envelope

```json
{
  "message_id": "uuid",
  "type": "candidate.answer.received.v1",
  "schema_version": 1,
  "occurred_at": "2026-08-31T12:00:00Z",
  "correlation_id": "uuid",
  "partition_key": 123456789,
  "payload": {}
}
```

`message_id` используется для идемпотентности через `inbox_messages`.
`partition_key` равен `telegram_chat_id`. Неизвестная версия, сломанный JSON или
несовпадение shard отправляются в DLQ без бесконечного requeue.

## PostgreSQL

- `interview_sessions` — вакансия, Telegram user/chat, статус и текущая позиция;
- `interview_session_questions` — неизменяемый snapshot вопросов на момент старта;
- `interview_answers` — один ответ на одну позицию snapshot;
- `telegram_deliveries` — состояние исходящей доставки и Telegram message ID;
- `inbox_messages` — уже обработанные входящие `message_id`;
- `outbox_events` — события, созданные в одной транзакции с изменением домена.
- `interview_answer_transcriptions` — постоянный результат транскрибации ответа.

Partial unique index запрещает более одной строки со статусом `active` для одного
`telegram_user_id`.

Backend обрабатывает входящее сообщение так:

1. начинает PostgreSQL-транзакцию;
2. вставляет `message_id` в inbox; конфликт означает повтор и безопасный ACK;
3. блокирует активную сессию через `FOR UPDATE`;
4. изменяет сессию или сохраняет ответ;
5. добавляет следующую команду в outbox;
6. фиксирует транзакцию;
7. только после commit отправляет RabbitMQ `ACK`.

Outbox publisher отправляет события с publisher confirms и после подтверждения
записывает `published_at`. PostgreSQL advisory lock допускает только один active
outbox publisher на все реплики backend; остальные являются standby. Это не даёт
двум репликам опубликовать соседние события в обратном порядке. Возможен редкий повтор публикации при падении между
RabbitMQ confirm и записью `published_at`; consumers поэтому обязаны быть
идемпотентными.

## Сценарии интервью

Кнопка welcome message содержит callback `interview:start:<public_token>`.
При открытии deep link бот сначала запрашивает через REST API состояние пары
`public_token + telegram_user_id`. Если попытка уже закреплена в
`interview_attempt_claims`, бот сразу сообщает, что повторное прохождение
недоступно, и не отправляет welcome message с кнопкой.
После успешной публикации команды `candidate.start.requested.v1` бот удаляет
inline-клавиатуру у welcome message. Если RabbitMQ недоступен, кнопка остаётся для
повторной попытки.

При каждом callback или ответе бот передаёт Telegram user/chat ID и только поля
профиля `username`, `first_name`, `last_name`, `is_premium`, `language_code`.
Backend хранит их отдельными колонками в `telegram_candidates` и обновляет
`last_seen_at`; остальные поля Telegram не передаются и не сохраняются.

- Если активной сессии нет, backend проверяет вакансию, создаёт сессию и snapshot,
  затем ставит первый вопрос в outbox.
- Первая попытка атомарно закрепляется в `interview_attempt_claims` уникальным
  ключом `(vacancy_id, telegram_user_id)`. Повторный старт той же вакансии не
  создаёт сессию и показывает уведомление о текущей или уже использованной попытке.
- Старт другой вакансии показывает кнопки «Продолжить текущее» и «Отменить и
  начать новое». Переключение выполняется только после явного подтверждения.
- `/cancel` отменяет активную сессию.
- После корректного ответа backend сразу сохраняет его и ставит следующий вопрос в
  outbox. После последнего ответа завершает сессию.
- `text` принимает только Telegram text, `voice` — только Telegram voice.
- Входящий ответ не содержит session/question ID: backend находит единственную
  активную сессию кандидата и её текущую позицию.
- Уже начатое интервью разрешено закончить после архивации или истечения вакансии;
  ограничения вакансии проверяются только при создании новой сессии.

Для вакансии задаётся `interview_duration_minutes`: `5`, `15`, `30` или `60`
минут. При старте значение фиксируется в `interview_sessions.expires_at`. Ответ
проверяется относительно дедлайна перед записью, а отдельный worker раз в секунду
переводит просроченную сессию в `expired` и отправляет кандидату уведомление.

Backend публикует для вопроса только сырые предметные данные: `text`,
`answer_type` и служебные `question_number/question_total`. Он не знает о Telegram
HTML. Telegram-бот формирует Rich HTML и отправляет вопрос без ForceReply методом
`sendRichMessage`: текст находится в компактной таблице, счётчик `X/N` выводится
справа нижним индексом через `<sub>`, а подсказка ожидаемого формата — мелким
блоком `<footer>`. Динамический текст HTML-экранируется в Telegram-боте.

## Транскрибация голосовых ответов

В транзакции завершения backend создаёт `pending`-строки транскрипций и кладёт
`interview.completed.v1` в outbox. Сообщение кандидату об успешном завершении
отправляется сразу и не ждёт AI.

Worker использует отдельную PostgreSQL-схему `openrouter_worker` и таблицы
`inbox_events`, `action_jobs`, `voice_transcription_jobs`, `outbox_events`.
Уникальность `(action_type, subject_id)` не позволяет создать вторую job одного
ответа. Четыре runner работают параллельно; lease возвращает зависшую после падения
job в обработку. Успех или permanent failure записываются в одной транзакции с
result outbox, а backend применяет result event идемпотентно через inbox.

Каждая попытка заново вызывает Telegram `getFile`, потоково скачивает не более 20
МБ во временный `tmpfs` и отправляет base64 audio в OpenRouter. Временная директория
удаляется после успеха и любой ошибки, а остатки аварийного завершения очищаются при
старте. Network error, `408`, `429`, `5xx`, ошибки ключа/баланса повторяются без
лимита с exponential backoff и jitter, максимум 30 минут. Удалённый Telegram file,
превышение размера и неподдерживаемое аудио становятся `permanent_failed`.

Worker не пишет в лог API key, Telegram download URL, audio и полный transcript.

## ACK, повторная доставка и ошибки

RabbitMQ `ACK` означает, что consumer закончил обработку и брокер может удалить
сообщение из очереди. Пока ACK не получен, сообщение находится в состоянии
`unacked`. Если consumer или соединение падает, RabbitMQ возвращает его в очередь.

- Успешная Telegram-отправка → событие `delivery.succeeded`, затем ACK.
- Telegram `429` → ожидание `retry_after` и повтор той же отправки без ACK.
- Network error или Telegram `5xx` → exponential backoff `1, 2, 4, 8...`, максимум
  30 секунд; ACK только после успеха.
- Telegram `400/403` → `delivery.failed`, затем ACK, потому что повтор бессмысленен.
- Невалидная RabbitMQ-команда → reject без requeue, брокер направляет её в DLQ.

Нельзя делать немедленный `NACK + requeue` после `429`: это создаёт горячий цикл.
Во время `retry_after` текущая команда остаётся unacked и блокирует только свой
shard, сохраняя порядок.

Гарантия доставки — at least once. Если Telegram принял сообщение, а бот упал до
ACK, RabbitMQ повторит команду и кандидат редко может получить дубль. Обычный Bot
API не предоставляет идемпотентного ключа, полностью закрывающего это окно.

## Лимиты Telegram

Перед каждым `sendMessage` и `sendRichMessage` применяется два ограничения:

- максимум `2 сообщения/с` на каждый outgoing shard;
- максимум `1 сообщение/с` в конкретный Telegram chat.

Таким образом общий настроенный потолок одного bot token составляет
`8 × 2 = 16 сообщений/с`, что оставляет запас относительно бесплатного лимита
Telegram около 30 сообщений/с. Limiter не зависит от числа backend consumers.

Длинный welcome message режется на части до 4096 символов, части проходят через тот
же limiter, а inline-кнопка добавляется только к последней части. `answerCallbackQuery`
выполняется сразу, поскольку не создаёт сообщение в чате, но также обрабатывает
`TelegramRetryAfter`.

Метрики: глубина каждой delivery queue, возраст старейшего сообщения, число `429`,
retry и DLQ, latency от `created_at` до `sent_at`. Увеличение числа consumers не
поднимает лимит одного bot token.

## Retention и cleanup worker

`inbox_messages`, `outbox_events` и `telegram_deliveries` накапливаются при каждом
сообщении, поэтому отдельный Go worker очищает их по расписанию небольшими batch с
`FOR UPDATE SKIP LOCKED`. PostgreSQL advisory lock гарантирует, что при нескольких
репликах очистку выполняет только одна из них.

Безопасные значения по умолчанию:

- `inbox_messages` — 30 дней (`CLEANUP_INBOX_RETENTION=720h`);
- опубликованные `outbox_events` — 7 дней (`CLEANUP_OUTBOX_RETENTION=168h`);
- завершённые `telegram_deliveries` — 90 дней
  (`CLEANUP_TELEGRAM_DELIVERY_RETENTION=2160h`).

Worker запускается сразу при старте, затем раз в `CLEANUP_INTERVAL` (по умолчанию
час) и удаляет по `CLEANUP_BATCH_SIZE` строк (по умолчанию 1000) за запрос.
Неопубликованные `outbox_events` (`published_at IS NULL`) и доставки со статусом
`pending` не удаляются независимо от возраста. Предметные данные интервью,
кандидаты, вопросы и ответы этим worker не затрагиваются.

Технические таблицы `openrouter-worker` очищаются отдельно: inbox через 30 дней,
опубликованный outbox через 7 дней, terminal jobs через 90 дней. Backend хранит
транскрипции как часть интервью без автоматического удаления.

30-дневное окно inbox защищает от обычных повторных доставок RabbitMQ. Если в
будущем появится ручной replay сообщений старше этого окна, перед replay retention
нужно временно увеличить или обеспечить отдельную идемпотентность такого импорта.
