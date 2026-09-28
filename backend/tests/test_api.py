import pytest
from fastapi.testclient import TestClient

from app.db.postgres import get_session
from app.main import app
from app.services import import_service


@pytest.fixture
def client(session_factory, monkeypatch):
    def override_session():
        with session_factory() as s:
            yield s

    started = []
    monkeypatch.setattr(import_service, "execute_import", lambda run_id: started.append(run_id))
    monkeypatch.setattr("app.main.SessionLocal", session_factory)
    app.dependency_overrides[get_session] = override_session
    with TestClient(app) as c:
        c.started = started
        yield c
    app.dependency_overrides.clear()


def test_start_and_list_imports(client):
    resp = client.post("/api/imports", json={"year": 2025})
    assert resp.status_code == 202
    run = resp.json()
    assert run["status"] == "running"
    assert client.started == [run["id"]]

    assert client.post("/api/imports", json={"year": 2025}).status_code == 409
    assert [r["id"] for r in client.get("/api/imports").json()] == [run["id"]]
    assert client.get(f"/api/imports/{run['id']}").json()["period_year"] == 2025
    assert client.get("/api/imports/999").status_code == 404


def test_rejects_invalid_year(client):
    assert client.post("/api/imports", json={"year": 1800}).status_code == 422


def test_list_sources(client):
    assert client.get("/api/imports/sources").json()[0]["target_table"] == "example_records"
