/** The public production domain. Crawlers can fetch it; per-deployment Vercel hostnames sit behind deployment protection. */
export const PRODUCTION_URL = "https://wasl-topaz-mu.vercel.app";

type Env = Partial<Record<"NEXT_PUBLIC_SITE_URL" | "VERCEL_PROJECT_PRODUCTION_URL" | "VERCEL_URL" | "NODE_ENV" | "PORT", string | undefined>>;

const absolute = (v: string) => (/^https?:\/\//.test(v) ? v : `https://${v}`).replace(/\/+$/, "");

/**
 * The origin that social-preview URLs (og:image, twitter:image) are built on:
 *  1. an explicit NEXT_PUBLIC_SITE_URL;
 *  2. else VERCEL_PROJECT_PRODUCTION_URL, the project's production domain;
 *  3. else the fixed public domain, in any production build;
 *  4. localhost only in development.
 * VERCEL_URL, the per-deployment hostname, is deliberately never used, so preview deployments also point at production.
 */
export function siteUrl(env: Env = process.env): string {
  const explicit = env.NEXT_PUBLIC_SITE_URL?.trim();
  if (explicit) return absolute(explicit);
  const project = env.VERCEL_PROJECT_PRODUCTION_URL?.trim();
  if (project) return absolute(project);
  if (env.NODE_ENV === "development") return `http://localhost:${env.PORT ?? 3000}`;
  return PRODUCTION_URL;
}
