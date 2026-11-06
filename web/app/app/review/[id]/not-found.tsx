import { CaseNotFound } from "@/components/case/demo-cases";

export const metadata = { title: "Case not found" };

export default function NotFound() {
  return <CaseNotFound base="/app/review" />;
}
