import { cn } from "@/lib/utils";

export type ChainNodeKind = "system" | "human" | "current";

export type ChainNode = {
  id: string;
  /** system: hollow circle (link, reader, gate). human: filled square (a signature). current: hue circle. */
  kind: ChainNodeKind;
  who: string;
  what: string;
  when: string;
  hash?: string;
  /** The segment leading into this node was tampered with or failed. */
  broken?: boolean;
};

const KIND_WORD: Record<ChainNodeKind, string> = {
  system: "System decision",
  human: "Human signature",
  current: "Current step",
};

function Mark({ kind }: { kind: ChainNodeKind }) {
  return (
    <svg width="12" height="12" viewBox="0 0 12 12" role="img" aria-label={KIND_WORD[kind]} className="relative z-10 mt-1 flex-none">
      {kind === "human" && <rect x="1" y="1" width="10" height="10" fill="var(--color-ink)" />}
      {kind === "current" && <circle cx="6" cy="6" r="5" fill="var(--color-hue)" />}
      {kind === "system" && <circle cx="6" cy="6" r="4.25" fill="var(--color-surface)" stroke="var(--color-ink)" strokeWidth="1.5" />}
    </svg>
  );
}

/** A segment from this node's centre to the next node's centre; a gap with a cross when the next node is broken. */
function Segment({ broken }: { broken: boolean }) {
  if (!broken) return <span aria-hidden className="absolute -bottom-2.5 left-[5px] top-2.5 w-0.5 bg-ink" />;
  return (
    <span aria-hidden className="absolute -bottom-2.5 left-0 top-2.5 flex w-3 flex-col items-center">
      <span className="w-0.5 flex-1 bg-ink" />
      <svg width="10" height="10" viewBox="0 0 10 10" className="my-1.5 flex-none text-negative">
        <path d="M2 2l6 6M8 2l-6 6" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" />
      </svg>
      <span className="w-0.5 flex-1 bg-ink" />
    </span>
  );
}

/** The signature element: one line of ink, a node per decision, drawn the same on every surface. */
export function SealedChain({ nodes, className }: { nodes: ChainNode[]; className?: string }) {
  return (
    <ol className={cn("m-0 list-none p-0", className)} aria-label="Sealed chain">
      {nodes.map((n, i) => {
        const next = nodes[i + 1];
        return (
          <li key={n.id} data-kind={n.kind} data-broken={n.broken || undefined} className="relative flex gap-3 pb-5 last:pb-0">
            {next && <Segment broken={!!next.broken} />}
            <Mark kind={n.kind} />
            <div className="min-w-0 flex-1">
              <p className="m-0 text-body">
                <span className="font-medium">{n.who}</span> · {n.what}
              </p>
              <p className="m-0 text-small text-muted">
                {n.when}
                {n.broken && <span className="text-negative"> · chain broken before this step</span>}
              </p>
              {n.hash && (
                <details className="mt-1 text-small text-muted">
                  <summary className="inline-flex min-h-6 cursor-pointer items-center">Hash</summary>
                  <code className="break-all font-mono text-small">{n.hash}</code>
                </details>
              )}
            </div>
          </li>
        );
      })}
    </ol>
  );
}
