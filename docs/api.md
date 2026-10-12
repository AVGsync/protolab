# ProtoLab REST API v1 — контракт frontend ↔ backend

Документ описывает API между React-фронтендом (`frontend/`) и Go-сервером (`internal/server`).
Структуры данных описаны дважды и должны совпадать: TypeScript-типы в
`frontend/src/entities/*/model/types.ts` и `frontend/src/shared/api/types.ts`, Go-структуры в
`internal/model/entities.go`. Поведение реализовано в `internal/app` (Go) и повторено в
mock-обработчиках `frontend/src/mocks/handlers/*` для разработки интерфейса без сервера.

## Общие соглашения

| Аспект | Соглашение |
|---|---|
| Базовый путь | `/api/v1`. Фронтенд и API отдаются одним бинарём с одного origin (по умолчанию `http://127.0.0.1:7420`); в dev Vite проксирует `/api` на `127.0.0.1:7420` |
| Формат | `application/json`, поля в `camelCase` |
| Время | RFC 3339 в UTC с наносекундами (`time.RFC3339Nano`), например `2026-09-14T10:00:05.000310Z`. Фронтенд берёт доли секунды прямо из строки — не округляйте до миллисекунд |
| Идентификаторы | непрозрачные строки (`prj-…`, `cap-…`, `ses-…`); фронтенд не разбирает их |
| Списки | `{"items": [...], "total": N}` |
| Ошибки | HTTP-статус + `{"error": {"code": "snake_case", "message": "текст для человека", "details": <any>}}` |
| Долгие операции | `202 Accepted` + `jobId`; прогресс — `GET /jobs/{id}` (фронтенд опрашивает раз в ~0,7 с, пока есть активные задачи) |
| Загрузка файлов | `multipart/form-data`, поле `file` |
| Скачивание файлов | обычный `GET` с `Content-Disposition: attachment; filename="..."` |

Коды ошибок, которые фронтенд отображает осмысленно: `validation_error` (422), `invalid_spec` (422, `details` — `SpecIssue[]`),
`unsupported_format` (422), `invalid_action_log` (422), `message_limit` (422), `capture_too_large` (413),
`duplicate_capture` (409), `no_changes` (409), `not_cancellable` (409), `source_mismatch` (409), `not_found` (404).
Клиент не повторяет запросы при 4xx.

Защита локального сервера (`internal/server/server.go`): запросы с `Host`, отличным от loopback
(`127.0.0.1`, `localhost`, `[::1]`), отклоняются `403` — это закрывает DNS rebinding. Изменяющие запросы
(`POST/PATCH/DELETE`) с `Sec-Fetch-Site: cross-site` или с `Origin` чужого сайта отклоняются `403` —
это закрывает CSRF из браузера. Если сервер явно запущен на не-loopback адресе (`--addr 0.0.0.0:7420`),
проверка `Host` отключается, о чём пишется предупреждение в журнал.

## Эндпоинты

### Проекты

| Метод и путь | Тело / параметры | Ответ |
|---|---|---|
| `GET /projects` | — | `ListResponse<Project>` |
| `POST /projects` | `CreateProjectInput` | `201 Project` |
| `GET /projects/{projectId}` | — | `Project` (со `stats`) |
| `PATCH /projects/{projectId}` | `{settings?: Partial<ProjectSettings>, name?, description?}` | `Project` |
| `POST /projects/import` | multipart `file` (архив проекта) | `201 Project`, импорт идёт задачей `import_project` |
| `GET /projects/{projectId}/export` | — | архив проекта (`.zip`), см. «Перенос проекта» |
| `POST /demo` | — | `201 Project` — демо-проект ThermoLink TC-200 (записи, журнал, интерпретация v1–v3, проверки) |

### Записи трафика и сессии

| Метод и путь | Тело / параметры | Ответ |
|---|---|---|
| `GET /projects/{projectId}/captures` | — | `ListResponse<Capture>` |
| `GET /captures/{captureId}` | — | `Capture` |
| `POST /projects/{projectId}/captures` | multipart `file` (PCAP/PCAPNG) | `202 {capture, jobId}` — `capture.status = "indexing"` |
| `GET /projects/{projectId}/sessions` | `captureId?`, `status?` (`complete\|partial\|error`), `target?` (`true\|false`), `q?` | `ListResponse<TcpSession>` |
| `GET /sessions/{sessionId}` | — | `TcpSession` |
| `GET /sessions/{sessionId}/packets` | — | `ListResponse<Packet>` |
| `GET /sessions/{sessionId}/streams/{dir}` | `dir` = `c2s\|s2c` | `DirectedStream` (метаданные: сегменты, пропуски, конфликты) |
| `GET /sessions/{sessionId}/streams/{dir}/bytes` | `offset?`, `length?` | `application/octet-stream` + заголовок `X-Stream-Length` |

