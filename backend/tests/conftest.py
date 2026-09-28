from contextlib import contextmanager

import pytest
from sqlalchemy import create_engine, event
from sqlalchemy.orm import sessionmaker
from sqlalchemy.pool import StaticPool

from app.db.postgres import Base


@pytest.fixture
def session_factory():
    engine = create_engine(
        "sqlite://", connect_args={"check_same_thread": False}, poolclass=StaticPool
    )
    event.listen(engine, "connect", lambda c, _: c.execute("PRAGMA foreign_keys=ON"))
    Base.metadata.create_all(engine)
    yield sessionmaker(bind=engine, autoflush=False, expire_on_commit=False)
    engine.dispose()


class FakeCursor:
    def __init__(self, tables):
        self._tables = tables
        self.arraysize = 100
        self.description = None
        self._rows = []
        self.executed = []

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False

    def execute(self, sql, **params):
        self.executed.append((sql, params))
        columns, rows = self._tables[sql.strip()]
        if isinstance(rows, Exception):
            raise rows
        self.description = [(c.upper(),) for c in columns]
        self._rows = [r for r in rows if r[-1] == params["year"]]
        self._rows = [r[:-1] for r in self._rows]
        self.description = self.description[:-1]

    def fetchmany(self, size):
        batch, self._rows = self._rows[:size], self._rows[size:]
        return batch


class FakeOracle:
    """Имитация Oracle: sql -> (колонки, строки). Последняя колонка — год."""

    def __init__(self, tables):
        self.tables = tables
        self.cursors = []

    @contextmanager
    def connect(self):
        yield self

    def cursor(self):
        cur = FakeCursor(self.tables)
        self.cursors.append(cur)
        return cur
