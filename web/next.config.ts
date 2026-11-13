import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Cache Components is off: pages read the API (or fixtures) per request, and the shell reads the pathname.
  cacheComponents: false,
  // Lets parallel dev servers in one checkout keep separate build folders.
  distDir: process.env.NEXT_DIST_DIR ?? ".next",
  // Fixtures mode reads data/api with fs at request time, which the tracer cannot see.
  outputFileTracingIncludes: { "/app/**": ["./data/api/**"] },
  async headers() {
    return [
      {
        source: "/patient/:path*",
        headers: [
          { key: "Cache-Control", value: "no-store" },
          { key: "Referrer-Policy", value: "no-referrer" },
        ],
      },
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
