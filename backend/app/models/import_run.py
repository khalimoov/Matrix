import enum
from datetime import datetime

from sqlalchemy import JSON, DateTime, Enum, Index, Integer, Text, func, text
from sqlalchemy.orm import Mapped, mapped_column

from app.db.postgres import Base


class ImportStatus(enum.StrEnum):
    running = "running"
    success = "success"
    failed = "failed"


class ImportRun(Base):
    """Журнал запусков импорта из Oracle."""

    __tablename__ = "import_runs"
    __table_args__ = (
        # Одновременно может выполняться только один импорт.
        Index(
            "uq_import_runs_single_running",
            "status",
            unique=True,
            postgresql_where=text("status = 'running'"),
            sqlite_where=text("status = 'running'"),
        ),
    )

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    period_year: Mapped[int] = mapped_column(Integer, nullable=False)
    status: Mapped[ImportStatus] = mapped_column(
        Enum(ImportStatus, name="import_status", native_enum=False, length=16),
        nullable=False,
        default=ImportStatus.running,
    )
    started_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, server_default=func.now()
    )
    finished_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    # Результат по каждому источнику: {"<имя источника>": {"rows": N}}
    details: Mapped[dict] = mapped_column(JSON, nullable=False, default=dict)
    error: Mapped[str | None] = mapped_column(Text)
