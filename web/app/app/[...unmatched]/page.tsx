import { notFound } from "next/navigation";

/** Unknown /app/* URLs: calling notFound() here renders app/app/not-found.tsx inside the app shell, with a 404 status. */
export default function Unmatched() {
  notFound();
}
