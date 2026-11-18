import { NextResponse } from "next/server";
import { calculatePKCECodeChallenge, randomPKCECodeVerifier } from "openid-client";
import { oidcConfig, reviewAuthMode } from "@/lib/auth/oidc-config";
import {
  beginAuthorization,
  discoverProvider,
  hasIdentityParameters,
  logOidcFailure,
  oidcFailureRedirect,
  safeReturnTo,
  type OidcFailureCategory,
} from "@/lib/auth/oidc";
import { defaultSessionStore, LOGIN_LIFETIME_MS, nonceForVerifier, randomOpaqueId } from "@/lib/auth/oidc-store";
import { OIDC_CORRELATION_COOKIE, OIDC_CORRELATION_COOKIE_PATH } from "@/components/review/auth";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

/*
 * Next's client router adds `_rsc` (a short cache-busting hash) when it fetches a URL as a React Server Component, and
 * answers a route handler's redirect by retrying with it. It carries no identity or destination, so a single
 * well-formed `_rsc` is dropped before validation; every other parameter is still refused.
 */
const NEXT_RSC_PARAMETER = "_rsc";
const NEXT_RSC_VALUE = /^[A-Za-z0-9_-]{0,64}$/;

function withoutNextRsc(parameters: URLSearchParams): URLSearchParams | undefined {
  const values = parameters.getAll(NEXT_RSC_PARAMETER);
  if (values.length === 0) return parameters;
  if (values.length > 1 || !NEXT_RSC_VALUE.test(values[0])) return undefined;
  const rest = new URLSearchParams(parameters);
  rest.delete(NEXT_RSC_PARAMETER);
  return rest;
}

export async function GET(request: Request): Promise<Response> {
  const url = new URL(request.url);
  const returnTo = safeReturnTo(url.searchParams.get("returnTo"), url.origin);
  const refuse = (category: OidcFailureCategory, error?: unknown, target: string | undefined = returnTo) => {
    logOidcFailure("start", category, error);
    return oidcFailureRedirect(request, target);
  };
  let stage: OidcFailureCategory = "config";
  try {
    if (reviewAuthMode() !== "oidc") return refuse("disabled");
    const parameters = withoutNextRsc(url.searchParams);
    if (!parameters) return refuse("bad_request");
    if (hasIdentityParameters(parameters)) return refuse("bad_request");
    if (parameters.getAll("returnTo").length > 1) return refuse("bad_request", undefined, undefined);
    for (const name of parameters.keys()) if (name !== "returnTo") return refuse("bad_request");
    if (parameters.has("returnTo") && returnTo === "/app/review" && parameters.get("returnTo") !== returnTo) {
      return refuse("bad_request", undefined, undefined);
    }
    const config = oidcConfig();
    stage = "discovery";
    const provider = await discoverProvider(config);
    stage = "session_store";
    const state = randomOpaqueId();
    const correlation = randomOpaqueId();
    const pkceVerifier = randomPKCECodeVerifier();
    const nonce = nonceForVerifier(pkceVerifier);
    const codeChallenge = await calculatePKCECodeChallenge(pkceVerifier);
    await (await defaultSessionStore(config.sessionStore)).createLogin({ state, correlation, nonce, pkceVerifier, returnTo });
    stage = "authorize";
    const destination = beginAuthorization(provider, config, { state, nonce, codeChallenge });
    const response = NextResponse.redirect(destination, 303);
    response.headers.set("cache-control", "no-store");
    response.cookies.set(OIDC_CORRELATION_COOKIE, correlation, {
      httpOnly: true,
      secure: true,
      sameSite: "lax",
      path: OIDC_CORRELATION_COOKIE_PATH,
      maxAge: LOGIN_LIFETIME_MS / 1000,
    });
    return response;
  } catch (error) {
    return refuse(stage, error);
  }
}