> **Пропуски в потоке.** В бинарном ответе `/bytes` байты пропусков заполнены нулями *только для
> транспорта* (в памяти сервера на их месте тоже нули, но движок разбора знает их диапазоны). Их значение неизвестно; фронтенд накладывает `DirectedStream.gaps` и показывает `??`.
> Движок разбора тоже обязан считать эти байты неизвестными (поле, задевающее пропуск, получает
> `status: "unknown"`).

### Сообщения (кадрирование и разбор)

| Метод и путь | Параметры | Ответ |
|---|---|---|
| `GET /sessions/{sessionId}/messages` | `versionId` (обязательно) | `ListResponse<Message>` — оба направления, по времени |
| `GET /projects/{projectId}/messages` | `versionId`, `ruleId?`, `direction?`, `sessionIds?` (через запятую) | `ListResponse<Message>` по целевым сессиям — для сравнения примеров |

### Журнал действий

| Метод и путь | Тело | Ответ |
|---|---|---|
| `GET /projects/{projectId}/actions` | — | `ListResponse<ActionEntry>` (по времени) |
| `POST /projects/{projectId}/actions` | `CreateActionInput` | `201 ActionEntry` |
| `POST /projects/{projectId}/actions/import` | multipart `file`: CSV или JSON | `ImportActionsResult` |

CSV: `timestamp,kind,label,params,result[,observed]`; `params` и `observed` — `k=v;k=v` или JSON-объект
(значения — числа или строки). Первая строка с `timestamp` в первой колонке считается заголовком.
JSON — массив `CreateActionInput`. Ошибка в любой записи отклоняет весь файл (`422 invalid_action_log`,
в `message` — номер строки). `kind` ∈ `read_params | set_param | get_measurement | read_log | other`
(неизвестный — `other`). Сессии привязываются автоматически: из целевых сессий, начавшихся в окне
от 0,5 с до действия до 5 с после него, выбирается ближайшая по времени
(`links[].method = "auto_time"`, `deltaMs` — начало сессии минус время действия).

### Интерпретации

| Метод и путь | Тело | Ответ |
|---|---|---|
| `GET /projects/{projectId}/interpretations` | — | `ListResponse<Interpretation>` |
| `POST /projects/{projectId}/interpretations` | `{name, description?, spec}` | `201 Interpretation` (создаётся v1) |
| `GET /interpretations/{id}` | — | `Interpretation` |
| `GET /interpretations/{id}/versions` | — | `ListResponse<InterpretationVersion>` (новые сверху) |
| `POST /interpretations/{id}/versions` | `{spec, comment}` | `201 InterpretationVersion`; `422 invalid_spec`; `409 no_changes`, если хеш совпал с последней версией |
| `GET /interpretation-versions/{versionId}` | — | `InterpretationVersion` |
| `GET /interpretation-versions/{versionId}/export` | — | JSON-файл спецификации |
| `POST /interpretations/validate` | `{spec}` | `ValidationResult` (всегда `200`) |
| `POST /interpretations/preview` | `{spec, sessionId}` | `ListResponse<Message>` — применение несохранённого черновика |

Формат спецификации — [`interpretation-format.md`](interpretation-format.md).

### Гипотезы и наблюдения

| Метод и путь | Тело / параметры | Ответ |
|---|---|---|
| `GET /projects/{projectId}/hypotheses` | — | `ListResponse<Hypothesis>` |
| `POST /projects/{projectId}/hypotheses` | `CreateHypothesisInput` | `201 Hypothesis` (`status = "proposed"`) |
| `PATCH /hypotheses/{id}` | `UpdateHypothesisInput` | `Hypothesis`; смена `status` без `note` → `422` |
| `GET /projects/{projectId}/observations` | `hypothesisId?`, `sessionId?` | `ListResponse<Observation>` (новые сверху) |
| `POST /projects/{projectId}/observations` | `CreateObservationInput` | `201 Observation`; `409 source_mismatch`, если `captureSha256` не совпал |
| `DELETE /observations/{id}` | — | `204` |

### Проверка

| Метод и путь | Тело / параметры | Ответ |
|---|---|---|
| `GET /projects/{projectId}/corpora` | — | `ListResponse<Corpus>` |
| `POST /projects/{projectId}/corpora` | `CreateCorpusInput` | `201 Corpus` |
| `GET /projects/{projectId}/runs` | — | `ListResponse<VerificationRun>` (новые сверху) |
| `POST /projects/{projectId}/runs` | `{versionId, corpusId}` | `202 {run, jobId}` |
| `GET /runs/{runId}` | — | `VerificationRun` |
| `GET /runs/{runId}/findings` | `kind?` | `ListResponse<Finding>` |

### Отчёты, задачи, система

