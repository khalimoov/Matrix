from typing import Annotated

from fastapi import APIRouter, BackgroundTasks, Depends, HTTPException
from sqlalchemy.orm import Session

from app.api.schemas import ImportRequest, ImportRunOut, SourceOut
from app.db.postgres import get_session
from app.importers import IMPORTERS
from app.models import ImportRun
from app.services import import_service

SessionDep = Annotated[Session, Depends(get_session)]

router = APIRouter(prefix="/api/imports", tags=["imports"])


@router.get("/sources", response_model=list[SourceOut])
def list_sources() -> list[SourceOut]:
    return [
        SourceOut(name=i.name, title=i.title, target_table=i.target.__tablename__)
        for i in IMPORTERS
    ]


@router.get("", response_model=list[ImportRunOut])
def list_runs(session: SessionDep):
    return import_service.list_runs(session)


@router.get("/{run_id}", response_model=ImportRunOut)
def get_run(run_id: int, session: SessionDep):
    run = session.get(ImportRun, run_id)
    if run is None:
        raise HTTPException(status_code=404, detail="Запуск не найден")
    return run


@router.post("", response_model=ImportRunOut, status_code=202)
def start_import(
    body: ImportRequest,
    background: BackgroundTasks,
    session: SessionDep,
):
    try:
        run = import_service.start_import(session, body.year)
    except import_service.ImportAlreadyRunningError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    background.add_task(import_service.execute_import, run.id)
    return run
