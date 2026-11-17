import { describe, expect, it } from "vitest";
import { credentialVariable, mappedCredentialToken, oidcConfig, parseSubjectMap, reviewAuthMode } from "@/lib/auth/oidc-config";

const env = {
  WASL_REVIEW_AUTH_MODE: "oidc",
  WASL_OIDC_ISSUER: "https://id.example.test",
  WASL_OIDC_CLIENT_ID: "review-client",
  WASL_OIDC_CLIENT_SECRET: "server-secret",
  WASL_OIDC_REDIRECT_URI: "https://wasl.example.test/app/auth/oidc/callback",
  WASL_OIDC_SUBJECT_MAP_JSON: '{"synthetic-r1":"reviewer","synthetic-r2":"reviewer_2"}',
  WASL_OIDC_SESSION_DB: "/private/wasl/sessions.sqlite",
  WASL_API_REVIEWER_TOKEN: "reviewer-server-token-1234",
  WASL_API_REVIEWER_2_TOKEN: "reviewer-two-server-token-1234",
} satisfies Record<string, string>;

describe("OIDC configuration", () => {
  it("defaults to demo and accepts only an explicit mode", () => {
    expect(reviewAuthMode({})).toBe("demo");
    expect(reviewAuthMode({ WASL_REVIEW_AUTH_MODE: "oidc" })).toBe("oidc");
    expect(() => reviewAuthMode({ WASL_REVIEW_AUTH_MODE: "OIDC" })).toThrow();
  });

  it("loads the complete server-only provider configuration", () => {
    const config = oidcConfig(env);
    expect(config.clientSecret).toBe("server-secret");
    expect(config.subjectMap.get("synthetic-r1")).toBe("reviewer");
    expect(mappedCredentialToken(config, "synthetic-r1", "reviewer", env)).toBe("reviewer-server-token-1234");
    expect(mappedCredentialToken(config, "synthetic-r1", "reviewer_2", env)).toBeUndefined();
    expect(mappedCredentialToken(config, "unknown", undefined, env)).toBeUndefined();
    expect(credentialVariable("clinician_3")).toBe("WASL_API_CLINICIAN_3_TOKEN");
  });

  it.each(["WASL_OIDC_ISSUER", "WASL_OIDC_CLIENT_ID", "WASL_OIDC_CLIENT_SECRET", "WASL_OIDC_REDIRECT_URI", "WASL_OIDC_SUBJECT_MAP_JSON", "WASL_OIDC_SESSION_DB"])("requires %s", (name) => {
    expect(() => oidcConfig({ ...env, [name]: "" })).toThrow(name);
  });

  it.each(["reviewer_4", "web_reviewer", "auditor", "coordinator", "admin"])("refuses shared or unknown credential %s", (credential) => {
    expect(() => parseSubjectMap(JSON.stringify({ subject: credential }))).toThrow("unknown credential");
  });

  it("rejects duplicate subjects, duplicate assignments, unknown fields and invalid subjects", () => {
    expect(() => parseSubjectMap('{"same":"reviewer","same":"reviewer_2"}')).toThrow("duplicate subject");
    expect(() => parseSubjectMap('{"\\u0073ame":"reviewer","same":"reviewer_2"}')).toThrow("duplicate subject");
    expect(() => parseSubjectMap('{"a":"reviewer","b":"reviewer"}')).toThrow("only one subject");
    expect(() => parseSubjectMap('{"a":{"credential":"reviewer"}}')).toThrow();
    expect(() => parseSubjectMap('{"":"reviewer"}')).toThrow();
    expect(() => parseSubjectMap(`{"${"x".repeat(256)}":"reviewer"}`)).toThrow();
    expect(() => parseSubjectMap('{" padded ":"reviewer"}')).toThrow();
  });

  it("requires HTTPS for both issuer and callback", () => {
    expect(() => oidcConfig({ ...env, WASL_OIDC_ISSUER: "http://id.example.test" })).toThrow("HTTPS");
    expect(() => oidcConfig({ ...env, WASL_OIDC_REDIRECT_URI: "http://wasl.example.test/app/auth/oidc/callback" })).toThrow("HTTPS");
  });

  it("allows HTTP only for explicitly synthetic loopback testing", () => {
    const loopback = {
      ...env,
      WASL_OIDC_ISSUER: "http://127.0.0.1:4444",
      WASL_OIDC_REDIRECT_URI: "http://localhost:3000/app/auth/oidc/callback",
      WASL_OIDC_INSECURE_LOOPBACK: "1",
      WASL_SYNTHETIC_DATA_ONLY: "1",
    };
    expect(oidcConfig(loopback).insecureLoopback).toBe(true);
    expect(() => oidcConfig({ ...loopback, WASL_SYNTHETIC_DATA_ONLY: "0" })).toThrow("loopback synthetic");
    expect(() => oidcConfig({ ...loopback, WASL_OIDC_ISSUER: "http://192.0.2.1" })).toThrow("loopback synthetic");
  });

  it("keeps SQLite as the default single-instance session store", () => {
    expect(oidcConfig(env).sessionStore).toEqual({ kind: "sqlite", path: "/private/wasl/sessions.sqlite" });
    expect(() => oidcConfig({ ...env, WASL_OIDC_SESSION_STORE: "redis" })).toThrow("sqlite or nats");
  });

  describe("shared NATS session store", () => {
    const nats = {
      ...env,
      WASL_OIDC_SESSION_DB: "",
      WASL_OIDC_SESSION_STORE: "nats",
      WASL_OIDC_NATS_URL: "tls://nats-0:4222, tls://nats-1:4222,tls://nats-2:4222",
      WASL_OIDC_NATS_USER: "wasl-web",
      WASL_OIDC_NATS_PASSWORD: "server-only-nats-password",
      WASL_OIDC_NATS_TLS_CA: "/run/secrets/ca.crt",
      WASL_OIDC_NATS_TLS_CERT: "/run/secrets/web.crt",
      WASL_OIDC_NATS_TLS_KEY: "/run/secrets/web.key",
    };

    it("loads an authenticated TLS cluster with three replicas by default and needs no SQLite path", () => {
      expect(oidcConfig(nats).sessionStore).toEqual({
        kind: "nats",
        servers: ["nats-0:4222", "nats-1:4222", "nats-2:4222"],
        user: "wasl-web",
        password: "server-only-nats-password",
        tls: { caFile: "/run/secrets/ca.crt", certFile: "/run/secrets/web.crt", keyFile: "/run/secrets/web.key" },
        replicas: 3,
      });
      expect(oidcConfig({ ...nats, WASL_OIDC_NATS_REPLICAS: "1" }).sessionStore).toMatchObject({ replicas: 1 });
    });

    it("refuses plaintext, unauthenticated or malformed connections outside loopback synthetic testing", () => {
      expect(() => oidcConfig({ ...nats, WASL_OIDC_NATS_TLS_CA: "", WASL_OIDC_NATS_TLS_CERT: "", WASL_OIDC_NATS_TLS_KEY: "" })).toThrow("WASL_OIDC_NATS_TLS_CA");
      expect(() => oidcConfig({ ...nats, WASL_OIDC_NATS_USER: "", WASL_OIDC_NATS_PASSWORD: "" })).toThrow("WASL_OIDC_NATS_USER");
      expect(() => oidcConfig({ ...nats, WASL_OIDC_NATS_PASSWORD: "" })).toThrow("set together");
      expect(() => oidcConfig({ ...nats, WASL_OIDC_NATS_TLS_KEY: "" })).toThrow("set together");
      expect(() => oidcConfig({ ...nats, WASL_OIDC_NATS_URL: "" })).toThrow("WASL_OIDC_NATS_URL");
      expect(() => oidcConfig({ ...nats, WASL_OIDC_NATS_URL: "nats://user:secret@nats-0:4222" })).toThrow("without credentials");
      expect(() => oidcConfig({ ...nats, WASL_OIDC_NATS_URL: "https://nats-0:4222" })).toThrow("nats:// or tls://");
      expect(() => oidcConfig({ ...nats, WASL_OIDC_NATS_REPLICAS: "2" })).toThrow("1, 3 or 5");
    });

    it("allows plaintext unauthenticated NATS only for loopback synthetic testing", () => {
      const loopback = {
        ...env,
        WASL_OIDC_SESSION_STORE: "nats",
        WASL_OIDC_NATS_URL: "nats://127.0.0.1:4222",
        WASL_OIDC_ISSUER: "http://127.0.0.1:4444",
        WASL_OIDC_REDIRECT_URI: "http://localhost:3000/app/auth/oidc/callback",
        WASL_OIDC_INSECURE_LOOPBACK: "1",
        WASL_SYNTHETIC_DATA_ONLY: "1",
      };
      expect(oidcConfig(loopback).sessionStore).toEqual({ kind: "nats", servers: ["127.0.0.1:4222"], replicas: 3 });
      expect(() => oidcConfig({ ...loopback, WASL_OIDC_NATS_URL: "nats://nats-0:4222" })).toThrow("WASL_OIDC_NATS_TLS_CA");
    });
  });
});
