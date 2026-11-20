/**
 * Who a signed-in review session acts as, in words for the app shell. Client-safe: no token ever enters this type.
 * The API binds each bearer token to an actor and role; the web server can name that actor only when it holds the same
 * token map (WASL_API_TOKENS, "token:role:actor,…", as scripts/demo.sh exports it). Otherwise it names the credential.
 */
export type SessionPersona = { actor: string; role: string; method: "demo" | "persona" | "oidc" };

type Env = Record<string, string | undefined>;

const ROLE_WORDS: Record<string, string> = {
  reviewer: "Reviewer",
  senior_reviewer: "Senior reviewer",
  clinician: "Clinician",
  senior_clinician: "Senior clinician",
  auditor: "Auditor",
  coordinator: "Coordinator",
};

/** Web-server credentials a review session can carry, in words. */
const CREDENTIAL_WORDS: Record<string, string> = {
  reviewer: "Reviewer",
  reviewer_2: "Second reviewer",
  reviewer_3: "Senior reviewer",
  clinician: "Clinician",
  clinician_2: "Second clinician",
  clinician_3: "Senior clinician",
  web_reviewer: "Shared reader",
};

export const roleWords = (role: string) => ROLE_WORDS[role] ?? role.replaceAll("_", " ");
export const credentialWords = (credential: string) => CREDENTIAL_WORDS[credential] ?? credential.replaceAll("_", " ");

/** `token:role:actor` entries; the token is everything before the last two colons. */
function tokenMap(raw: string | undefined): Map<string, { role: string; actor: string }> {
  const out = new Map<string, { role: string; actor: string }>();
  for (const entry of (raw ?? "").split(",")) {
    const parts = entry.trim().split(":");
    if (parts.length < 3) continue;
    const actor = parts.pop()!.trim();
    const role = parts.pop()!.trim();
    const token = parts.join(":").trim();
    if (token && role && actor) out.set(token, { role, actor });
  }
  return out;
}

/** A demo-token session: the API's actor when this server knows the token map, else the credential it matches. */
export function personaFromToken(token: string, env: Env = process.env): SessionPersona {
  const bound = tokenMap(env.WASL_API_TOKENS).get(token);
  if (bound) return { actor: bound.actor, role: roleWords(bound.role), method: "demo" };
  for (const credential of Object.keys(CREDENTIAL_WORDS)) {
    if (env[`WASL_API_${credential.toUpperCase()}_TOKEN`]?.trim() === token) {
      return { actor: `${credentialWords(credential)} credential`, role: "Demo token · actor bound by the API", method: "demo" };
    }
  }
  return { actor: "Pasted reviewer token", role: "Demo token · actor bound by the API", method: "demo" };
}

/** An open-demo persona session: the persona's name, labelled as a demo persona (the API still binds the actor). */
export function personaFromDemo(name: string, role: string): SessionPersona {
  return { actor: name, role: `${role} · demo persona`, method: "persona" };
}

/** An OIDC session: the identity provider's subject, acting with its mapped credential. */
export function personaFromOidc(subject: string, credential: string): SessionPersona {
  return { actor: subject, role: `${credentialWords(credential)} · OIDC`, method: "oidc" };
}
