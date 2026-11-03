# HR Assistant

Монорепозиторий сервиса асинхронных HR-интервью. Реализованы авторизация HR, вакансии, вопросы, интервью в Telegram, транскрипция и проверка ответов по ориентирам HR.

## Возможности

- Личный кабинет HR: авторизация, создание вакансий и вопросов для интервью.
- Асинхронное прохождение интервью кандидатом в Telegram.
- Транскрибация голосовых ответов и AI-проверка по ориентирам HR.
- Общий отчёт по кандидату с оценкой по критериям вакансии.
- Автоматическая обратная связь кандидату в Telegram.

## Структура проекта

| Каталог | Назначение |
| --- | --- |
| `frontend/` | Личный кабинет на React, TypeScript и Vite |
| `backend/` | REST API на Go, миграции и фоновая очистка данных |
| `telegram-bot/` | Telegram-бот на Python для проведения интервью |
| `openrouter-worker/` | Go-воркер транскрибации, проверки ответов, отчётов и фидбека через OpenRouter |
| `shared/` | Общий Go-модуль |
| `docs/` | Архитектура и описание реализованных функций |
| `compose.yml` | Локальное окружение с PostgreSQL, RabbitMQ и сервисами приложения |

## Запуск

### Требования

Установите Docker с Docker Compose v2 и запустите Docker. Для полного сценария
интервью нужны Telegram-бот, созданный через BotFather, и ключ OpenRouter.
Сервисы обращаются к Telegram и OpenRouter через интернет.

### Настройка окружения

В корне проекта создайте локальный файл настроек, если его ещё нет:

```bash
cp .env.example .env
```

Если `.env` уже существует, сохраните его настройки и добавьте недостающие
переменные из `.env.example` вручную.

Заполните следующие значения:

| Переменная | Что указать |
| --- | --- |
| `BOT_TOKEN` | Токен Telegram-бота от BotFather |
| `TELEGRAM_BOT_URL` | Ссылку на своего бота вида `https://t.me/your_bot` |
| `JWT_SECRET` | Случайный секрет длиной не менее 32 символов для авторизации |
| `MASTER_API_TOKEN` | Случайный служебный секрет длиной не менее 32 символов, общий для backend и бота |
| `OPENROUTER_API_KEY` | Действующий ключ OpenRouter |

Модели задаются через `OPENROUTER_MODEL` (транскрибация),
`OPENROUTER_EVALUATION_MODEL` (ответы) и `OPENROUTER_REPORT_MODEL` (отчёт и фидбек).
Выберите модели, доступные вашему аккаунту и поддерживающие соответствующие задачи.

### Сборка и запуск

Из корня проекта выполните:

```bash
docker compose up --build -d
```

Миграции базы данных и настройка очередей выполняются автоматически перед запуском
зависимых сервисов. Первый запуск может занять несколько минут.

Проверьте состояние и при необходимости откройте логи:

```bash
docker compose ps -a
docker compose logs -f backend openrouter-worker telegram-bot
```

Успешно завершившиеся служебные контейнеры `migrate`, `openrouter-worker-migrate`
и `openrouter-worker-topology` имеют статус `Exited (0)` — это ожидаемо.

### Адреса и остановка

При настройках из `.env.example`:

| Сервис | Адрес |
| --- | --- |
| Личный кабинет HR | http://localhost:5173 |
| REST API | http://localhost:8080/api/v1 |
| Проверка готовности API | http://localhost:8080/health/ready |
| RabbitMQ Management | http://localhost:15672 |
| PostgreSQL с компьютера | `localhost:5450` |

Для входа в RabbitMQ используйте `RABBITMQ_USER` и `RABBITMQ_PASSWORD` из `.env`.
Если меняете порты, обновите связанные адреса, включая `FRONTEND_ORIGIN`
и `FRONTEND_API_URL`.

Остановка с сохранением данных PostgreSQL и RabbitMQ:

```bash
docker compose down
```

### Разработка frontend вне Docker

Для локальной разработки интерфейса установите Node.js 24 и npm.
Запустите остальные сервисы и затем Vite:

```bash
docker compose up --build -d backend openrouter-worker cleanup-worker telegram-bot
cd frontend
cp .env.example .env
npm ci
npm run dev
```

