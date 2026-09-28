from fastapi import APIRouter
from sqlalchemy import text

from app.api.schemas import HealthOut
from app.db import oracle
from app.db.postgres import engine

router = APIRouter(prefix="/api", tags=["health"])


@router.get("/health", response_model=HealthOut)
def health() -> HealthOut:
    return HealthOut(postgres=_check_postgres(), oracle=_check_oracle())


def _check_postgres() -> str:
    try:
        with engine.connect() as conn:
            conn.execute(text("SELECT 1"))
        return "ok"
    except Exception as exc:
        return f"error: {exc}"


def _check_oracle() -> str:
    try:
        with oracle.connect() as conn, conn.cursor() as cur:
            cur.execute("SELECT 1 FROM dual")
        return "ok"
    except Exception as exc:
        return f"error: {exc}"
