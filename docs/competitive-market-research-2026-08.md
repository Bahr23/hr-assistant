# Конкурентное исследование: асинхронный HR-скрининг в Telegram, WhatsApp и web

**Дата среза:** 30 августа 2026 года  
**Продукт:** HR Assistant  
**Фокус:** продукты, тарифы, функции, продвижение, клиенты и доступные финансовые сигналы

## 1. Краткий вывод

Рынок уже подтвержден и деньгами, и крупными сделками. В верхнем сегменте Paradox вырос до более чем $100 млн ARR по оценке отраслевых источников и был приобретен Workday примерно за $1 млрд. В среднем сегменте работают Sapia.ai, Emi, Talkpush, Spark Hire, Willo и Jobma. В нижнем self-service сегменте продукты стоят от $49–299 в месяц.

При этом ниша именно Telegram-first уже не свободна:

- **Brondem** проводит асинхронный первичный отбор в Telegram, оценивает ответы по заданным HR критериям, ранжирует кандидатов и имеет публичные тарифы от 799 грн в месяц.
- **SoftSkillsChecker** собирает голосовые и video-circle ответы в Telegram, делает транскрипт и оценивает английский и soft skills.
- **BRaiN HR** в России продает текстовые AI-диалоги и видеоинтервью с аналитикой от 9–10 тыс. ₽ в месяц.
- **robotHR** проводит скоринговые интервью по телефону и в Telegram/WhatsApp/других мессенджерах, но продается как кастомный enterprise-проект.

Поэтому слабое позиционирование для HR Assistant — «бот проводит интервью и дает AI-оценку»: это уже commodity. Более сильная позиция:

> **Голосовой первичный скрининг sales/support-кандидатов в Telegram: HR задает проверяемые критерии, кандидат отвечает голосом, сервис дает транскрипт, доказательства по каждому баллу и shortlist — без оценки акцента, тембра и других нерелевантных признаков.**

Главное окно на рынке находится между дешевым, но преимущественно текстовым Brondem и более тяжелыми российскими/международными платформами. Победить можно не широтой ATS, а быстрым запуском, качеством голосового сценария, прозрачностью оценки и сильной вертикальной специализацией.

## 2. Что именно сравнивается с HR Assistant

Согласно продуктовой концепции в репозитории, целевой продукт должен позволять HR:

- создать вакансию, вопросы и критерии оценки;
- отправить публичную или персональную Telegram-ссылку;
- получить голосовые ответы кандидата;
- получить записи, транскрипт, краткое содержание и оценку по критериям;
- увидеть подтверждающие фрагменты, сильные стороны, пробелы и вопросы для следующего этапа;
- сравнить кандидатов одной вакансии;
- оставить финальное решение человеку.

Важно разделять концепцию и текущую реализацию: README и `docs/CONTEXT.md` говорят, что сейчас реализован backend v1 — авторизация HR, вакансии, вопросы и публичные ссылки; кандидаты и ответы еще не входят в backend v1. Поэтому ниже сравнивается **целевая концепция**, а не текущая полнота продукта.

## 3. Карта рынка

### Сегмент A. Почти прямые Telegram-конкуренты

| Продукт | Канал кандидата | Формат | Основная ценность | Сходство |
|---|---|---|---|---:|
| Brondem | Telegram bot / Mini App | текст, тесты, файлы, на платных планах video | первый скрининг, AI-отчет, рейтинг 0–100, risk signals | Очень высокое |
| SoftSkillsChecker | Telegram bot | voice и video circles | английский, коммуникация, soft skills, транскрипт | Очень высокое |
| HR Assistant | Telegram | планируется voice | критерии HR, транскрипт, доказательная оценка, сравнение | — |

### Сегмент B. Мессенджерные платформы массового найма

| Продукт | Каналы | Основной сценарий | Целевой клиент |
|---|---|---|---|
| BRaiN HR | web/video/chat, интеграции | скрининг резюме, текстовый бот, AI-video, avatar | Россия: SMB–enterprise |
| robotHR | телефон, Telegram, WhatsApp, VK, Viber и др. | поиск, скоринг, запись, сопровождение до выхода | массовый найм |
| Talkpush | WhatsApp, Messenger, Instagram, SMS, TikTok | attract → screen → assess → documents → onboarding | BPO, retail, logistics, enterprise |
| Emi | WhatsApp, Messenger, SMS | attract → screen → schedule → hire | frontline hiring, Латинская Америка |
| Paradox / Workday | SMS, WhatsApp, web chat, Messenger | apply, screening, scheduling, events, offers, onboarding | крупный frontline enterprise |
| Sapia.ai | mobile web chat | структурированное текстовое AI-интервью и predictive scoring | международный enterprise |

### Сегмент C. Асинхронные video/web-интервью

