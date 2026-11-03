# Frontend architecture

Frontend написан на React + TypeScript и следует доменной модели из
`docs/CONTEXT.md`. Визуальный контракт для любого нового UI — только
`frontend/DESIGN.md`; его базовые значения перенесены в `src/app/styles/tokens.css`.

## Module map

```text
src/
├── app/       # composition root: providers, router, global design tokens
├── pages/     # route-level composition, без повторной бизнес-логики
├── widgets/   # крупные части layout, например AppShell
├── features/  # пользовательские сценарии: HR-сессия, работа с вакансиями
├── entities/  # отображение доменных сущностей
└── shared/    # OpenAPI transport, UI primitives, общие чистые utilities
```

Зависимости направлены сверху вниз. `shared` не импортирует остальные слои,
`entities` не знает о страницах, а `features` не знает о роутинге.

## Deep modules

- `features/session` — Module HR-сессии. Его Interface: bootstrap, login,
  register, logout и текущий HR. Access credential хранится только в памяти,
  refresh credential остаётся в HttpOnly cookie. Refresh выполняется single-flight,
  после `401` запрос повторяется не больше одного раза.
- `features/vacancies` — frontend-Module описания интервью. Вакансия, вопросы и
  жизненный цикл публичной ссылки остаются одним агрегатом; mutation инвалидирует
  общий detail cache, а не создаёт независимые глобальные stores.
- `shared/api` — transport Module и единственный Seam с backend. DTO генерируются
  из `backend/openapi/openapi.yaml`; ручные копии контрактных типов запрещены.
- `shared/ui` — design-system Module. Интерактивные состояния, accessibility и
  вариации контролов концентрируются здесь. Не создавайте pass-through обёртки,
  если они не добавляют поведения или инвариантов.

## UI rules

1. Цвета, типографика, радиусы и отступы берутся из CSS variables в
   `src/app/styles/tokens.css`, которые отражают `DESIGN.md`.
2. Чёрная pill-кнопка — главное действие. Miro Yellow используется только для
   логотипа и меток, Brand Blue — для ссылок, focus и активной навигации.
3. Новый повторяемый контрол сначала добавляется в `shared/ui` вместе с focus,
   hover, disabled, error и mobile состояниями.
4. CSS Modules отвечают за локальные стили. Глобальными остаются только reset,
   design tokens и действительно общие accessibility utilities.
5. Server state хранится в TanStack Query. Отдельный client store появляется
   только при реальном втором adapter или сложном локальном workflow.
6. Auth-экраны состоят только из центрированной формы. В AppShell используется
   компактная панель из логотипа и иконок; подписи открываются при hover/focus.

## Commands

```bash
npm install
npm run dev
npm run generate:api
npm run typecheck
npm run lint
npm run test
npm run build
```

Локальный frontend работает на `http://localhost:5173`, backend ожидается на
`http://localhost:8080/api/v1`. Весь stack запускается из корня командой
`docker compose up --build`; `src` и frontend-конфиги подключаются в контейнер
как volumes, поэтому Vite HMR остаётся доступен, а `node_modules` всегда берётся
из свежего Docker-образа. Другой адрес backend задаётся через
`FRONTEND_API_URL` в корневом `.env` или через `VITE_API_URL` при запуске вне Docker.
