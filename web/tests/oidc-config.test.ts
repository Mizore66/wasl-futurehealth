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
});
