import type { HonestyKind } from "@/lib/honesty";

export type ResultEntry = {
  id: string;
  value: number;
  display: string;
  label: HonestyKind;
  /** Repo-relative file the number was read from. */
  source: string;
  context: string;
  denominator?: number;
  ci95?: [number, number];
  upper95?: string;
};

export type Results = {
  frozen_revision: string;
  sources: string[];
  entries: Record<string, ResultEntry>;
  evidence: { what: string; result: string; source: string; label: HonestyKind }[];
  conflicts: {
    id: string;
    question: string;
    values: { result: string; value: number; source: string }[];
    note: string;
  }[];
};
