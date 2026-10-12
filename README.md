# ProtoLab — лаборатория протоколов

Локальный инструмент для исследования недокументированного бинарного обмена с устройством поверх TCP.
Инженер открывает записи трафика (PCAP/PCAPNG) и сопоставляет их с журналом действий в штатном клиенте.
Затем он формулирует интерпретацию, проверяет её на дополнительных записях и передаёт воспроизводимое
исследование другому инженеру.

ProtoLab — один исполняемый файл без зависимостей. При запуске в терминале открывается
терминальный интерфейс (TUI) с состоянием сервера. Клавиша `o` открывает подробный веб-интерфейс
в браузере. Веб-интерфейс встроен в бинарь и работает только на `127.0.0.1`, без интернета.

## Установка

Скачайте архив для своей системы со страницы [Releases](https://github.com/AVGsync/protolab/releases).
Каждое слияние в `main` публикует новую версию `v0.1.N`.

| Система | Архив |
|---|---|
| Linux x86-64 / ARM64 | `protolab_<версия>_linux_amd64.tar.gz` / `..._linux_arm64.tar.gz` |
| macOS Intel / Apple Silicon | `protolab_<версия>_darwin_amd64.tar.gz` / `..._darwin_arm64.tar.gz` |
| Windows x86-64 / ARM64 | `protolab_<версия>_windows_amd64.zip` / `..._windows_arm64.zip` |

```bash
sha256sum -c SHA256SUMS --ignore-missing      # проверить загрузку
tar -xzf protolab_*_linux_amd64.tar.gz
./protolab_*_linux_amd64/protolab
```

Бинари не подписаны. На macOS снимите карантин: `xattr -d com.apple.quarantine protolab`.
На Windows SmartScreen может попросить подтверждения.

## Запуск

```bash
protolab                 # сервер + терминальный интерфейс
protolab --demo          # то же, и создать демо-проект ThermoLink TC-200, если проектов ещё нет
protolab --no-tui        # без TUI (сервисы, CI, Docker): журнал в stderr, Ctrl+C — выход
```

Терминальный интерфейс:

| Клавиша | Действие |
|---|---|
| `o` / `Enter` | открыть веб-интерфейс в браузере |
| `c` | скопировать адрес (OSC 52 — работает и по SSH) |
| `d` | создать демо-проект |
| `l` | журнал во весь экран |
| `?` | справка |
| `q` / `Ctrl+C` | остановить сервер и выйти |

Если stdout не терминал (перенаправление в файл, systemd), TUI не запускается: адрес печатается
в stderr, сервер работает до `SIGINT`/`SIGTERM`.

Флаги сервера:

| Флаг | По умолчанию | Назначение |
|---|---|---|
| `--addr` | `127.0.0.1:7420` | адрес HTTP; если порт по умолчанию занят, берётся свободный |
| `--data` | см. ниже | каталог данных |
| `--open` | — | сразу открыть браузер |
| `--demo` | — | создать демо-проект, если проектов нет |
| `--no-tui` | — | без терминального интерфейса |
| `--max-capture-mb` | `512` | предел размера одной записи |
| `--debug` | — | подробный журнал |

Каталог данных: `$PROTOLAB_DATA`, иначе `~/.local/share/protolab` (Linux, учитывает `XDG_DATA_HOME`),
`~/Library/Application Support/ProtoLab` (macOS), `%LOCALAPPDATA%\ProtoLab` (Windows).
Каждый проект — папка `projects/<id>/` с `project.json`, исходными записями `captures/<sha256>.pcapng`
и отчётами `reports/`.

## Командная строка без интерфейса

```bash
protolab demo [--data DIR]                     # создать демо-проект и выйти
protolab apply --spec interpretation.json --input capture.pcapng --out results.json [--port 5020] [--strict]
protolab verify --examples examples.<имя>.v3.json   # повторить проверку из отчёта
protolab version
```

`apply` разбирает запись по интерпретации тем же движком, что и веб-интерфейс, и пишет JSON с
сообщениями и сводкой. С `--strict` код выхода `3`, если есть нарушения или неполные сообщения.
`verify` берёт файл примеров из отчёта, находит исходные записи по `sha256`, заново применяет
интерпретацию и сравнивает сводку с сохранённой. При расхождении код выхода `1`.

## Структура репозитория

```text
protolab/
├── cmd/protolab/        # main: TUI + сервер, команды demo/apply/verify
├── internal/
│   ├── pcap/            # чтение PCAP и PCAPNG (без libpcap и cgo)
│   ├── netpkt/          # разбор Ethernet/SLL/IPv4/TCP, контрольные суммы
│   ├── tcpflow/         # сборка TCP-сессий и направленных потоков по sequence numbers
│   ├── spec/            # формат интерпретации protolab/v1 и его проверка
│   ├── engine/          # кадрирование, разбор полей, проверка утверждений
│   ├── app/             # прикладной слой: проекты, анализ, проверки, отчёты
│   ├── store/           # хранение на диске, экспорт и импорт архивов
│   ├── jobs/            # фоновые задачи с прогрессом и отменой
│   ├── report/          # Markdown/JSON-отчёты
│   ├── server/          # HTTP API и раздача веб-интерфейса
│   ├── tui/             # терминальный интерфейс (Bubble Tea)
│   ├── demo/            # генератор демо-данных ThermoLink TC-200
│   └── webui/           # встраиваемая сборка фронтенда (go:embed)
├── frontend/            # React + TypeScript (Vite), Feature-Sliced Design
└── docs/
    ├── backend.md                # как устроен Go-код (подробно)
    ├── api.md                    # REST API frontend ↔ backend
    └── interpretation-format.md  # формат интерпретации protolab/v1
```

## Сборка из исходников

Нужны Go (версия в `go.mod`) и Node.js ≥ 20.19.

```bash
make build              # фронтенд + бинарь bin/protolab с встроенным интерфейсом
make run                # собрать и запустить
make test               # go test -race + vitest
make check              # всё, что проверяет CI
make release-local      # шесть платформ в dist/, как в релизе
```

Разработка интерфейса: запустите `protolab --no-tui` и `make web-dev` — Vite на
`http://localhost:5173` проксирует `/api` на `127.0.0.1:7420`. Без бэкенда: `cd frontend && npm run dev:mock`
(эмулированное API на MSW).

## Релизы

`.github/workflows/release.yml` запускается на каждый push в `main`. Он прогоняет те же проверки, что и
pull request (`ci.yml`), собирает фронтенд один раз и кросс-компилирует шесть бинарей с `CGO_ENABLED=0`.
Затем проверяет архивы (контрольные суммы, версию в бинаре, наличие веб-интерфейса) и создаёт тег
`v<VERSION>.<номер запуска>` и GitHub Release с заметками о слитых PR. Мажорная и минорная части версии
лежат в файле `VERSION`.

Подробное описание того, как работает Go-код, — в [docs/backend.md](docs/backend.md).
