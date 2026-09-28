# Matrix

Веб-приложение: данные загружаются из **Oracle** (только чтение) в **PostgreSQL**,
дальше вся работа и доработки ведутся на PostgreSQL.

Импорт запускается **по кнопке** в интерфейсе — раз в год, после Нового года,
с выбором года (по умолчанию — прошлый).

## Стек

| Часть     | Технологии                                                      |
|-----------|-----------------------------------------------------------------|
| Backend   | Python 3.11+, FastAPI, SQLAlchemy 2, Alembic, python-oracledb   |
| Frontend  | React 19, TypeScript, Vite                                      |
| БД        | PostgreSQL 16 (основная), Oracle (источник)                     |

python-oracledb работает в **thin-режиме** — Oracle Instant Client устанавливать не нужно.

## Структура

```
backend/
  app/
    api/            REST API (/api/health, /api/imports)
    db/             подключения к PostgreSQL и Oracle
    models/         таблицы PostgreSQL (SQLAlchemy)
    importers/      описание источников: SQL-запрос к Oracle → таблица PostgreSQL
    services/       логика импорта
  alembic/          миграции схемы PostgreSQL
  tests/
frontend/
  src/              React-приложение
docker-compose.yml  PostgreSQL (+ локальный Oracle для разработки)
```

## Как работает импорт

1. Пользователь выбирает год и нажимает «Запустить импорт».
2. Создаётся запись в `import_runs` (статус `running`); одновременно может идти только один импорт.
3. В фоне для каждого источника из `app/importers/registry.py` выполняется SQL-запрос к Oracle
   с параметром `:year`, строки пачками (`IMPORT_BATCH_SIZE`) вставляются в PostgreSQL.
4. Всё выполняется **в одной транзакции**: либо загружены все источники, либо ничего.
5. Повторный импорт за тот же год **заменяет** данные этого года, другие годы не трогаются.
6. В истории видно статус, время, количество строк и текст ошибки.

## Запуск для разработки

```bash
# 1. PostgreSQL
docker compose up -d postgres
# (опционально) локальный Oracle: docker compose --profile oracle up -d

# 2. Backend
cd backend
python -m venv .venv && source .venv/bin/activate
pip install -e ".[dev]"
cp .env.example .env          # укажите параметры Oracle
alembic upgrade head
uvicorn app.main:app --reload # http://localhost:8000/docs

# 3. Frontend
cd frontend
npm install
npm run dev                   # http://localhost:5173
```

Проверки backend: `pytest` и `ruff check .`

## Как добавить реальный источник из Oracle

Сейчас в реестре есть только **пример** (`example_records`) — его нужно заменить.

1. Модель таблицы в `backend/app/models/` — обязательно с колонками `period_year` и `import_run_id`
   (см. `example.py`).
2. Миграция: `alembic revision --autogenerate -m "add <table>"` → `alembic upgrade head`.
3. Запись `Importer(...)` в `backend/app/importers/registry.py`: SQL к Oracle с параметром `:year`,
   псевдонимы колонок в `SELECT` = имена колонок таблицы в PostgreSQL.

Собственные доработки (справочники, правки, расчёты) — отдельные таблицы PostgreSQL,
которые ссылаются на загруженные данные; импорт их не затрагивает.
