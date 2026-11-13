import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { reviewView } from "@/components/review/case-view";
import { ReviewDesk } from "@/components/review/review-desk";
import { CaseDetail, ReviewState } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

const mocks = vi.hoisted(() => ({ token: vi.fn(), fetchPhoto: vi.fn() }));
vi.mock("@/app/app/review/[id]/actions", () => ({ claimReviewQuestion: vi.fn(), submitReviewRead: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }) }));
vi.mock("@/lib/api/reviewer-session", () => ({ reviewerToken: mocks.token }));
vi.mock("@/lib/api/source", () => ({ apiMode: () => "live", getReviewPhotoWithToken: mocks.fetchPhoto }));

const base = CaseDetail.parse(hold);
const ITEM = "rv-d55f7ccf9f12c409";
const CAECUM = "a".repeat(64);
const M1 = "b".repeat(64);
const M2 = "c".repeat(64);
const next = { href: "/app/review", label: "Back to Review", last: true };

function detailFor(reason: string | undefined, photos?: { sha: string; station: string; available: boolean }[]): CaseDetail {
  const d = structuredClone(base);
  d.review_items = d.review_items.filter((i) => i.kinds.includes("evidence")).map((i) => ({ ...i, reason }));
  d.evidence[0].image_reader = { model: "hk-mnv3s-v1/cached", caecal_photo: "verified", prep: "adequate", hint: reason ?? "" };
  d.review_photos = photos ? { [ITEM]: photos } : undefined;
  return d;
}

const evidenceState = (submitted: boolean) =>
  ReviewState.parse({
    item: ITEM, question: "evidence", state: submitted ? "awaiting_r2" : "awaiting_r1", head: "", next_read: submitted ? "r2" : "r1", may_submit: !submitted,
    claim_head: "c".repeat(64),
    claim: { owner: "Rev-A", role: "clinician", read: "r1", claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "active" },
    claim_actions: { claim: false, renew: true, release: true, takeover: false },
    reads: [{ read: "r1", submitted, ...(submitted ? { decision: "adequate", hash: "aaaaaaaaaaaa1111" } : {}) }],
    disagreement: false, canonical_unchanged: true,
  });

const render = (d: CaseDetail, mode: "live" | "fixtures", submitted = false) =>
  renderToStaticMarkup(createElement(ReviewDesk, { view: reviewView(d), mode, next, states: mode === "live" ? [evidenceState(submitted)] : undefined }));

describe("photos behind image-backed CRC items", () => {
  it("shows the caecal photo with its station and a case/item-bound source for a completion review", () => {
    const d = detailFor("completion_undocumented", [{ sha: CAECUM, station: "caecum", available: true }]);
    const view = reviewView(d);
    expect(view.photos).toEqual([{ sha: CAECUM, station: "caecum", available: true, src: `/app/review/${encodeURIComponent(d.case.id)}/photo/${ITEM}/${CAECUM}` }]);
    const markup = render(d, "live");
    expect(markup).toContain(`src="${view.photos[0].src}"`);
    expect(markup).toContain("Capture station: Caecum");
    expect(reviewView(d).photos).toEqual(view.photos);
  });

  it.each(["prep_grade_one_click", "image_prep_conflict"])("shows the mucosal views of a %s review with stations and preselects nothing", (reason) => {
    const markup = render(detailFor(reason, [{ sha: M1, station: "mucosa", available: true }, { sha: M2, station: "mucosa", available: true }]), "live");
    expect(markup).toContain(`/photo/${ITEM}/${M1}`);
    expect(markup).toContain(`/photo/${ITEM}/${M2}`);
    expect(markup).not.toContain(`/photo/${ITEM}/${CAECUM}`);
    expect(markup).not.toContain("checked");
  });

  it("hides the image model's read until the viewer's own first read exists, then labels it as model output", () => {
    const d = detailFor("image_prep_conflict", [{ sha: M1, station: "mucosa", available: true }]);
    const before = render(d, "live", false);
    expect(before).toContain("image-model-hidden");
    expect(before).not.toContain("hk-mnv3s-v1/cached");
    const after = render(d, "live", true);
    expect(after).toContain("hk-mnv3s-v1/cached");
    expect(after).toContain("model output, not a clinician answer");
    expect(after).toContain("Conflict: the report text grades the prep");
    expect(render(d, "fixtures")).not.toContain("hk-mnv3s-v1/cached");
  });

  it("says plainly when photos or pixels are unavailable and never fabricates an image", () => {
    const none = render(detailFor("completion_undocumented", []), "live");
    expect(none).toContain("No photos are available for this item here");
    expect(none).not.toContain("<img");
    const lost = render(detailFor("completion_undocumented", [{ sha: CAECUM, station: "caecum", available: false }]), "live");
    expect(lost).toContain("Pixels unavailable");
    expect(lost).not.toContain("<img");
    expect(render(detailFor("completion_undocumented", undefined), "fixtures")).toContain("No photos are available for this item here");
  });

  it("leaves ordinary text-only CRC reviews without a photo section", () => {
    for (const reason of [undefined, "prep_ungraded_day15", "evidence_disputed"]) {
      const markup = render(detailFor(reason, [{ sha: CAECUM, station: "caecum", available: true }]), "live");
      expect(markup).not.toContain("review-photos");
      expect(markup).not.toContain("<img");
    }
  });
});

describe("photo route", () => {
  const ctx = (id: string, item: string, sha: string) => ({ params: Promise.resolve({ id, item, sha }) }) as never;
  const get = async (id: string, item: string, sha: string) => (await import("@/app/app/review/[id]/photo/[item]/[sha]/route")).GET(undefined as never, ctx(id, item, sha));

  beforeEach(() => {
    mocks.token.mockReset();
    mocks.fetchPhoto.mockReset();
  });

  it("streams the API's image with the reviewer's own token and no-store", async () => {
    mocks.token.mockResolvedValue("tok");
    mocks.fetchPhoto.mockResolvedValue({ ok: true, bytes: new Uint8Array([1, 2, 3]).buffer, type: "image/png" });
    const res = await get(encodeURIComponent("prov/ServiceRequest/X-1"), ITEM, CAECUM);
    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toBe("image/png");
    expect(res.headers.get("cache-control")).toBe("no-store");
    expect(mocks.fetchPhoto).toHaveBeenCalledWith("prov/ServiceRequest/X-1", ITEM, CAECUM, "tok");
  });

  it("refuses without a reviewer session, and for path-like or malformed arguments, without calling the API", async () => {
    mocks.token.mockResolvedValue(undefined);
    expect((await get("prov%2FX", ITEM, CAECUM)).status).toBe(401);
    mocks.token.mockResolvedValue("tok");
    for (const [item, sha] of [[ITEM, ".."], [ITEM, CAECUM.toUpperCase()], [ITEM, CAECUM.slice(1)], ["..", CAECUM], ["a/b", CAECUM]]) {
      expect((await get("prov%2FX", item, sha)).status).toBe(404);
    }
    expect(mocks.fetchPhoto).not.toHaveBeenCalled();
  });

  it("passes the API's refusals through as fixed bodies", async () => {
    mocks.token.mockResolvedValue("tok");
    mocks.fetchPhoto.mockResolvedValue({ ok: false, status: 403 });
    const res = await get("prov%2FX", ITEM, CAECUM);
    expect(res.status).toBe(403);
    expect(await res.json()).toEqual({ error: "Photo unavailable." });
  });
});
