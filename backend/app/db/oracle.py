"""Подключение к Oracle (только чтение).

Используется thin-режим python-oracledb: Oracle Instant Client не нужен.
Импорт выполняется редко (раз в год), поэтому пул соединений не держим —
открываем соединение на время импорта и закрываем.
"""

from collections.abc import Callable
from contextlib import AbstractContextManager

import oracledb

from app.config import get_settings

OracleConnectionFactory = Callable[[], AbstractContextManager[oracledb.Connection]]


class OracleNotConfiguredError(RuntimeError):
    pass


def connect() -> oracledb.Connection:
    settings = get_settings()
    if not (settings.oracle_user and settings.oracle_dsn):
        raise OracleNotConfiguredError(
            "Oracle не настроен: задайте ORACLE_USER, ORACLE_PASSWORD и ORACLE_DSN"
        )
    return oracledb.connect(
        user=settings.oracle_user,
        password=settings.oracle_password,
        dsn=settings.oracle_dsn,
    )
