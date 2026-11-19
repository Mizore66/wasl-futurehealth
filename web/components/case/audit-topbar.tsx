"use client";

import { AppearanceButton } from "@/components/shell/appearance-button";
import { TopbarActionsPortal } from "@/components/shell/top-bar";

/**
 * Audit's own top-bar controls: Palette (the shared colour-role dialog) and Layout. The prototype's other layouts
 * (Rounds, Handoff board) are workspace-wide directions that render this page identically, so only the approved
 * Casebook layout is offered.
 */
export function AuditTopbarControls() {
  return (
    <TopbarActionsPortal>
      <AppearanceButton label="Palette" />
      <label className="flex items-center gap-[7px] text-[12px] text-muted">
        <span className="max-md:sr-only">Layout</span>
        <select
          defaultValue="casebook"
          className="min-h-9 rounded-[6px] border border-limestone-600 bg-limestone-50 px-2.5 text-[12px] font-semibold text-ink max-md:min-h-11"
        >
          <option value="casebook">Casebook</option>
        </select>
      </label>
    </TopbarActionsPortal>
  );
}
