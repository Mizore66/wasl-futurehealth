"use client";

import { useEffect, useState } from "react";
import { readReviewSource } from "@/app/app/review/[id]/actions";
import type { ReviewSource, SourceStatus } from "@/lib/api/schema";
import type { ReviewSourceResult } from "@/lib/api/source";

type Loaded = { status: "loading" } | { status: "available"; source: ReviewSource } | { status: "unavailable"; reason: string };

const UNAVAILABLE: Record<SourceStatus, string> = {
  missing: "The report was not found.",
  ambiguous: "More than one report matches this item and version, so none is shown.",
  unreadable: "The report could not be opened.",
};

/**
 * The complete report revision an evidence item judges. Signing stays withheld until it is on screen;
 * opening it records nothing and answers nothing.
 */
export function SourceReport({
  caseId,
  item,
  onAvailable,
  preloaded,
}: {
  caseId: string;
  item: string;
  onAvailable: (available: boolean) => void;
  /** Fixtures mode: the page already read the report on the server, so no server action (and no POST) is made. */
  preloaded?: ReviewSourceResult;
}) {
  const [loaded, setLoaded] = useState<Loaded>({ status: "loading" });
  useEffect(() => {
    let current = true;
    onAvailable(false);
    (preloaded ? Promise.resolve(preloaded) : readReviewSource(caseId, item))
      .then((res): Loaded => {
        if (res.ok) return { status: "available", source: res.source };
        if (res.sourceStatus) return { status: "unavailable", reason: UNAVAILABLE[res.sourceStatus] };
        return { status: "unavailable", reason: res.status === 401 ? "Sign in again to open the report." : "You do not have access to this report." };
      })
      .catch((): Loaded => ({ status: "unavailable", reason: "The report could not be opened." }))
      .then((next) => {
        if (!current) return;
        setLoaded(next);
        onAvailable(next.status === "available");
      });
    return () => {
      current = false;
    };
  }, [caseId, item, onAvailable, preloaded]);

  if (loaded.status === "loading") {
    return (
      <p className="m-0 mt-3 text-small text-muted" data-testid="source-report" data-source="loading">
        Opening the report…
      </p>
    );
  }
  if (loaded.status === "unavailable") {
    return (
      <p role="alert" className="m-0 mt-3 rounded-control bg-pill-negative-bg px-3 py-2 text-pill-negative-ink" data-testid="source-report" data-source="unavailable">
        {loaded.reason} You cannot sign until the report is shown.
      </p>
    );
  }
  const { source } = loaded;
  return (
    <figure className="m-0 mt-3" data-testid="source-report" data-source="available">
      <figcaption className="text-small font-medium">
        Report {source.report}, version {source.report_version}
      </figcaption>
      <pre tabIndex={0} className="m-0 mt-1 max-h-72 overflow-auto whitespace-pre-wrap rounded-control border border-rule bg-surface px-3 py-2 text-small text-ink">
        {source.text}
      </pre>
    </figure>
  );
}
