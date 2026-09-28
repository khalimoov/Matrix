"""Реестр источников, которые загружаются при нажатии «Запустить импорт».

Чтобы добавить новый источник:
  1. Создайте модель таблицы в ``app/models`` (с колонками period_year и import_run_id).
  2. Сгенерируйте миграцию: ``alembic revision --autogenerate -m "..."``.
  3. Добавьте сюда ``Importer`` с SQL-запросом к Oracle.
"""

from app.importers.base import Importer
from app.models import ExampleRecord

IMPORTERS: list[Importer] = [
    # ПРИМЕР — замените на реальный запрос и таблицу.
    Importer(
        name="example",
        title="Пример: записи за год",
        target=ExampleRecord,
        source_sql="""
            SELECT t.id        AS source_id,
                   t.name      AS name,
                   t.amount    AS amount,
                   t.doc_date  AS doc_date
              FROM some_schema.some_table t
             WHERE EXTRACT(YEAR FROM t.doc_date) = :year
        """,
    ),
]
