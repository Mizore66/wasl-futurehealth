import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Cache Components is off: pages read the API (or fixtures) per request, and the shell reads the pathname.
  cacheComponents: false,
  experimental: {
    // Vercel restores .next/cache between builds, and the restored Turbopack cache served the Tailwind CSS compiled
    // from the previous globals.css (production lost the staff palette while the HTML was new). Build cold instead.
    turbopackFileSystemCacheForBuild: false,
  },
  // Lets parallel dev servers in one checkout keep separate build folders.
  distDir: process.env.NEXT_DIST_DIR ?? ".next",
  // Fixtures mode reads data/api with fs at request time, which the tracer cannot see; the landing evidence reads it too.
  outputFileTracingIncludes: { "/app/**": ["./data/api/**"], "/": ["./data/api/**"] },
  // The expert page now lives under the landing story.
  async redirects() {
    return [{ source: "/experts", destination: "/#evidence", permanent: false }];
  },
  async headers() {
    const patientHeaders = [
      { key: "Cache-Control", value: "no-store" },
      { key: "Referrer-Policy", value: "no-referrer" },
    ];
    return [
      { source: "/patient/:path*", headers: patientHeaders },
      // The internal translation-review preview (#133) reads the same synthetic patient link and must not be indexed.
      { source: "/translation-review/:path*", headers: [...patientHeaders, { key: "X-Robots-Tag", value: "noindex, nofollow" }] },
    ];
  },
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
};

export default nextConfig;
