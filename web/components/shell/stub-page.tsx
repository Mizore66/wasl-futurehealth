import { HonestyLabel } from "@/components/wasl/honesty-label";
import type { HonestyKind } from "@/lib/honesty";

/** Phase 1 placeholder: the route exists, its owner replaces this page in phase 2. */
export function StubPage({ title, board, honesty }: { title: string; board: string; honesty: HonestyKind }) {
  return (
    <>
      <header className="flex flex-col gap-1">
        <h1 className="m-0 text-title font-semibold">{title}</h1>
        <p className="m-0 text-body text-muted">TODO: port the {board} board.</p>
      </header>
      <div className="mt-6">
        <HonestyLabel kind={honesty} />
      </div>
    </>
  );
}
