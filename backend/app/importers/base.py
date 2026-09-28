from dataclasses import dataclass

from app.db.postgres import Base


@dataclass(frozen=True)
class Importer:
    """Описание одного источника: SQL-запрос к Oracle и таблица в PostgreSQL.

    Требования к ``source_sql``:
      * параметр ``:year`` — год, за который загружаются данные;
      * псевдонимы колонок в SELECT совпадают с именами колонок целевой таблицы
        (регистр не важен: Oracle возвращает имена в верхнем регистре).

    Колонки ``period_year`` и ``import_run_id`` заполняются автоматически.
    """

    name: str
    title: str
    target: type[Base]
    source_sql: str
