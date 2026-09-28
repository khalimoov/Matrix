from datetime import datetime

from pydantic import BaseModel, ConfigDict, Field

from app.models import ImportStatus


class ImportRequest(BaseModel):
    year: int = Field(ge=1990, le=2100)


class ImportRunOut(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    period_year: int
    status: ImportStatus
    started_at: datetime
    finished_at: datetime | None
    details: dict
    error: str | None


class SourceOut(BaseModel):
    name: str
    title: str
    target_table: str


class HealthOut(BaseModel):
    postgres: str
    oracle: str
