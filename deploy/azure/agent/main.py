"""Foundry hosted-agent entrypoint for the Wasl audit explainer."""

from __future__ import annotations

import asyncio
import functools
import os

from azure.ai.agentserver.responses import (
    CreateResponse,
    ResponseContext,
    ResponsesAgentServerHost,
    TextResponse,
)

import wasl_agent


@functools.cache
def auditor_token() -> str:
    vault_url = os.environ.get("WASL_KEYVAULT_URL")
    if not vault_url:
        return os.environ["WASL_API_TOKEN"]
    from azure.identity import DefaultAzureCredential
    from azure.keyvault.secrets import SecretClient

    secret_name = os.environ.get("WASL_AUDITOR_SECRET_NAME", "wasl-auditor-token")
    return SecretClient(vault_url, DefaultAzureCredential()).get_secret(secret_name).value


def foundry_model(endpoint: str, deployment: str) -> wasl_agent.ModelFn:
    client = None

    def call(system: str, user: str) -> str:
        nonlocal client
        if client is None:
            from azure.ai.projects import AIProjectClient
            from azure.identity import DefaultAzureCredential

            client = AIProjectClient(endpoint=endpoint, credential=DefaultAzureCredential()).get_openai_client()
        completion = client.chat.completions.create(
            model=deployment,
            messages=[{"role": "system", "content": system}, {"role": "user", "content": user}],
            max_tokens=wasl_agent.MAX_OUTPUT_TOKENS,
        )
        return completion.choices[0].message.content or ""

    return call


def build_app(model: wasl_agent.ModelFn, deployment: str) -> ResponsesAgentServerHost:
    api_url = os.environ["WASL_API_URL"]
    app = ResponsesAgentServerHost()

    @app.response_handler
    async def handler(request: CreateResponse, context: ResponseContext, _cancel: asyncio.Event):
        text = await context.get_input_text() or ""
        reply = await asyncio.to_thread(
            wasl_agent.answer, text, api_url=api_url, token=auditor_token, model=model, deployment=deployment
        )
        return TextResponse(context, request, text=reply)

    return app


if __name__ == "__main__":
    deployment = os.environ.get("AZURE_AI_MODEL_DEPLOYMENT_NAME", "gpt-4.1-mini")
    build_app(foundry_model(os.environ.get("FOUNDRY_PROJECT_ENDPOINT", ""), deployment), deployment).run()
