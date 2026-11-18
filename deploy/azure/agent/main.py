"""Foundry hosted-agent entrypoint for the Wasl audit explainer."""

from __future__ import annotations

import asyncio
import functools
import json
import os
import urllib.request

from azure.ai.agentserver.responses import (
    CreateResponse,
    ResponseContext,
    ResponsesAgentServerHost,
    TextResponse,
)

import wasl_agent

MODEL_TIMEOUT_S = 180


@functools.cache
def secret(env_name: str, vault_secret: str) -> str:
    vault_url = os.environ.get("WASL_KEYVAULT_URL")
    if not vault_url:
        return os.environ[env_name]
    from azure.identity import DefaultAzureCredential
    from azure.keyvault.secrets import SecretClient

    return SecretClient(vault_url, DefaultAzureCredential()).get_secret(vault_secret).value


def auditor_token() -> str:
    return secret("WASL_API_TOKEN", os.environ.get("WASL_AUDITOR_SECRET_NAME", "wasl-auditor-token"))


def chat_model(base_url: str, model_name: str) -> wasl_agent.ModelFn:
    """One call to an OpenAI-compatible /v1/chat/completions, such as the repository's llama-server reader."""

    def call(system: str, user: str) -> str:
        body = {
            "model": model_name,
            "messages": [{"role": "system", "content": system}, {"role": "user", "content": user}],
            "max_tokens": wasl_agent.MAX_OUTPUT_TOKENS,
        }
        key = secret("WASL_LLM_KEY", os.environ.get("WASL_LLM_KEY_SECRET_NAME", "gemma-api-key"))
        request = urllib.request.Request(
            base_url.rstrip("/") + "/v1/chat/completions",
            data=json.dumps(body).encode(),
            headers={"Content-Type": "application/json", "Authorization": f"Bearer {key}"},
        )
        with urllib.request.urlopen(request, timeout=MODEL_TIMEOUT_S) as response:
            return json.loads(response.read())["choices"][0]["message"]["content"] or ""

    return call


def build_app(model: wasl_agent.ModelFn, model_name: str) -> ResponsesAgentServerHost:
    api_url = os.environ["WASL_API_URL"]
    app = ResponsesAgentServerHost()

    @app.response_handler
    async def handler(request: CreateResponse, context: ResponseContext, _cancel: asyncio.Event):
        text = await context.get_input_text() or ""
        reply = await asyncio.to_thread(
            wasl_agent.answer, text, api_url=api_url, token=auditor_token, model=model, model_name=model_name
        )
        return TextResponse(context, request, text=reply)

    return app


if __name__ == "__main__":
    model_name = os.environ.get("WASL_LLM_MODEL", "gemma-3-4b-it-Q4_K_M")
    build_app(chat_model(os.environ["WASL_LLM_URL"], model_name), model_name).run()
