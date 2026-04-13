"""Tests for CommunityProvider — uses a mock agent-api server."""

import json
import threading
import time
from http.server import HTTPServer, BaseHTTPRequestHandler

import pytest

from provider import CommunityProvider


class MockAgentAPI(BaseHTTPRequestHandler):
    """Minimal mock of the agent-api task endpoints."""

    tasks = {}

    def do_POST(self):
        if self.path == "/api/tasks":
            length = int(self.headers.get("Content-Length", 0))
            body = json.loads(self.rfile.read(length))
            task_id = "test-task-001"
            MockAgentAPI.tasks[task_id] = {
                "id": task_id,
                "model": body["model"],
                "messages": body["messages"],
                "max_tokens": body.get("max_tokens", 300),
                "status": "queued",
                "result": None,
            }
            # Simulate agent completing the task after a short delay
            threading.Timer(0.5, self._complete_task, args=[task_id]).start()
            self.send_response(201)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"task_id": task_id}).encode())
        else:
            self.send_response(404)
            self.end_headers()

    def do_GET(self):
        if self.path.startswith("/api/tasks/"):
            task_id = self.path.split("/")[-1]
            task = MockAgentAPI.tasks.get(task_id)
            if task:
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(json.dumps(task).encode())
            else:
                self.send_response(404)
                self.end_headers()
        else:
            self.send_response(404)
            self.end_headers()

    @staticmethod
    def _complete_task(task_id):
        if task_id in MockAgentAPI.tasks:
            MockAgentAPI.tasks[task_id]["status"] = "done"
            MockAgentAPI.tasks[task_id]["result"] = {
                "content": "The capital of Portugal is Lisbon.",
                "tokens_used": 42,
            }

    def log_message(self, format, *args):
        pass  # Silence logs during tests


@pytest.fixture(scope="module")
def mock_server():
    MockAgentAPI.tasks = {}
    server = HTTPServer(("127.0.0.1", 19090), MockAgentAPI)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    yield server
    server.shutdown()


@pytest.fixture(autouse=True)
def set_api_url(monkeypatch):
    monkeypatch.setenv("AGENT_API_URL", "http://127.0.0.1:19090")


def test_sync_completion(mock_server):
    import provider
    provider.AGENT_API_URL = "http://127.0.0.1:19090"

    p = CommunityProvider()
    result = p.completion(
        model="community/gemma4:e4b",
        messages=[{"role": "user", "content": "What is the capital of Portugal?"}],
    )
    assert result.choices[0].message.content == "The capital of Portugal is Lisbon."
    assert result.usage["completion_tokens"] == 42


@pytest.mark.asyncio
async def test_async_completion(mock_server):
    import provider
    provider.AGENT_API_URL = "http://127.0.0.1:19090"
    MockAgentAPI.tasks = {}

    p = CommunityProvider()
    result = await p.acompletion(
        model="community/gemma4:e4b",
        messages=[{"role": "user", "content": "What is the capital of Portugal?"}],
    )
    assert result.choices[0].message.content == "The capital of Portugal is Lisbon."
    assert result.usage["completion_tokens"] == 42


def test_extract_model():
    p = CommunityProvider()
    assert p._extract_model("community/gemma4:e4b") == "gemma4:e4b"
    assert p._extract_model("gemma4:e4b") == "gemma4:e4b"
    assert p._extract_model("community/qwen3.5:9B") == "qwen3.5:9B"


def test_format_messages():
    p = CommunityProvider()
    msgs = [
        {"role": "user", "content": "hello"},
        {"role": "assistant", "content": "hi"},
    ]
    formatted = p._format_messages(msgs)
    assert len(formatted) == 2
    assert formatted[0]["role"] == "user"
    assert formatted[1]["content"] == "hi"
