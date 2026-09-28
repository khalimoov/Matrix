import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.api import health, imports
from app.config import get_settings
from app.db.postgres import SessionLocal
from app.services.import_service import fail_interrupted_runs

logging.basicConfig(level=logging.INFO)


@asynccontextmanager
async def lifespan(_: FastAPI):
    with SessionLocal() as session:
        fail_interrupted_runs(session)
    yield


app = FastAPI(title="Matrix", lifespan=lifespan)
app.add_middleware(
    CORSMiddleware,
    allow_origins=get_settings().cors_origins,
    allow_methods=["*"],
    allow_headers=["*"],
)
app.include_router(health.router)
app.include_router(imports.router)