| Продукт | Формат | AI | Целевой клиент |
|---|---|---|---|
| Truffle | resume + one-way video + assessments | match по критериям, summaries, 30-sec highlights | small business, self-service |
| Willo | video/audio/text/files/tests | AI-supported screening, human decision | mid-market/high volume |
| Hireflix | one-way video | транскрипция, без заявленного AI-ranking | SMB/mid-market |
| Spark Hire | live + one-way video | transcript, summary, scoring | mid-market, 10k+ организаций |
| Jobma | one-way/live/agentic AI video + assessments | AI scoring, language, fraud deterrence | mid-market/enterprise |

## 4. Сравнение ключевых функций

Обозначения: **Да** — явно заявлено продуктом; **Частично** — есть близкая функция; **Нет/не заявлено** — не найдено в публичных материалах.

| Продукт | Telegram | WhatsApp | Voice/audio | Video | HR задает критерии | AI transcript/summary | AI score/ranking | Scheduling | ATS/API |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| HR Assistant, концепция | Да | Нет | Да | Нет | Да | Да | Да, с доказательствами | Позже | Позже |
| Brondem | Да | Нет | Не основной | Да, платно | Да, expected answer | Да | Да | Нет | Enterprise/custom |
| SoftSkillsChecker | Да | Нет | Да | Да | Пока нет полного конструктора | Да | Да, English/soft skills | Нет | Не заявлено |
| BRaiN HR | Частично/бот | Неясно | Частично | Да | Да | Да | Да | Частично | 1С, Битрикс24, API и др. |
| robotHR | Да | Да | телефон/речь | Да | Да | Частично | Да | Да | Кастомно |
| Talkpush | Нет | Да | Да | Да | Да | Да | Да | Да | Meta, Workday, SAP, Oracle, API |
| Emi | Нет | Да | Не основной | Не основной | Да | Да | Да | Да | Enterprise integrations |
| Paradox | Нет | Да | Не основной | On-demand video | Да | Да | screening | Да, сильная сторона | Workday, SAP и др. |
| Sapia.ai | Нет | Нет, mobile web | Нет | Доп. этап | Модель компетенций | Да | Да, explainable | Да | SAP/ATS |
| Truffle | Нет | Нет | Через video | Да | Да | Да | Да, evidence-linked | Нет | ATS/Zapier |
| Willo | Нет | SMS invites | Да | Да | Scorecards | Да | AI-supported | Нет | API/ATS |
| Hireflix | Нет | invitations | Не основной | Да | Scorecards/comments | Transcript | Нет ranking | Нет | GraphQL/webhooks/ATS |
| Spark Hire | Нет | Нет, SMS invites | В video | Да | Да | Да | Да | Да | 40+ ATS, API add-on |
| Jobma | Нет | Не основной | В video | Да | Да | Да | Да | Да | ATS/HRMS/CRM/Zapier |

### Что видно из матрицы

1. **Telegram сам по себе не является moat.** Brondem и SoftSkillsChecker уже используют этот канал.
2. **Voice-first остается хорошим wedge.** Brondem в публичном описании прежде всего показывает текст, тесты и Mini App; SoftSkillsChecker использует voice, но пока не дает HR полноценный конструктор критериев.
3. **Проверяемая оценка важнее общего AI-score.** Truffle, Sapia и современный Spark Hire уже объясняют оценки. Черный ящик будет проигрывать и по доверию, и по enterprise procurement.
4. **Scheduling и ATS-интеграции — table stakes для enterprise, но не обязательны для первой версии SMB-продукта.**

## 5. Тарифы

Цены приведены в исходной валюте, без пересчета. Это исключает искажение из-за курса и налогов.

### 5.1. Прозрачные публичные цены

#### Brondem — наиболее прямой ценовой ориентир

