from datetime import date
from decimal import Decimal

import pytest
from sqlalchemy import select

from app.importers import Importer
from app.models import ExampleRecord, ImportRun, ImportStatus
from app.services import import_service
from tests.conftest import FakeOracle

SQL = "SELECT ... WHERE year = :year"
COLUMNS = ["source_id", "name", "amount", "doc_date", "year"]
IMPORTER = Importer(name="example", title="Пример", target=ExampleRecord, source_sql=SQL)


def rows(year, n):
    return [(i, f"row {i}", Decimal("1.50"), date(year, 1, 1), year) for i in range(n)]


def run_import(session_factory, oracle, year):
    with session_factory() as s:
        run_id = import_service.start_import(s, year).id
    import_service.execute_import(
        run_id,
        oracle_connect=oracle.connect,
        session_factory=session_factory,
        importers=[IMPORTER],
    )
    with session_factory() as s:
        return s.get(ImportRun, run_id)


def records(session_factory, year):
    with session_factory() as s:
        return s.scalars(select(ExampleRecord).where(ExampleRecord.period_year == year)).all()


def test_import_loads_rows_for_year(session_factory):
    oracle = FakeOracle({SQL: (COLUMNS, rows(2025, 7) + rows(2024, 3))})

    run = run_import(session_factory, oracle, 2025)

    assert run.status == ImportStatus.success
    assert run.details == {"example": {"rows": 7}}
    assert run.finished_at is not None
    loaded = records(session_factory, 2025)
    assert len(loaded) == 7
    assert {r.import_run_id for r in loaded} == {run.id}
    assert oracle.cursors[0].executed == [(SQL, {"year": 2025})]


def test_reimport_same_year_replaces_data_and_keeps_other_years(session_factory):
    oracle = FakeOracle({SQL: (COLUMNS, rows(2025, 5) + rows(2024, 2))})
    run_import(session_factory, oracle, 2024)
    run_import(session_factory, oracle, 2025)

    oracle.tables[SQL] = (COLUMNS, rows(2025, 3))
    second = run_import(session_factory, oracle, 2025)

    assert [r.import_run_id for r in records(session_factory, 2025)] == [second.id] * 3
    assert len(records(session_factory, 2024)) == 2


def test_failed_import_rolls_back_and_records_error(session_factory):
    oracle = FakeOracle({SQL: (COLUMNS, rows(2025, 4))})
    run_import(session_factory, oracle, 2025)

    oracle.tables[SQL] = (COLUMNS, RuntimeError("ORA-00942: table or view does not exist"))
    run = run_import(session_factory, oracle, 2025)

    assert run.status == ImportStatus.failed
    assert "ORA-00942" in run.error
    # предыдущие данные за год не удалены
    assert len(records(session_factory, 2025)) == 4


def test_only_one_import_can_run(session_factory):
    with session_factory() as s:
        import_service.start_import(s, 2025)
        with pytest.raises(import_service.ImportAlreadyRunningError):
            import_service.start_import(s, 2025)


def test_interrupted_runs_are_marked_failed(session_factory):
    with session_factory() as s:
        run = import_service.start_import(s, 2025)
        import_service.fail_interrupted_runs(s)
        s.refresh(run)
        assert run.status == ImportStatus.failed
