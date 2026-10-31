import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Cache Components is off: pages read the API (or fixtures) per request, and the shell reads the pathname.
  cacheComponents: false,
  // Lets parallel dev servers in one checkout keep separate build folders.
  distDir: process.env.NEXT_DIST_DIR ?? ".next",
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
