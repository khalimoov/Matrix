import logging
from collections.abc import Callable, Sequence
from datetime import UTC, datetime

from sqlalchemy import delete, insert, select, update
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from app.config import get_settings
from app.db import oracle
from app.db.postgres import SessionLocal
from app.importers import IMPORTERS, Importer
from app.models import ImportRun, ImportStatus

logger = logging.getLogger(__name__)


class ImportAlreadyRunningError(Exception):
    pass


def start_import(session: Session, year: int) -> ImportRun:
    """Регистрирует новый запуск. Сам импорт выполняет ``execute_import``."""
    run = ImportRun(period_year=year, status=ImportStatus.running, details={})
    session.add(run)
    try:
        session.commit()
    except IntegrityError as exc:
        session.rollback()
        raise ImportAlreadyRunningError("Импорт уже выполняется") from exc
    return run


def execute_import(
    run_id: int,
    *,
    oracle_connect: Callable = oracle.connect,
    session_factory: Callable[[], Session] = SessionLocal,
    importers: Sequence[Importer] = IMPORTERS,
) -> None:
    """Загружает данные за год из Oracle в PostgreSQL.

    Весь импорт выполняется в одной транзакции PostgreSQL: либо загружены все
    источники, либо ничего. Данные за этот же год от предыдущих запусков
    удаляются, поэтому импорт можно безопасно повторять.
    """
    batch_size = get_settings().import_batch_size

    with session_factory() as session:
        run = session.get(ImportRun, run_id)
        year = run.period_year
        details: dict[str, dict] = {}
        try:
            with oracle_connect() as conn:
                for imp in importers:
                    logger.info("Импорт %s за %s год", imp.name, year)
                    rows = _copy(session, conn, imp, year, run_id, batch_size)
                    details[imp.name] = {"rows": rows}
            run.status = ImportStatus.success
            run.details = details
            run.finished_at = datetime.now(UTC)
            session.commit()
        except Exception as exc:
            logger.exception("Импорт %s завершился ошибкой", run_id)
            session.rollback()
            run = session.get(ImportRun, run_id)
            run.status = ImportStatus.failed
            run.details = details
            run.error = f"{type(exc).__name__}: {exc}"
            run.finished_at = datetime.now(UTC)
            session.commit()


def _copy(session: Session, conn, imp: Importer, year: int, run_id: int, batch_size: int) -> int:
    table = imp.target.__table__
    session.execute(delete(table).where(table.c.period_year == year))

    total = 0
    with conn.cursor() as cursor:
        cursor.arraysize = batch_size
        cursor.execute(imp.source_sql, year=year)
        columns = [d[0].lower() for d in cursor.description]
        while rows := cursor.fetchmany(batch_size):
            session.execute(
                insert(table),
                [
                    {
                        **dict(zip(columns, row, strict=True)),
                        "period_year": year,
                        "import_run_id": run_id,
                    }
                    for row in rows
                ],
            )
            total += len(rows)
    return total


def fail_interrupted_runs(session: Session) -> None:
    """Помечает ошибкой запуски, прерванные перезапуском сервера."""
    session.execute(
        update(ImportRun)
        .where(ImportRun.status == ImportStatus.running)
        .values(
            status=ImportStatus.failed,
            error="Прервано перезапуском сервера",
            finished_at=datetime.now(UTC),
        )
    )
    session.commit()


def list_runs(session: Session, limit: int = 50) -> Sequence[ImportRun]:
    return session.scalars(select(ImportRun).order_by(ImportRun.id.desc()).limit(limit)).all()
