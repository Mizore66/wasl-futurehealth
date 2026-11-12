import { calculatePKCECodeChallenge, randomPKCECodeVerifier } from "openid-client";
import { oidcConfig, reviewAuthMode } from "@/lib/auth/oidc-config";
import { beginAuthorization, discoverProvider, hasIdentityParameters, oidcFailureRedirect, safeReturnTo } from "@/lib/auth/oidc";
import { defaultSessionStore, nonceForVerifier, randomOpaqueId } from "@/lib/auth/oidc-store";

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
    const pkceVerifier = randomPKCECodeVerifier();
    const nonce = nonceForVerifier(pkceVerifier);
    const codeChallenge = await calculatePKCECodeChallenge(pkceVerifier);
    defaultSessionStore(config.sessionDb).createLogin({ state, nonce, pkceVerifier, returnTo });
    const destination = beginAuthorization(provider, config, { state, nonce, codeChallenge });
    return new Response(null, { status: 303, headers: { location: destination.href, "cache-control": "no-store" } });
  } catch {
    return oidcFailureRedirect(request, returnTo);
  }
}