| Метод и путь | Тело / параметры | Ответ |
|---|---|---|
| `GET /projects/{projectId}/reports` | — | `ListResponse<Report>` |
| `POST /projects/{projectId}/reports` | `{title, versionId, runIds}` | `202 {report, jobId}` |
| `GET /reports/{reportId}/files/{fileName}` | — | файл артефакта (`ReportArtifact.url` указывает сюда) |
| `GET /jobs` | `limit?` (по умолчанию 20) | `ListResponse<Job>` — последние задачи всех проектов |
| `GET /jobs/{jobId}` | — | `Job` |
| `POST /jobs/{jobId}/cancel` | — | `Job`; `409 not_cancellable`, если задача уже завершена |
| `GET /system/info` | — | `SystemInfo` (сборка, защитные пределы, `runtime.MemStats`, каталог данных) |

## Обязательная семантика

Без этих свойств UI будет показывать неверные выводы. Каждое свойство напрямую следует из ТЗ.

1. **Неизменяемость источников.** Исходный файл записи хранится как есть, `Capture.sha256` считается
   при импорте. `integrity` сообщает результат сверки при открытии проекта (`ok | mismatch | unchecked`).
2. **Сессии.** Каждый экземпляр TCP-соединения — отдельная `TcpSession`. Повторное использование
   4-tuple не объединяет соединения. `roles = "handshake"`, только если роли установлены по SYN.
3. **Восстановление потока.** Порядок — по TCP sequence numbers (RFC 9293). Повторные передачи
   не дублируют байты (`flags: ["retransmission"]`). Перекрытия с разными данными — `OverlapConflict`,
   копия выбирается по `ProjectSettings.overlapPolicy`. Потери и усечения — `StreamGap` с `reason`.
   Если начало не захвачено, `startKnown = false`: смещение 0 — первый увиденный байт.
4. **Диагностика.** Неподдерживаемые и неполные данные описываются в `diagnostics` (стабильный `code`).
   Ошибка одной сессии не блокирует остальные.
5. **Версии интерпретаций неизменяемы.** `hash` — SHA-256 канонического JSON спецификации.
6. **Устаревание проверок.** `VerificationRun.stale = true` вместе с `staleReasons[]`, если:
   у интерпретации есть версия новее `run.version`; изменился `corpus.hash`; изменились настройки
   восстановления проекта относительно `run.settings`. Старый результат не удаляется и не выдаётся за актуальный.
7. **Сводка проверки** (`RunSummary`) учитывает весь корпус: нарушения, неполные, неоднозначные и
   нераспознанные сообщения, неописанные байты, неприменимые правила (`inapplicableRules`).
   `previousRunId` — предыдущая успешная проверка той же интерпретации на том же корпусе.
8. **Происхождение.** Каждый `Finding.examples[]` и `Observation.source` — это `SourceRef`
   (запись + sha256 + сессия + направление + диапазон + номера пакетов). `Observation.snapshotHex` —
   байты в момент создания (неизвестные — `??`). `integrity = "mismatch"`, если байты по ссылке изменились
   (например, после смены политики перекрытий).
9. **Ресурсы.** Пределы из `SystemInfo.limits` проверяются явно: превышение — ошибка/диагностика,
   а не скрытое обрезание.

## Перенос проекта

`GET /projects/{id}/export` отдаёт zip-архив (`<имя>.protolab.zip`) — это каталог проекта как есть,
с относительными путями:

```text
project.json                 # всё состояние: проект, записи, журнал, интерпретации и версии,
                             # гипотезы, наблюдения, корпуса, проверки с находками, отчёты
captures/<sha256>.pcapng     # исходные записи без изменений (хранятся в архиве без сжатия)
reports/<reportId>/...       # артефакты отчётов
```

Сессии, потоки и сообщения в архив не попадают: они детерминированно восстанавливаются из записей при
первом обращении. Поэтому после импорта (`POST /projects/import`) запускается задача `import_project`,
которая заново разбирает записи и сверяет `sha256`.

Импорт проверяет архив: допускаются только `project.json`, `captures/` и `reports/`; абсолютные пути,
`..` и `:` в именах отклоняются; общий объём распакованных данных ограничен. Если в каталоге данных уже
есть проект с теми же идентификаторами (импорт собственного экспорта), все идентификаторы
перевыпускаются, ссылки между сущностями сохраняются.

## Раздача фронтенда из Go

`npm run build` складывает сборку в `internal/webui/dist`, а `internal/webui/webui.go` встраивает её в
бинарь директивой `//go:embed all:dist`. `internal/server/static.go` отдаёт файлы как SPA: существующий
файл — как есть (`/assets/*` с `Cache-Control: immutable`), `index.html` — с `no-cache`, любой другой путь
вне `/api/` — `index.html` (клиентский роутинг), отсутствующий `/assets/*` — `404`. Если бинарь собран без
фронтенда, вместо интерфейса отдаётся страница-подсказка, API при этом работает.
