from datetime import date
from decimal import Decimal

from sqlalchemy import Date, ForeignKey, Index, Integer, Numeric, String
from sqlalchemy.orm import Mapped, mapped_column

from app.db.postgres import Base


class ExampleRecord(Base):
    """ПРИМЕР таблицы, в которую загружаются данные из Oracle.

    Замените на реальные таблицы, когда станет известна структура источника.
    Каждая загружаемая таблица должна иметь колонки ``period_year`` и
    ``import_run_id`` — по ним повторный импорт за тот же год заменяет данные.
    """

    __tablename__ = "example_records"
    __table_args__ = (Index("ix_example_records_period_year", "period_year"),)

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    period_year: Mapped[int] = mapped_column(Integer, nullable=False)
    import_run_id: Mapped[int] = mapped_column(
        ForeignKey("import_runs.id", ondelete="CASCADE"), nullable=False
    )

    source_id: Mapped[int] = mapped_column(Integer, nullable=False)
    name: Mapped[str] = mapped_column(String(255), nullable=False)
    amount: Mapped[Decimal | None] = mapped_column(Numeric(18, 2))
    doc_date: Mapped[date | None] = mapped_column(Date)
