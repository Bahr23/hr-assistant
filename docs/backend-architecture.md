# Backend v1: архитектура и запуск

## Границы релиза

Backend обслуживает HR REST API и асинхронное прохождение интервью кандидатами.
Кандидатский контур не добавляет публичных HTTP endpoints: бот использует существующий
REST endpoint вакансии с master token, а команды интервью передаются через RabbitMQ.
Термины предметной области зафиксированы в `docs/CONTEXT.md`.

## Deep Modules

Зависимости направлены от transport и persistence Adapters к двум бизнес-Module:

```text
internal/core
├── domain       # общие сущности, value objects и инварианты
├── database     # подключение к БД и generated sqlc
├── config       # serve / migrate / healthcheck
├── logger       # JSON slog
└── server       # root router, middleware и health

internal/app
├── auth
│   ├── transport
│   ├── service
│   └── repository
├── vacancy
    ├── transport
    ├── service
    └── repository
└── interview
    └── worker      # RabbitMQ consumers, inbox/outbox и состояние интервью
```

- `internal/app/auth/service` — deep HR Session Module. Его Interface — `Open`, `Authenticate`, `Close`; JWT, Argon2id, refresh hash, rotation/reuse detection и проверка отзыва являются Implementation-деталями.
- `internal/app/vacancy/service` — deep Module описания интервью. Его Interface — `List`, `Get`, `Change` с закрытым набором типизированных изменений. Вакансия, вопросы и публичная ссылка изменяются как один агрегат.
- `internal/app/interview/worker` — кандидатский Module. Он принимает sharded команды
  RabbitMQ, изменяет session/answer state транзакционно через inbox/outbox и создаёт
  команды доставки Telegram. После завершения публикует предметное событие для
  отдельного `openrouter-worker`, а его result events применяет идемпотентно.
  Подробный протокол находится в
  `candidate-interview-rabbitmq-plan.md`.
- `internal/core/domain` — общие сущности, value objects, агрегат `InterviewDefinition` и стабильные Fault. Здесь нет HTTP, PostgreSQL и JWT.
- `repository` каждой feature реализует её внутренний persistence Seam. `vacancy/repository.DefinitionStore.Change` блокирует вакансию, загружает вопросы, повторно проверяет инварианты и сохраняет всё одной транзакцией.
- `transport` каждой feature владеет собственным chi router и generated strict OpenAPI Interface. `cmd/api` монтирует auth router на `/api/v1/auth`, а vacancy router — на `/api/v1/vacancies`.
- `internal/core/server` владеет только общими middleware, health endpoints и корневым router; он не знает Implementation feature.
- `cmd/api` — единственный composition root и владелец lifecycle/graceful shutdown.

HTTP-типы и sqlc-модели преобразуются только в Adapters. Для одной production Implementation не создаётся публичный Go `interface`: тестовой поверхностью служат конкретные Interfaces deep Module.

## Транзакционные инварианты

- Чужая вакансия неотличима от отсутствующей.
- Публикация, архивирование, перевыпуск ссылки и все изменения вопросов сериализуются блокировкой одной вакансии.
- Опубликованная вакансия всегда содержит название, язык интервью, приветствие и хотя бы один вопрос; заданный срок действия находится в будущем.
- Позиции вопросов плотные; перестановка содержит каждый текущий вопрос ровно один раз и откатывается целиком при ошибке.
- Каждый вопрос имеет формат ответа `text` или `voice`. Для нового вопроса по умолчанию выбран `text`; существующие вопросы при первой миграции типов сохраняются как `voice`.
- Публичная ссылка существует только у опубликованной вакансии; архивирование удаляет token, восстановление создаёт новый.

## Сессии и безопасность

- Access JWT передаётся как `Authorization: Bearer` и живёт 15 минут.
- Refresh JWT находится в `HttpOnly`, `SameSite=Lax` cookie, живёт 30 дней и ротируется при каждом использовании.
- В БД хранится SHA-256 хэш текущего refresh credential. Повторное использование старого credential отзывает сессию.
- Каждый защищённый запрос проверяет не только JWT, но и активность сессии в PostgreSQL. Поэтому logout и reuse немедленно инвалидируют ранее выданные access JWT.
- Пароли хэшируются Argon2id, email нормализуется.
- Login и register ограничены десятью запросами с одного IP в минуту на экземпляр backend.

## OpenAPI как исполняемый контракт

`backend/openapi/openapi.yaml` — источник маршрутов, request/response types и допустимых статусов. После изменения контракта:

```bash
cd backend
make generate
```

Генератор закреплён на `oapi-codegen v2.8.0`. Один OpenAPI-файл генерирует два feature-local contract Module по тегам: `oapi-auth.yaml` и `oapi-vacancy.yaml`. Optional-nullable поля используют tri-state тип. Будущий TypeScript client для frontend должен генерироваться из этого же файла.

## Локальный запуск

Из корня репозитория:

```bash
cp .env.example .env
docker compose up --build
```

Compose читает все параметры окружения из корневого `.env` и поднимает PostgreSQL,
RabbitMQ, одноразовый `migrate`, API и Telegram-бота. `migrate` требует только
`DATABASE_URL`; backend дополнительно требует `RABBITMQ_URL`; healthcheck следует
`HTTP_ADDRESS` или `HEALTHCHECK_URL`.

Проверки:

```bash
cd backend
make test
TEST_DATABASE_URL='postgres://...' go test -count=1 ./internal/app/auth/repository ./internal/app/vacancy/repository
```

Полный HTTP-контракт находится в `backend/openapi/openapi.yaml`.
