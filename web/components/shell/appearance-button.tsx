"use client";

import { Dialog, DialogContent, DialogDescription, DialogTitle, DialogTrigger } from "@/components/ui/dialog";

const JADE: [string, string, string][] = [
  ["50", "#F0F6F0", "Hover wash"],
  ["100", "#DFEBDF", "Selected row"],
  ["200", "#C1D8CA", "Selection edge"],
  ["300", "#98BDA9", "Data fill"],
  ["400", "#70A38A", "Timeline"],
  ["500", "#51826B", "Focus ring"],
  ["600", "#346B53", "Links"],
  ["700", "#18533E", "Primary action"],
  ["800", "#124534", "Hover action"],
  ["900", "#0D3429", "Pressed action"],
];
const LIMESTONE: [string, string, string][] = [
  ["50", "#FFFEF8", "Card surface"],
  ["100", "#F8F6ED", "Input surface"],
  ["200", "#F2EFE5", "Page ground"],
  ["300", "#E7E3D5", "Inset surface"],
  ["400", "#D6D2C2", "Divider"],
  ["500", "#B9B6A6", "Disabled edge"],
  ["600", "#858578", "Control edge"],
  ["700", "#62665A", "Secondary text"],
  ["800", "#454D42", "Body text"],
  ["900", "#20372E", "Heading ink"],
];
const STATES: [string, string, string][] = [
  ["Permission granted", "bg-jade-100 text-jade-800", "Jade 100 / 800"],
  ["Waiting", "bg-limestone-300 text-limestone-800", "Limestone 300 / 800"],
  ["Review pending", "bg-ochre-wash text-ochre-ink", "Ochre wash / ink"],
  ["Correction required", "bg-clay-wash text-clay-ink", "Clay wash / ink"],
];

function Scale({ title, use, tones }: { title: string; use: string; tones: [string, string, string][] }) {
  return (
    <section className="mt-5">
      <div className="mb-2 flex flex-wrap items-baseline justify-between gap-2">
        <h3 className="staff-display m-0 text-[24px]">{title}</h3>
        <p className="m-0 text-small text-muted">{use}</p>
      </div>
      <ul className="m-0 grid list-none grid-cols-5 overflow-hidden rounded-md border border-line bg-surface p-0 md:grid-cols-10">
        {tones.map(([step, hex, role]) => (
          <li key={step} className="min-w-0">
            <span aria-hidden className="block h-9 border-b border-line" style={{ background: hex }} />
            <span className="block px-1.5 pb-2 pt-1.5">
              <b className="block text-[12px]">{step}</b>
              <code className="block text-[11px] tabular-nums text-muted">{hex}</code>
              <small className="block pt-0.5 text-[11px] text-ink-soft">{role}</small>
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

/** The shared Appearance control: the jade & limestone colour roles the workspace uses. */
export function AppearanceButton() {
  return (
    <Dialog>
      <DialogTrigger asChild>
        <button
          type="button"
          className="min-h-9 rounded-[5px] border border-limestone-600 bg-limestone-50 px-3 text-[12px] font-semibold text-jade-700 hover:bg-jade-800 hover:text-limestone-50"
        >
          Appearance
        </button>
      </DialogTrigger>
      <DialogContent className="staff-tokens max-h-[90dvh] w-[min(1180px,96vw)] max-w-[96vw] overflow-auto bg-limestone-200 sm:max-w-[min(1180px,96vw)]">
        <div className="staff-heading">
          <DialogTitle className="staff-display m-0 text-[clamp(32px,4vw,52px)] font-medium">Jade &amp; limestone</DialogTitle>
          <DialogDescription className="m-0 mt-1 text-limestone-800">Colour roles for the Wasl staff workspace.</DialogDescription>
        </div>
        <Scale title="Jade" use="Selection, controls and data" tones={JADE} />
        <Scale title="Limestone" use="Surfaces, borders and type" tones={LIMESTONE} />
        <section className="mt-5">
          <h3 className="staff-display m-0 mb-2 text-[24px]">Status colours</h3>
          <ul className="m-0 grid list-none grid-cols-2 gap-2.5 p-0 md:grid-cols-4">
            {STATES.map(([label, cls, pair]) => (
              <li key={label} className="rounded-[5px] border border-line bg-surface px-3 py-2.5">
                <span className={`inline-block rounded px-2 py-0.5 text-[12px] font-semibold ${cls}`}>{label}</span>
                <small className="mt-1 block text-muted">{pair}</small>
              </li>
            ))}
          </ul>
        </section>
      </DialogContent>
    </Dialog>
  );
}
