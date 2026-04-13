"""
CommunityProvider — LiteLLM custom provider that routes inference
to community nodes via the agent-api task queue.

LiteLLM calls this like any other backend. Inside, it enqueues a task,
waits for a community node agent to complete it, and returns the result.
"""

import os
import time
import json
import httpx
import litellm
from litellm import CustomLLM, ModelResponse, Message, Choices


AGENT_API_URL = os.getenv("AGENT_API_URL", "http://localhost:9090")
TASK_POLL_INTERVAL = float(os.getenv("COMMUNITY_POLL_INTERVAL", "0.3"))
TASK_TIMEOUT = float(os.getenv("COMMUNITY_TASK_TIMEOUT", "120"))


class CommunityProvider(CustomLLM):

    def completion(self, model: str, messages: list, **kwargs) -> ModelResponse:
        """Synchronous completion — enqueue task, poll for result."""
        model_name = self._extract_model(model)
        max_tokens = kwargs.get("optional_params", {}).get("max_tokens", 300)

        # Enqueue
        task_id = self._enqueue(model_name, messages, max_tokens)

        # Wait for result
        result = self._wait_for_result(task_id)

        return ModelResponse(
            id=f"chatcmpl-community-{task_id}",
            choices=[
                Choices(
                    message=Message(role="assistant", content=result["content"]),
                    index=0,
                    finish_reason="stop",
                )
            ],
            model=model,
            usage={
                "prompt_tokens": 0,
                "completion_tokens": result.get("tokens_used", 0),
                "total_tokens": result.get("tokens_used", 0),
            },
        )

    async def acompletion(self, model: str, messages: list, **kwargs) -> ModelResponse:
        """Async completion — enqueue task, poll for result."""
        model_name = self._extract_model(model)
        max_tokens = kwargs.get("optional_params", {}).get("max_tokens", 300)

        async with httpx.AsyncClient() as client:
            # Enqueue
            task_id = await self._async_enqueue(client, model_name, messages, max_tokens)

            # Wait for result
            result = await self._async_wait_for_result(client, task_id)

        return ModelResponse(
            id=f"chatcmpl-community-{task_id}",
            choices=[
                Choices(
                    message=Message(role="assistant", content=result["content"]),
                    index=0,
                    finish_reason="stop",
                )
            ],
            model=model,
            usage={
                "prompt_tokens": 0,
                "completion_tokens": result.get("tokens_used", 0),
                "total_tokens": result.get("tokens_used", 0),
            },
        )

    def _extract_model(self, model: str) -> str:
        """Extract model name from 'community/gemma4' format."""
        if "/" in model:
            return model.split("/", 1)[1]
        return model

    def _format_messages(self, messages: list) -> list:
        """Convert LiteLLM message objects to dicts."""
        formatted = []
        for msg in messages:
            if isinstance(msg, dict):
                formatted.append({"role": msg["role"], "content": msg.get("content", "")})
            else:
                formatted.append({"role": msg.role, "content": getattr(msg, "content", "")})
        return formatted

    def _enqueue(self, model: str, messages: list, max_tokens: int) -> str:
        resp = httpx.post(
            f"{AGENT_API_URL}/api/tasks",
            json={
                "model": model,
                "messages": self._format_messages(messages),
                "max_tokens": max_tokens,
            },
            timeout=10,
        )
        resp.raise_for_status()
        return resp.json()["task_id"]

    async def _async_enqueue(self, client: httpx.AsyncClient, model: str, messages: list, max_tokens: int) -> str:
        resp = await client.post(
            f"{AGENT_API_URL}/api/tasks",
            json={
                "model": model,
                "messages": self._format_messages(messages),
                "max_tokens": max_tokens,
            },
            timeout=10,
        )
        resp.raise_for_status()
        return resp.json()["task_id"]

    def _wait_for_result(self, task_id: str) -> dict:
        deadline = time.time() + TASK_TIMEOUT
        while time.time() < deadline:
            resp = httpx.get(f"{AGENT_API_URL}/api/tasks/{task_id}", timeout=10)
            resp.raise_for_status()
            task = resp.json()
            if task["status"] == "done":
                return json.loads(task["result"]) if isinstance(task["result"], str) else task["result"]
            if task["status"] == "failed":
                raise litellm.exceptions.ServiceUnavailableError(
                    message="Community inference failed",
                    model="community",
                    llm_provider="community",
                )
            time.sleep(TASK_POLL_INTERVAL)
        raise litellm.exceptions.Timeout(
            message=f"Community task {task_id} timed out after {TASK_TIMEOUT}s",
            model="community",
            llm_provider="community",
        )

    async def _async_wait_for_result(self, client: httpx.AsyncClient, task_id: str) -> dict:
        import asyncio
        deadline = time.time() + TASK_TIMEOUT
        while time.time() < deadline:
            resp = await client.get(f"{AGENT_API_URL}/api/tasks/{task_id}", timeout=10)
            resp.raise_for_status()
            task = resp.json()
            if task["status"] == "done":
                return json.loads(task["result"]) if isinstance(task["result"], str) else task["result"]
            if task["status"] == "failed":
                raise litellm.exceptions.ServiceUnavailableError(
                    message="Community inference failed",
                    model="community",
                    llm_provider="community",
                )
            await asyncio.sleep(TASK_POLL_INTERVAL)
        raise litellm.exceptions.Timeout(
            message=f"Community task {task_id} timed out after {TASK_TIMEOUT}s",
            model="community",
            llm_provider="community",
        )


# Instance that LiteLLM's custom_provider_map references
handler = CommunityProvider()
