# protolab — Лаборатория протоколов

Локальный инструмент для исследования недокументированного бинарного обмена с устройством поверх TCP.
Инженер открывает записи трафика (PCAP/PCAPNG) и сопоставляет их с журналом действий в штатном клиенте.
Затем он формулирует интерпретацию, проверяет её на дополнительных записях и передаёт воспроизводимое
исследование другому инженеру.

```text
protolab/
├── frontend/   # React + TypeScript (Vite), Feature-Sliced Design, MSW-моки
└── docs/
    ├── api.md                    # контракт REST API frontend ↔ Go backend
    └── interpretation-format.md  # формат машиночитаемой интерпретации protolab/v1
```

## Быстрый старт (без backend)

```bash
cd frontend
npm ci
npm run dev:mock   # http://localhost:5173 — демо-проект на эмулированном API
```

С backend: `npm run dev` проксирует `/api` на `http://localhost:8080`. Production-сборка (`npm run build`)
встраивается в Go-бинарник через `embed.FS` — см. [docs/api.md](docs/api.md#раздача-фронтенда-из-go).

Подробности об архитектуре фронтенда — в [frontend/README.md](frontend/README.md).
