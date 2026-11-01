import type { MetadataRoute } from "next";
import { BRAND } from "@/components/brand/mark";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "Wasl",
    short_name: "Wasl",
    description: "Every positive screening test, followed to a documented end. Synthetic data demo.",
    start_url: "/",
    display: "standalone",
    background_color: BRAND.paper,
    theme_color: BRAND.teal,
    icons: [
      { src: "/icon-192.png", sizes: "192x192", type: "image/png", purpose: "any" },
      { src: "/icon-512.png", sizes: "512x512", type: "image/png", purpose: "any" },
      { src: "/icon-maskable-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
    ],
  };
}
