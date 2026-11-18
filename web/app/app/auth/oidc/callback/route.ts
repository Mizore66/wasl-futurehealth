import { NextResponse } from "next/server";
import { mappedCredentialToken, oidcConfig, reviewAuthMode } from "@/lib/auth/oidc-config";
import { discoverProvider, exchangeAuthorizationCode, hasIdentityParameters, logOidcFailure, oidcFailureRedirect, type OidcFailureCategory } from "@/lib/auth/oidc";
import { defaultSessionStore, nonceForVerifier, sha256 } from "@/lib/auth/oidc-store";
import {
  OIDC_CORRELATION_COOKIE,
  OIDC_CORRELATION_COOKIE_PATH,
  OIDC_REVIEW_COOKIE,
  OIDC_SESSION_SAME_SITE,
  REVIEW_COOKIE_PATH,
} from "@/components/review/auth";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const CALLBACK_PARAMETERS = new Set(["code", "state", "iss", "error", "error_description", "error_uri"]);

function correlationCookie(request: Request): string | undefined {
  const values = (request.headers.get("cookie") ?? "").split(";").flatMap((part) => {
    const separator = part.indexOf("=");
    if (separator < 0 || part.slice(0, separator).trim() !== OIDC_CORRELATION_COOKIE) return [];
    return [part.slice(separator + 1).trim()];
  });
  return values.length === 1 && /^[A-Za-z0-9_-]{43}$/.test(values[0]) ? values[0] : undefined;
}

export async function GET(request: Request): Promise<Response> {
  const url = new URL(request.url);
  let returnTo = "/app/review";
  const refuse = (category: OidcFailureCategory, error?: unknown, target?: string) => {
    logOidcFailure("callback", category, error);
    return oidcFailureRedirect(request, target);
  };
  let stage: OidcFailureCategory = "config";
  try {
    if (reviewAuthMode() !== "oidc") return refuse("disabled");
    if (hasIdentityParameters(url.searchParams)) return refuse("bad_request");
    for (const name of url.searchParams.keys()) {
      if (!CALLBACK_PARAMETERS.has(name) || url.searchParams.getAll(name).length !== 1) return refuse("bad_request");
    }
    if (url.searchParams.has("error")) return refuse("provider_error");
    const config = oidcConfig();
    const code = url.searchParams.get("code");
    const state = url.searchParams.get("state");
    if (!code || !state) return refuse("bad_request");
    if (url.searchParams.has("iss") && url.searchParams.get("iss") !== config.issuer) return refuse("issuer_mismatch");

    /* Consume only the transaction bound to this browser before discovery or token exchange. */
    const correlation = correlationCookie(request);
    if (!correlation) return refuse("no_correlation");
    stage = "session_store";
    const store = await defaultSessionStore(config.sessionStore);
    const transaction = await store.consumeLogin(state, correlation);
    if (!transaction) return refuse("unknown_state");
    returnTo = transaction.returnTo;
    const nonce = nonceForVerifier(transaction.pkceVerifier);
    if (sha256(nonce) !== transaction.nonceHash) return refuse("nonce_mismatch", undefined, returnTo);

    stage = "discovery";
    const provider = await discoverProvider(config);
    const callback = new URL(config.redirectUri);
    callback.search = url.search;
    stage = "token_exchange";
    const identity = await exchangeAuthorizationCode(provider, config, callback, transaction, state, nonce);
    if (identity.issuer !== config.issuer) return refuse("issuer_mismatch", undefined, returnTo);
    if (!mappedCredentialToken(config, identity.subject)) return refuse("unmapped_subject", undefined, returnTo);
    const credential = config.subjectMap.get(identity.subject)!;
    stage = "session_store";
    const { sessionId, session } = await store.createSession({ issuer: identity.issuer, subject: identity.subject, credential });

    const destination = new URL(returnTo, config.redirectUri);
    const response = NextResponse.redirect(destination, 303);
    response.headers.set("cache-control", "no-store");
    /* Lax, not Strict: see OIDC_SESSION_SAME_SITE for why this keeps CSRF protection and fixes the first page. */
    response.cookies.set(OIDC_REVIEW_COOKIE, sessionId, {
      httpOnly: true,
      secure: true,
      sameSite: OIDC_SESSION_SAME_SITE,
      path: REVIEW_COOKIE_PATH,
      maxAge: Math.max(1, Math.floor((session.expiresAt - Date.now()) / 1000)),
    });
    response.cookies.set(OIDC_CORRELATION_COOKIE, "", {
      httpOnly: true,
      secure: true,
      sameSite: "lax",
      path: OIDC_CORRELATION_COOKIE_PATH,
      maxAge: 0,
    });
    return response;
  } catch (error) {
    return refuse(stage, error, returnTo);
  }
}