Команду копирования выполняйте только при отсутствии `frontend/.env`.
Проверки интерфейса: `npm run typecheck`, `npm run lint`, `npm test`;
сборка: `npm run build`. Проверки Go-сервисов запускаются командой
`go test ./...` отдельно из `backend/` и `openrouter-worker/`.

### Если запуск не удался

- Если порт занят, измените соответствующий порт в `.env` и связанные адреса.
- Если сервис не готов, проверьте `docker compose ps -a` и
  `docker compose logs --tail=100 <имя-сервиса>`.
- При ошибках Telegram проверьте токен и отсутствие другого экземпляра того же бота.
- При ошибках AI-задач проверьте ключ OpenRouter, доступность моделей и баланс аккаунта.

## Дополнительные сведения

Перед запуском укажите в `.env` токен бота от BotFather в `BOT_TOKEN`, адрес
бота в `TELEGRAM_BOT_URL` и одинаковый служебный секрет длиной от 32 символов
в `MASTER_API_TOKEN` для бота и backend. Для транскрибации укажите новый ключ
OpenRouter в `OPENROUTER_API_KEY`. Ключ, попавший в чат или логи, сначала нужно
отозвать и перевыпустить.

Команда поднимает PostgreSQL, RabbitMQ, обе группы миграций, backend,
`openrouter-worker`, Telegram-бота и frontend. API доступен на
`http://localhost:8080`, OpenAPI — в `backend/openapi/openapi.yaml`, личный
кабинет — на `http://localhost:5173`, RabbitMQ Management — на
`http://localhost:15672`.

Структура frontend и правила
разработки описаны в `frontend/ARCHITECTURE.md`, а весь UI следует
`frontend/DESIGN.md`.

Все настройки локального окружения находятся в `.env`; файл исключён из Git. `DATABASE_URL` используется контейнерами, а `DATABASE_EXTERNAL_URL` — клиентами на хосте, например DataGrip или DBeaver. Подключение без локального `psql`:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
```

Проверка готовности:

```bash
curl http://localhost:8080/health/ready
docker compose exec openrouter-worker /app/openrouter-worker healthcheck
```

Подробности REST API находятся в `docs/backend-architecture.md`, а поток
кандидатского интервью, shards, ACK, retry и лимиты Telegram — в
`docs/candidate-interview-rabbitmq-plan.md`.

Проверка ответов по ориентирам HR: [план, решения и проверки](docs/feature-answer-evaluation-plan.md).

### Критерии вакансии и общий отчёт

Вакансия поддерживает публичное `description` (до 10 000 символов) и необязательный
список `evaluationCriteria` (до 20). Каталог из 10 шаблонов:
`GET /api/v1/evaluation-criteria/templates`. Список критериев обновляется атомарно
через PATCH вакансии; описание очищается значением null, критерии — пустым массивом.
Пропущенные поля PATCH сохраняют текущие значения.

После обработки завершённого интервью карточка кандидата возвращает `report`:
последний успешный результат, состояние текущей версии, средний балл и покрытие.
`POST /api/v1/candidates/{sessionId}/report/rebuild` с `{ "version": N }` создаёт
следующую версию идемпотентно; для старого интервью без отчёта используется N=0.
Общий анализ использует снимок вакансии при старте и первичные ответы. Без критериев
числовых оценок нет. Модель настраивается через `OPENROUTER_REPORT_MODEL`.

Решения, протокол и проверки: [план реализации](docs/feature-candidate-scoring-plan.md).

### Фидбек кандидату

После успешного сохранения отчёта отдельная AI-задача готовит краткую обратную
связь и отправляет её кандидату в Telegram: сильные стороны, что раскрыть подробнее
и конкретные советы для будущих интервью. Сообщение содержит от одного до трёх
абзацев, каждый до 600 символов; длина проверяется перед отправкой.
Промпт: [candidate_feedback.v1.md](openrouter-worker/prompts/candidate_feedback.v1.md).
Используется модель `OPENROUTER_REPORT_MODEL`.

Фидбек создаётся один раз на интервью. Повторные события и пересборка успешного
отчёта не создают повторную отправку. Если генерация фидбека завершилась ошибкой,
успешная пересборка отчёта запускает её заново. Ошибка фидбека не меняет статус
готового отчёта. Для включения нужны миграции backend `00015` и worker `00004`
и обновление обоих сервисов.
