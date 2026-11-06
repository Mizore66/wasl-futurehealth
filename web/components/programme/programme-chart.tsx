"use client";

import { ChartFigure, FILL, Legend, Mark } from "./chart-kit";
import type { ProgrammeRow } from "./model";

const n = (v: number) => v.toLocaleString("en-US");

function segments(r: ProgrammeRow): { key: string; fill: "solid" | "hatch" | "open"; count: number }[] {
  const all = [
    { key: "closed", fill: "solid" as const, count: r.closed },
    { key: "overdue", fill: "hatch" as const, count: r.overdue },
    ...(r.open === null ? [] : [{ key: "open", fill: "open" as const, count: r.open }]),
  ];
  return all.filter((s) => s.count > 0);
}

/** Closed, overdue and still open cases for each programme; a row opens the patient list for that programme. */
export function ProgrammeChart({ rows }: { rows: ProgrammeRow[] }) {
  const showOpen = rows.every((r) => r.open !== null);
  return (
    <ChartFigure
      caption="Every case in each programme, split into closed, overdue and still open. Bars show shares of that programme's cases. Select a row to open its patients."
      table={{
        caption: "Cases by programme",
        head: ["Programme", "Cases", "Closed", "Overdue", ...(showOpen ? ["Still open"] : [])],
        rows: rows.map((r) => [r.name, n(r.cases), n(r.closed), n(r.overdue), ...(showOpen ? [n(r.open ?? 0)] : [])]),
      }}
    >
      <ul className="m-0 list-none p-0">
        {rows.map((r) => {
          const segs = segments(r);
          const total = segs.reduce((t, s) => t + s.count, 0);
          return (
            <li key={r.key}>
              <Mark
                href={r.href}
                className="py-2 [&:hover_.row-name]:underline"
                tip={
                  <>
                    <strong className="font-medium">{r.name}</strong>: {n(r.closed)} closed, {n(r.overdue)} overdue
                    {r.open !== null && <>, {n(r.open)} still open</>}, of {n(r.cases)} cases. Opens the patient list.
                  </>
                }
              >
                <span className="flex items-baseline justify-between gap-3">
                  <span className="row-name text-body font-medium">{r.name}</span>
                  <span className="text-small text-muted">{n(r.cases)} cases</span>
                </span>
                <span aria-hidden className="mt-1.5 flex h-4 gap-0.5">
                  {segs.map((s) => (
                    <span key={s.key} className="block h-4 rounded-[3px]" style={{ ...FILL[s.fill], width: `${(s.count / total) * 100}%`, minWidth: 3 }} />
                  ))}
                </span>
                <span className="mt-1 block text-small text-ink-soft">
                  {n(r.closed)} closed · {n(r.overdue)} overdue{r.open !== null && <> · {n(r.open)} still open</>}
                </span>
              </Mark>
            </li>
          );
        })}
      </ul>
      <div className="mt-2">
        <Legend
          items={[
            { fill: "solid", label: "Closed" },
            { fill: "hatch", label: "Overdue" },
            ...(showOpen ? [{ fill: "open" as const, label: "Still open" }] : []),
          ]}
        />
      </div>
    </ChartFigure>
  );
}