[Публичные тарифы Brondem](https://brondem.com/pricing.html):

| План | Цена/мес. | Активные вакансии | AI-отчеты/мес. | HR | Важные функции |
|---|---:|---:|---:|---:|---|
| Free | 0 грн | 1 | 20 | только владелец | базовая модель, без экспорта/Mini App |
| Starter | 799 грн | 2 | 400 | только владелец | CSV, video replies, файлы, Telegram notifications, 2 GB |
| Pro | 2 349 грн | 5 | 1 150 | до 3 | улучшенная модель, Mini App, сравнение, PDF/Excel, 10 GB |
| Agency | 4 999 грн | 15 | 2 500 | до 6 | JSON, 50 GB, больше source links |

Add-ons: 300 отчетов — 699 грн; дополнительная вакансия — 299 грн/мес.; HR seat — 299 грн/мес.; 5 GB — 199 грн/мес. Годовой план дает скидку 20%. Публичные лимиты дают необычно низкую цену около 2 грн за AI-отчет при полном использовании; это похоже на агрессивное launch pricing и не должно автоматически приниматься за устойчивую себестоимость рынка.

#### BRaiN HR — российский ориентир по usage pricing

[Тарифы BRaiN HR 2026](https://brainhire.ru/tariffs):

| Линия | Пакеты в месяц | Цена/мес. | Эффективная цена |
|---|---|---:|---:|
| Chat | 300 / 1 000 / 3 000 диалогов | 9 000 / 25 000 / 60 000 ₽ | 30 / 25 / 20 ₽ за диалог |
| Interview | 50 / 150 / 500 video-интервью | 10 000 / 27 000 / 80 000 ₽ | 200 / 180 / 160 ₽ за интервью |
| Full | 300+30 / 600+60 / 1 000+200 / 2 000+400 | 15 000 / 27 000 / 60 000 / 104 000 ₽ | bundle |
| Real-time avatar | 1 интервью до 45 минут | 3 000 ₽ | за интервью |

Годовая скидка — 10%; неиспользованные объемы переносятся при продлении; доступен пилот на трех вакансиях.

#### Международный self-service/mid-market

| Продукт | Публичная цена | Модель и ограничения | Источник |
|---|---:|---|---|
| Truffle | от $49/мес. | shared credits; completed one-way interview = 5 credits; 7-day trial | [официальный сайт](https://www.hiretruffle.com/), [pricing explainer](https://www.hiretruffle.com/blog/candidate-screening-software-pricing) |
| Willo Lite | $59 за active role | pay-as-you-go, до 150 кандидатов/роль, до 5 active roles/users | [Willo pricing overview](https://www.willo.video/blog/the-top-5-free-video-interview-platforms) |
| Willo Enterprise | от $3 799/год | annual custom volume; в GBP — от £2 999/год | [pricing page](https://es.willo.video/pricing) |
| Hireflix Small | $75/мес. при годовой оплате | <50 сотрудников; unlimited users, positions, responses | [Hireflix pricing](https://hireflix.com/pricing) |
| Hireflix Medium | $150/мес. при годовой оплате | 50–250 сотрудников; все функции, unlimited | [Hireflix pricing](https://hireflix.com/pricing) |
| Spark Hire monthly | от $299/мес. | до 5 jobs и 5 users; unlimited interviews | [Spark Hire pricing](https://www.sparkhire.com/pricing/) |
| Spark Hire annual | от $249/мес. | unlimited jobs/users/interviews, annual billing | [Spark Hire pricing](https://www.sparkhire.com/pricing/) |
| Sapia Starter | $5 000 за 3 месяца | unlimited interviews/roles/users, marketplace interviews | [SAP Store listing](https://www.sap.com/products/hcm/partners/sapiaco-pty-ltd-sapiaai-smart-interviewer.html) |

### 5.2. Custom/enterprise pricing

| Продукт | Публичный статус | Доступный ориентир |
|---|---|---|
| SoftSkillsChecker | tailored offer, early access | цены не опубликованы |
| robotHR | запрос решения | цены не опубликованы; старый ориентир Stafory/«Вера» не переносим на текущий robotHR |
| Talkpush | Get a quote по модулям и объему | в старом founder interview: обычно $1–3k/мес., отдельные клиенты $5–6k/мес.; это не текущий прайс |
| Emi | demo/enterprise | не опубликовано |
| Paradox/Workday | custom quote | независимые оценки дают $30–95k/год плюс implementation, но официальный прайс отсутствует |
| Jobma | Starter/Growth/Professional/Enterprise — Request Pricing | цены не опубликованы |

### Ценовой вывод для HR Assistant

Рынок использует четыре базы тарификации:

1. по числу завершенных интервью/AI-анализов;
2. по числу active roles;
3. по headcount компании;
4. custom annual volume для enterprise.

Для HR Assistant лучше всего подходит **оплата за завершенный AI-отчет с лимитом active vacancies**, потому что клиент понимает единицу ценности, а себестоимость голоса/транскрипции контролируема. Seats не должны быть основной метрикой: Talkpush прямо объяснял, что автоматизация позволяет одному рекрутеру обрабатывать в 5–10 раз больше потока, поэтому per-seat плохо захватывает ценность.

## 6. Разбор основных продуктов

### 6.1. Brondem — главный прямой конкурент

**Продукт.** HR создает вакансию, вопросы и expected answers; кандидат проходит Telegram bot/Mini App flow; сервис выдает score 0–100, correctness/relevance/completeness, AI summary, strengths/weaknesses, recommendation и integrity/risk signals. Есть source links, файлы, экспорт, team roles и сравнение кандидатов. [Описание функций](https://brondem.com/en/features.html), [методика scoring](https://brondem.com/en/metrics.html).

**Продвижение.** Трехъязычный сайт (UA/RU/EN), прозрачные цены, бесплатный план без карты, отдельные SEO-страницы под features/metrics/pricing/privacy/terms. Основной conversion loop — «создай тестовую вакансию и сам пройди интервью». Независимых кейсов, узнаваемых логотипов и публичных финансовых данных не найдено.

**Сильная сторона.** Максимально близкая локальная упаковка и очень низкий порог входа.

**Слабая сторона/окно.** Публичный flow выглядит text/test-first; доказанный voice-first UX и оценка устной коммуникации не являются центром бренда. Integrity/dishonesty signals могут создавать недоверие, если методика не доказана.

### 6.2. SoftSkillsChecker — прямой voice-конкурент, но ранний

**Продукт.** Кандидат отвечает в Telegram голосом или video-circle на 8–10 динамических вопросов; процесс занимает менее 10 минут. Отчет включает CEFR English level A2–C1, communication/confidence/clarity, red flags, записи и transcript. [Официальный сайт](https://softskillschecker.com/).

**Ограничения, признанные самим продуктом.** Вопросы пока нельзя кастомизировать; anti-cheating еще разрабатывается; сервис не обещает автоматический reject. Это важное окно для HR Assistant: дать HR контроль над вопросами, rubric и весами раньше конкурента.

**Продвижение.** Одна Tilda-landing page, CTA на demo/early access и квалифицирующий survey по месячному объему, текущему способу проверки английского и использованию AI. То есть продукт пока проверяет ICP и willingness to pay, а не масштабирует self-service. Выручка не раскрыта.

### 6.3. BRaiN HR — российский full-funnel конкурент

**Продукт.** Генерация вакансий, resume screening, chat bot, AI-video interview с transcription/score/recommendations и proctoring, real-time avatar, integrations. Отдельно продаются chat, interview и combined plans. [Продукт](https://brainhire.ru/), [тарифы](https://brainhire.ru/tariffs).

**Продвижение.** Сильная локальная enterprise-упаковка: 152-ФЗ, российская инфраструктура/MWS, реестр отечественного ПО, отраслевые страницы, кейсы, ROI-калькулятор, три вакансии бесплатно, блог, партнерская программа и соцсети. На сайте заявлены кейсы iTeal, СКБ Контур и KNAM с числовыми результатами.

**Финансы.** Оператор — ООО «НДК», ИНН 5009132924. По данным бухгалтерской отчетности, агрегированным Т-Банком/Saby, в 2025 году у юрлица было около 349 тыс. ₽ выручки и 8,65 млн ₽ чистого убытка. Это данные всего юрлица, а не проверенная product revenue BRaiN; они показывают раннюю инвестиционную стадию, а не traction тарифа. [Карточка контрагента](https://www.tbank.ru/business/contractor/legal/1225000105010/).

### 6.4. robotHR — кастомная автоматизация массового найма

**Продукт.** Поиск кандидатов, чат- и voice-интервью, оценка по критериям, scheduling, reminders и сопровождение до выхода; заявлены Telegram, WhatsApp, VK, Messenger, Viber, email и телефон. [Официальный сайт](https://robothr.ru/).

**Продвижение.** Старый SEO-heavy сайт, длинные problem/solution страницы, массовый найм как специализация и CTA «запросить решение». Публичного self-service, актуальной тарифной сетки, независимых финансов и современной продуктовой аналитики не найдено.

**Вывод.** Это скорее интегратор/проектное решение. HR Assistant может выигрывать скоростью запуска и прозрачной подпиской, но не широтой кастомизации.

### 6.5. Talkpush — WhatsApp-first high-volume ATS

**Продукт.** Брендированные WhatsApp/Messenger/Instagram bots, social sourcing, landing pages, calendar/self-scheduling, audio/video/text assessments, language tests, scoring/ranking, document recognition и onboarding. Интеграции с Meta, Workday, SAP, Oracle и custom API. [Packages](https://www.talkpush.com/pricing).

**Продвижение.** Enterprise case studies с числовым ROI, отраслевые страницы, podcast CEO, webinars/events, white papers, academy/certification, newsletter и много языковых версий. Это полноценная content + community + enterprise sales машина.

**Финансы.** GetLatka оценивает 2024 revenue в $3,1 млн, около 30 клиентов, $1,3 млн привлеченного капитала и средний ACV около $104k. Источник смешивает estimate и более старое founder interview, поэтому уверенность средняя. [Профиль и transcript](https://getlatka.com/companies/talkpush).

### 6.6. Emi — эталон вертикализации вокруг frontline workers

**Продукт.** AI-агенты ведут кандидата через WhatsApp/Messenger/SMS от QR-кода или рекламы до screen, schedule, документов и hire. Компания заявляет 12 млн+ AI conversations, 50+ enterprise customers и то, что 1 из 10 frontline hires в Мексике проходит через Emi. [Официальный сайт](https://www.emilabs.ai/en).

**Продвижение.** Очень узкое географическое и вертикальное сообщение; логотипы Walmart, OXXO, Coca-Cola FEMSA, Mercado Libre, Home Depot, Heineken; кейсы с time-to-hire и vacancy fill rate; demo-led sales. Они продают не «бота», а инфраструктуру массового найма в регионе.

**Финансы.** В 2022 Emi подняла $11 млн Series A; общий капитал оценивается примерно в $13 млн. Вторичный профиль сообщает около $10,5 млн revenue в 2023 при команде 100+, но это не audited disclosure. [Funding release](https://www.prnewswire.com/news-releases/emi-raises-11m-to-help-enterprises-hire-better-frontline-workers-faster-301488746.html), [secondary company profile](https://yespress.io/emi-labs).

### 6.7. Paradox / Workday — доказательство верхнего потолка рынка

**Продукт.** Conversational Apply, screening, scheduling, career sites, events, CRM, ATS, offers и onboarding через SMS, WhatsApp, chat и Messenger. Candidate apply идет без логинов и форм; сильнейшая функция — сложный автоматический scheduling. [Conversational Apply](https://www.paradox.ai/products/conversational-apply), [Scheduling](https://www.paradox.ai/products/conversational-scheduling).

**Продвижение.** Enterprise ABM: case studies Chipotle/GM/7-Eleven/Dell/Compass, ROI в часах и днях, research reports, webinars, отраслевые страницы и стратегические интеграции. После приобретения главный канал — installed base Workday. [Workday integration](https://www.paradox.ai/partners/workday).

**Финансы.** Workday официально сообщил о сделке, но не раскрыл цену; отраслевые материалы Houlihan Lokey и broker research оценивают enterprise value примерно в $1 млрд, ARR более $100 млн и рост около 25%. До сделки Paradox привлек $200 млн Series C. Компания сообщала о 431% росте за три года к 2024 году. [Workday announcement](https://investor.workday.com/news-and-events/press-releases/news-details/2025/Workday-Signs-Definitive-Agreement-to-Acquire-Paradox-the-AI-Company-Redefining-the-Frontline-Candidate-Experience-08-21-2025/default.aspx), [Houlihan Lokey 2025 HCM review](https://cdn.hl.com/pdf/2026/hcm-year-in-review-2025-mar-2026.pdf), [Series C](https://www.prnewswire.com/news-releases/on-a-mission-to-build-the-next-generation-of-hr-and-recruiting-software-through-conversational-ai-paradox-raises-200m-series-c-301450591.html).

### 6.8. Sapia.ai — structured chat + science + explainability

**Продукт.** Одинаковое структурированное text-chat интервью для каждого кандидата, competency model, explainable scoring, shortlist, candidate feedback и analytics. В 2025 компания сообщила о 8,3 млн chat interviews и 9/10 candidate satisfaction. [Платформа](https://sapia.ai/), [2025 review](https://sapia.ai/2025/).

**Продвижение.** Scientific validation, independent bias audit, responsible AI, SAP marketplace, named enterprise case studies, industry pages, whitepapers/e-books и consultation-led sales. Они продают «fair and auditable hiring», а не просто экономию времени.

**Финансы.** Публичной отчетности нет. Несколько firmographic services дают более согласованный диапазон $9–10,4 млн annual revenue; headline GetLatka в $35 млн конфликтует с его же founder transcript про примерно $6 млн run-rate, поэтому $35 млн не стоит использовать как надежную цифру. Компания подняла $17 млн Series A в 2022; Dealroom указывает около $24 млн общего капитала. [Growjo estimate](https://growjo.com/company/Sapia), [Prospeo estimate](https://prospeo.io/c/sapia-ai-revenue), [Dealroom profile](https://app.dealroom.co/companies/sapia).

### 6.9. Truffle — опасный новый self-service конкурент и сильный GTM-кейс

**Продукт.** Resume screening, qualification checks, one-way video, assessments, AI Match по критериям, evidence-linked reasoning, summaries и 30-секундные Candidate Shorts. Цена от $49/мес., trial без карты. [Официальный сайт](https://www.hiretruffle.com/).

**Продвижение.** Очень агрессивный SEO/AEO: страницы сравнений с каждым конкурентом, статьи о pricing, vertical landing pages, собственный subreddit с comparison-постами и отдельная [canonical page для AI assistants](https://hiretruffle.com/ai-instructions). Последняя прямо инструктирует ChatGPT/Claude/Gemini, как категоризировать продукт. Это современная комбинация search, answer-engine optimization и product-led trial.

**Финансы.** Продукт новый; надежных revenue/funding данных не найдено.

### 6.10. Willo — пример сильного product/content-led mid-market

**Продукт.** Асинхронные video/audio/text/file responses, structured assessments, question library, scorecards, anti-cheating, AI-supported review, ATS/API. Компания заявляет 5 000+ hiring teams. [Официальный сайт](https://www.willo.video/).

**Продвижение.** 14-day trial, pay-as-you-go, comparison directory, free interview question generator, ROI/time-to-hire calculators, podcast, startup/nonprofit discounts, G2 badges, PR-исследования и customer stories. [Resources](https://www.willo.video/resources), [free tools](https://www.willo.video/tools).

**Финансы.** Абсолютная выручка не раскрыта. Willo официально сообщает 64x рост revenue с первого года и +70% в 2025, а также $4 млн инвестиций в сентябре 2025; CB Insights figure $50 млн выглядит несогласованно с остальными сигналами и не используется. [Funding announcement](https://www.willo.video/blog/willo-raises-4m-investment-to-help-you-hire-faster-fairer-and-smarter).

### 6.11. Hireflix — простой unlimited без AI-ranking

**Продукт.** One-way video с text/video questions, think time, retakes, branding, open links, WhatsApp/SMS/email reminders, team review, transcript, GraphQL API/webhooks и ATS integrations. Все функции доступны на всех планах. [Pricing/features](https://hireflix.com/pricing).

**Продвижение.** Прозрачная цена, один месяц бесплатно, простое обещание «unlimited everything», интеграции и white-label. Это сильный low-complexity benchmark.

**Финансы.** GetLatka оценивает 2025 ARR примерно в $990k; это независимая оценка, не audited result. [Estimate](https://getlatka.com/companies/hireflix.com).

### 6.12. Spark Hire — зрелый mid-market consolidator

**Продукт.** One-way/live video, SMS/link/CSV invites, reminders, scorecards, 50+ transcription languages, summaries, AI Video Review, scheduling, 40+ ATS integrations и ATS/behavioral assessment как соседние продукты. [Pricing](https://www.sparkhire.com/pricing/).

**Продвижение.** Огромная SEO-библиотека, weekly webinars, templates/playbooks, podcast, comparison pages, G2 reviews и 10 000+ customer logos. Monthly self-service ведет в annual demo motion. [Resources](https://www.sparkhire.com/resources/), [weekly demos](https://www.sparkhire.com/how-it-works/).

**Финансы.** GetLatka оценивал standalone Spark Hire в $5,8 млн revenue в 2024 при 6k клиентов. В 2025 Spark Hire, Comeet и Chally объединились под одним брендом, поэтому старая цифра не отражает текущую объединенную компанию. [Estimate](https://getlatka.com/companies/spark-hire), [rebrand note](https://www.sparkhire.com/faq/).

### 6.13. Jobma — широкий enterprise feature set

**Продукт.** One-way/live/agentic interviews, skills/language/coding/game assessments, fraud deterrence, mobile apps, ATS/HRMS/CRM/Zapier integrations, support кандидатов. Тарифы скрыты за Request Pricing. [Plans](https://www.jobma.com/plans-and-pricing), [product](https://www.jobma.com/our-product).

**Продвижение.** Named logos, отраслевые landing pages и особенно подробные case studies с completion, satisfaction, time saved и ROI. Например, StaffYourDealer сообщает 18k invites, 9k saved recruiter hours и 2,7x ROI. [Case study](https://www.jobma.com/case-studies/staff-your-dealer).

**Финансы.** Публичной выручки нет. Независимые каталоги дают слишком широкий диапазон $1–10 млн или $10–50 млн, поэтому точную цифру указывать нельзя. Более надежные scale signals: 51–200 сотрудников, клиенты в 40 странах и известные enterprise logos. [About](https://www.jobma.com/about-us), [LinkedIn profile](https://www.linkedin.com/company/jobma).

## 7. Как конкуренты продвигаются

### 7.1. Product-led motion

Используют Brondem, Truffle, Willo, Hireflix и частично Spark Hire:

- free plan или trial без карты;
- shareable link, чтобы запустить вакансию за минуты;
- прозрачные цены;
- self-demo, где HR сам проходит путь кандидата;
- pay-as-you-go для нерегулярного найма.

Это лучший путь для HR Assistant на старте: низкий ACV не выдержит дорогую sales-команду.

### 7.2. ROI-led enterprise sales

Используют Paradox, Talkpush, Emi, Sapia, Jobma и BRaiN HR:

- кейсы с time-to-hire, completion, cost saving, candidate satisfaction;
- узнаваемые logos и quotes с должностями;
- отраслевые страницы;
- demo/consultation вместо checkout;
- security/compliance pack;
- пилот на одной–трех вакансиях.

Главный урок: продается не «AI-интервью», а измеримый результат — часы рекрутера, скорость контакта, completion и качество shortlist.

### 7.3. Search, content и answer-engine optimization

- Spark Hire строит бесплатные курсы, templates, webinars и playbooks.
- Willo создает comparison directory, calculators, question generator, reports и podcast.
- Talkpush создает podcast, academy/certification, events и whitepapers.
- Truffle публикует конкурентные pricing pages и даже структурированную страницу специально для AI assistants.
- BRaiN HR локализует SEO под отрасли и дополняет его калькулятором экономии.

Это показывает, что обычный блог «про HR и AI» недостаточен. Лучше создавать инструменты и страницы с высоким намерением: «вопросы для первичного интервью sales manager», «стоимость телефонного скрининга», «как оценить голосовой ответ кандидата», «Telegram HR bot vs Google Forms».

### 7.4. Platform partnerships

- Paradox растет через Workday и enterprise ATS.
- Sapia — через SAP marketplace и ATS.
- Talkpush — через Meta/WhatsApp, Workday, SAP и Oracle.
- Spark Hire/Hireflix/Willo — через ATS directories, Zapier/Make и API.
- BRaiN HR — через MWS, 1С/Битрикс24 и реестр российского ПО.

Для HR Assistant российские equivalents — hh.ru workflow, Huntflow, Potok, Talantix, 1С:ЗУП, Bitrix24 и recruiting agencies. Но интеграции надо добавлять после доказанного completion rate, а не раньше core interview flow.

## 8. Финансовая сводка и качество данных

| Компания | Последний доступный revenue/ARR signal | Funding/M&A | Уверенность |
|---|---:|---:|---|
| Paradox | >$100 млн ARR, ~25% рост по отраслевым источникам | ~ $1 млрд acquisition; $200 млн Series C | Средне-высокая для диапазона, цена официально не раскрыта |
| Emi | ~ $10,5 млн revenue за 2023, secondary report | ~ $13 млн total funding | Средняя-низкая |
| Sapia.ai | согласованный estimate ~$9–10,4 млн; 8,3 млн interviews в 2025 | $17 млн Series A; ~$24 млн total | Средняя для scale, низкая для точной выручки |
| Spark Hire standalone | ~ $5,8 млн estimate за 2024 | bootstrapped historically; затем объединение брендов | Низкая для текущей компании |
| Talkpush | ~ $3,1 млн estimate за 2024 | ~ $1,3 млн | Средняя-низкая |
| Hireflix | ~ $0,99 млн ARR estimate за 2025 | не найдено | Низкая |
| BRaiN HR operator | юрлицо: 349 тыс. ₽ revenue, -8,65 млн ₽ profit/loss за 2025 | не найдено | Высокая для юрлица, низкая для product-only |
| Willo | absolute не раскрыт; +70% revenue growth в 2025 | $4 млн round в 2025 | Высокая для growth claim, нет абсолютной цифры |
| Jobma | точная выручка не раскрыта | не найдено надежных данных | Низкая |
| Brondem | не раскрыто | не найдено | Нет данных |
| SoftSkillsChecker | не раскрыто; early access | не найдено | Вероятно pre-scale, но не доказано |
| Truffle | не раскрыто; новый self-service product | не найдено | Нет данных |
| robotHR | не раскрыто | не найдено | Нет данных |

**Интерпретация:** у частных HR-tech компаний revenue databases часто противоречат друг другу. Funding, customer count, disclosed growth, named cases и acquisition value надежнее одной «точной» цифры из агрегатора. Любые оценки выше следует использовать для порядка величины, не для финансовой модели.

## 9. Возможность для HR Assistant

### 9.1. Лучший начальный ICP

**Рекрутинговые агентства и небольшие внутренние HR-команды, которые ежемесячно скринят 50–500 кандидатов на sales, support, account management и call-center роли.**

Почему:

- голос действительно дает полезный job-related signal;
- вопросы повторяются;
- экономия 15–30 минут на слабом кандидате легко объясняется;
- Telegram уже используется в коммуникации;
- агентства могут вести несколько вакансий и стать каналом распространения;
- для них Paradox/Talkpush слишком тяжелы, а формы не анализируют ответы.

Не стоит начинать одновременно с IT, массовым retail, top-management и всеми языками. Emi и Sapia показывают силу узкой вертикали; универсальность приходит позже.

### 9.2. Рекомендуемая продуктовая последовательность

**P0 — то, за что платят:**

- Telegram voice flow с test recording и понятным progress;
- text fallback для доступности и плохой связи;
- явное согласие на запись/транскрипцию;
- transcript + timestamps;
- criteria/rubric per question;
- score с цитатами/фрагментами ответа и confidence;
- summary, strengths, gaps, follow-up questions;
- shortlist одной вакансии;
- исходная запись всегда доступна человеку;
- CSV/PDF/share link для hiring manager.

**P1 — увеличивает retention:**

- source links и conversion funnel;
- reminders и incomplete candidates;
- team comments/statuses;
- agency/client workspace;
- reusable templates для sales/support;
- webhook/Zapier/Make, затем Huntflow/Potok/Bitrix24.

**P2 — только после PMF:**

- WhatsApp;
- adaptive follow-up questions;
- scheduling;
- full ATS;
- identity/fraud tooling.

Не следует делать emotion, accent, voice tone, gender/age inference или «lie detector» частью scoring. Это ухудшает доверие и противоречит уже сформулированному в концепции принципу оценивать только job-related content.

### 9.3. Тарифная гипотеза для теста

Это не рекомендация «навсегда», а стартовый willingness-to-pay experiment:

| План | Цена | Лимит | Для кого |
|---|---:|---:|---|
| Free | 0 ₽ | 1 vacancy, 10 completed AI reports, 1 user | проверить candidate и HR flow |
| Solo | 4 900 ₽/мес. | 2 active vacancies, 50 reports | solo recruiter/SMB |
| Team | 14 900 ₽/мес. | 5 vacancies, 250 reports, 5 users | регулярный найм |
| Agency | 39 900 ₽/мес. | 20 vacancies, 1 000 reports, 10 users/client exports | агентства |
| Overage | 70–120 ₽ | за completed report | сезонный поток |

Логика:

- ниже BRaiN HR video в пересчете на интервью;
- выше экстремально дешевого Brondem, но за voice/transcript/evidence;
- намного ниже международного mid-market;
- стоимость привязана к полученному отчету, а не приглашению;
- годовая оплата может давать два месяца бесплатно;
- первые 5–10 design partners лучше продавать пилотом по фиксированной цене, чтобы проверить экономику и поддержку.

### 9.4. Go-to-market на первые 90 дней

1. **Одна вертикальная landing page:** «Первичный голосовой скрининг менеджеров по продажам в Telegram».
2. **Бесплатный инструмент:** generator вопросов и rubric для sales interview, результат можно сразу импортировать в вакансию.
3. **Founder-led outbound:** 50 агентств/HR-команд с персональным ROI расчетом на их объеме звонков.
4. **Пять design partners:** один и тот же набор метрик до/после.
5. **Обязательные продуктовые метрики:** invite→start, start→complete, median completion time, recruiter review time, qualified-to-next-stage, candidate satisfaction, cost per completed report.
6. **Первый кейс:** не «использовали AI», а «заменили N первичных звонков, completion X%, сэкономили Y часов, HR согласился/не согласился с AI в Z% случаев».
7. **Контент с purchase intent:** Telegram HR bot, асинхронное интервью, вопросы sales manager, скрининг голосом, сравнение с Google Forms/телефонным звонком.
8. **Distribution loop:** source-specific links для hh.ru/Telegram channel/agency и shareable candidate report для hiring manager.

## 10. Главные риски

1. **Candidate drop-off.** Асинхронный формат экономит время HR, но может восприниматься как перекладывание работы на кандидата. Нужны duration preview, test recording, human review promise и быстрый feedback.
2. **Недоверие к AI-score.** Нужны rubric, evidence, confidence и возможность HR изменить решение; просто score 0–100 уже не дифференцирует.
3. **Слишком широкий roadmap.** Попытка сразу стать ATS приблизит продукт к BRaiN/Talkpush/Paradox и уничтожит преимущество скорости.
4. **Зависимость от Telegram.** Нужен web fallback и экспорт данных; позднее — WhatsApp, если ICP его требует.
5. **Экономика voice AI.** Лимиты должны считаться по завершенным минутам/отчетам, а не только по кандидатам.
6. **Локальное доверие и данные.** Для российского B2B отсутствие четкой страницы о согласиях, хранении и удалении данных будет blocker еще до demo.
7. **Прямая ценовая война с Brondem.** Нельзя выигрывать только ценой; нужно доказать, что голосовой signal и evidence сокращают review time или улучшают shortlist.

## 11. Итоговое решение

Рынок привлекательный, но не пустой. Есть подтвержденный enterprise ceiling, живой self-service сегмент и как минимум два прямых Telegram-аналога. Наиболее рациональная стратегия:

- не строить «еще одну универсальную AI hiring platform»;
- занять voice-first сегмент для sales/support;
- сделать explainable, criteria-based report главным продуктом;
- начать с self-service + design-partner sales;
- продавать completed reports и measurable recruiter time saved;
- превратить кейсы, ROI-калькулятор, role templates и Telegram distribution в основной growth loop;
- WhatsApp, scheduling и ATS добавлять только после доказанного core PMF.

Самая важная проверяемая гипотеза теперь не «нужен ли рынку Telegram-бот» — конкуренты уже доказали, что нужен. Гипотеза звучит так:

> **Готовы ли HR и агентства платить 60–100 ₽ за завершенный голосовой AI-отчет, если он сокращает первичный review до 2–3 минут и дает более полезный signal для sales/support, чем текстовая анкета?**

Именно этот вопрос должен определять следующие пилоты, pricing tests и продуктовые приоритеты.
