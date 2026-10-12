# ProtoLab REST API v1 — контракт frontend ↔ backend

Документ описывает API, которое ожидает React-фронтенд (`frontend/`). Источник истины для
структур данных — TypeScript-типы в `frontend/src/entities/*/model/types.ts`, а также
`frontend/src/shared/api/types.ts`. Эталонное поведение каждого эндпоинта реализовано в
mock-обработчиках `frontend/src/mocks/handlers/*` — их удобно читать как спецификацию.

## Общие соглашения

| Аспект | Соглашение |
|---|---|
| Базовый путь | `/api/v1` (фронтенд и API на одном origin; в dev Vite проксирует `/api` на `localhost:8080`) |
| Формат | `application/json`, поля в `camelCase` |
| Время | RFC 3339 в UTC с наносекундами (`time.RFC3339Nano`), например `2026-09-14T10:00:05.000310Z`. Фронтенд берёт доли секунды прямо из строки — не округляйте до миллисекунд |
| Идентификаторы | непрозрачные строки (`prj-…`, `cap-…`, `ses-…`); фронтенд не разбирает их |
| Списки | `{"items": [...], "total": N}` |
| Ошибки | HTTP-статус + `{"error": {"code": "snake_case", "message": "текст для человека", "details": <any>}}` |
| Долгие операции | `202 Accepted` + `jobId`; прогресс — `GET /jobs/{id}` (фронтенд опрашивает раз в ~0,7 с, пока есть активные задачи) |
| Загрузка файлов | `multipart/form-data`, поле `file` |
| Скачивание файлов | обычный `GET` с `Content-Disposition: attachment; filename="..."` |

Коды ошибок, которые фронтенд отображает осмысленно: `validation_error` (422), `invalid_spec` (422, `details` — `SpecIssue[]`),
`unsupported_format` (422), `duplicate_capture` (409), `no_changes` (409), `not_cancellable` (409),
`source_mismatch` (409), `not_found` (404). Клиент не повторяет запросы при 4xx.

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
> транспорта*. Их значение неизвестно; фронтенд накладывает `DirectedStream.gaps` и показывает `??`.
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

CSV: `timestamp,kind,label,params,result`; `params` — `k=v;k=v` или JSON-объект.
`kind` ∈ `read_params | set_param | get_measurement | read_log | other`. Сессии привязываются
автоматически: первая целевая сессия, начавшаяся не раньше чем за 0,5 с до действия и не позже чем через 5 с после него
(`links[].method = "auto_time"`, `deltaMs`).

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
| `GET /system/info` | — | `SystemInfo` (сборка, защитные пределы, `runtime.MemStats`) |

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

`GET /projects/{id}/export` отдаёт архив (`<id>.protolab.zip`) с относительными путями:

```text
project.json            # Project + settings
captures/<sha256>.pcapng
actions.json
interpretations/<id>/v<N>.json
hypotheses.json
observations.json
corpora.json
runs/<runId>.json        # run + findings
reports/<reportId>/...   # артефакты отчётов
```

В mock-режиме вместо zip отдаётся JSON с тем же содержимым (без файлов записей).

## Раздача фронтенда из Go

Собранный `frontend/dist` встраивается в бинарник. Все пути, кроме `/api/`, должны отдавать
`index.html` (fallback для SPA):

```go
//go:embed all:dist
var dist embed.FS

func spaHandler() http.Handler {
	sub, _ := fs.Sub(dist, "dist")
	files := http.FileServerFS(sub)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(sub, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
			r.URL.Path = "/" // client-side route → index.html
		}
		files.ServeHTTP(w, r)
	})
}

mux.Handle("/api/v1/", apiRouter)
mux.Handle("/", spaHandler())
```
