import type { ReviewState } from "@/lib/api/schema";
import type { ReviewView } from "./case-view";

const STATION: Record<string, string> = { caecum: "Caecum", mucosa: "Mucosa", retroflexion: "Retroflexion" };

/** The image model's read stays hidden until the viewer has an answer of their own on this item's evidence question. */
export function imageModelRevealed(states: readonly ReviewState[], item: string | undefined): boolean {
  return states.some((s) => s.item === item && s.question === "evidence" && s.reads.some((r) => r.submitted && r.decision !== undefined));
}

function ModelRead({ view }: { view: ReviewView }) {
  const m = view.imageContext;
  if (!m) return <p className="m-0 mt-2 text-small text-muted">No AI reading of the photos for this item.</p>;
  const conflict = view.items.evidence && m.hint === "image_prep_conflict";
  return (
    <div className="mt-3 rounded-control bg-raised px-4 py-3" data-testid="image-model">
      <p className="m-0 text-small font-medium">AI reading of the photos (not a clinician answer)</p>
      <p className="m-0 mt-1 text-small text-muted">
        Caecal photo {m.caecal} · bowel prep from the photos: {m.prep}
        {m.hint === "prep_grade_one_click" && ". It suggests the prep is adequate, but you still grade it."}
      </p>
      {conflict && (
        <p className="m-0 mt-1 text-small">
          Conflict: the report text grades the prep {m.textPrep}, but the photos read {m.prep}. The AI can only hold the case. It cannot decide it.
        </p>
      )}
    </div>
  );
}

/** The photographs behind an image-backed CRC item, each with its capture station, shown before any answer is chosen. */
export function ReviewPhotos({ view, revealModel }: { view: ReviewView; revealModel: boolean }) {
  const shown = view.photos;
  return (
    <section aria-labelledby={`photos-h-${view.items.evidence ?? "case"}`} className="rounded-card border border-line bg-surface p-4 md:p-6" data-testid="review-photos">
      <h2 id={`photos-h-${view.items.evidence ?? "case"}`} className="m-0 text-section font-medium">
        Photos filed with this report
      </h2>
      <p className="m-0 mt-1 text-small text-muted">Synthetic images, shown as filed by the unit. Judge them yourself. Nothing is preselected.</p>
      {shown.length === 0 ? (
        <p className="m-0 mt-3" role="status">
          No photos are available for this item here, so none can be judged from this page. Nothing was assumed from the AI reading.
        </p>
      ) : (
        <ul className="m-0 mt-3 flex list-none flex-wrap gap-3 p-0">
          {shown.map((p) => (
            <li key={p.sha} className="w-56">
              <figure className="m-0">
                {p.available ? (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img src={p.src} alt={`${STATION[p.station] ?? p.station} photo`} className="aspect-square w-full rounded-control border border-line object-cover" />
                ) : (
                  <div className="flex aspect-square w-full items-center justify-center rounded-control border border-line text-small text-muted">Image unavailable</div>
                )}
                <figcaption className="mt-1 text-small text-muted">
                  Site: {STATION[p.station] ?? p.station}
                </figcaption>
              </figure>
            </li>
          ))}
        </ul>
      )}
      {revealModel ? (
        <ModelRead view={view} />
      ) : (
        <p className="m-0 mt-3 text-small text-muted" data-testid="image-model-hidden">
          The AI reading is shown after you give your own answer.
        </p>
      )}
    </section>
  );
}
