import { NextResponse } from "next/server";
import { calculatePKCECodeChallenge, randomPKCECodeVerifier } from "openid-client";
import { oidcConfig, reviewAuthMode } from "@/lib/auth/oidc-config";
import { beginAuthorization, discoverProvider, hasIdentityParameters, oidcFailureRedirect, safeReturnTo } from "@/lib/auth/oidc";
import { defaultSessionStore, LOGIN_LIFETIME_MS, nonceForVerifier, randomOpaqueId } from "@/lib/auth/oidc-store";
import { OIDC_CORRELATION_COOKIE, OIDC_CORRELATION_COOKIE_PATH } from "@/components/review/auth";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request: Request): Promise<Response> {
  const url = new URL(request.url);
  const returnTo = safeReturnTo(url.searchParams.get("returnTo"), url.origin);
  try {
    if (reviewAuthMode() !== "oidc") return oidcFailureRedirect(request, returnTo);
    if (hasIdentityParameters(url.searchParams)) return oidcFailureRedirect(request, returnTo);
    if (url.searchParams.getAll("returnTo").length > 1) return oidcFailureRedirect(request);
    for (const name of url.searchParams.keys()) if (name !== "returnTo") return oidcFailureRedirect(request, returnTo);
    if (url.searchParams.has("returnTo") && returnTo === "/app/review" && url.searchParams.get("returnTo") !== returnTo) {
      return oidcFailureRedirect(request);
    }
    const config = oidcConfig();
    const provider = await discoverProvider(config);
    const state = randomOpaqueId();
    const correlation = randomOpaqueId();
    const pkceVerifier = randomPKCECodeVerifier();
    const nonce = nonceForVerifier(pkceVerifier);
    const codeChallenge = await calculatePKCECodeChallenge(pkceVerifier);
    defaultSessionStore(config.sessionDb).createLogin({ state, correlation, nonce, pkceVerifier, returnTo });
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
  } catch {
    return oidcFailureRedirect(request, returnTo);
  }
}
