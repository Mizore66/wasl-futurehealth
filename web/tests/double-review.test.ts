import { readFileSync } from "node:fs";
import path from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { HistoryCard } from "@/components/case/audit-view";
import { SignoffsCard } from "@/components/case/sections";
import { ledgerId, ledgerWords } from "@/components/case/words";
import { roundModel } from "@/components/review/double-review/model";
import { DoubleReviewPanel } from "@/components/review/double-review/panel";
import { DoubleReviewReadsCard } from "@/components/review/double-review/reads-card";
import { doubleItems, parseSeat } from "@/components/review/double-review/section";
import { getDoubleReview, postDoubleRead, seatCredentialName } from "@/lib/api/double-review";
import { Audit, CaseDetail, DoubleReviewError, DoubleReviewRound, DoubleReviewState, LedgerEvent } from "@/lib/api/schema";
import { caseStage } from "@/lib/domain/stage";
import { linkTrust } from "@/lib/people/identity";

/*
 * fixtures/double_review_*.json are the Go API's own responses (pipeline/api_double_review_test.go,
 * TestDoubleReviewContractFixtures, WASL_WRITE_LEDGER_FIXTURES=1): reviewer-one gives r1 "same", reviewer-two r2
 * "different", reviewer-three resolves with r3 "same", each through its own bearer token. Not hand-written JSON.
 */
const load = (f: string) => JSON.parse(readFileSync(path.join(__dirname, "fixtures", f), "utf8"));
const LINK = "rv-dr-link-0001";

const state = (f: string) => DoubleReviewState.parse(load(f));
const round = (f: string, item = LINK) => state(f).rounds.find((r) => r.item === item)!;

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("double review contract (real Go handler output)", () => {
  it("parses every served state and keeps the first answer out of the other reader's view", () => {
    for (const f of ["double_review_state_blinded.json", "double_review_state_own.json", "double_review_state_disagreement.json", "double_review_state_resolved.json"]) {
      expect(DoubleReviewState.safeParse(load(f)).error?.issues ?? []).toEqual([]);
    }
    const blinded = state("double_review_state_blinded.json");
    expect(blinded.viewer).toEqual({ role: "reviewer", actor: "reviewer-two" });
    const r1 = blinded.rounds.find((r) => r.item === LINK)!.reads.r1!;
    expect([r1.blinded, r1.actor, r1.decision, r1.note]).toEqual([true, "reviewer-one", undefined, undefined]);
    expect(JSON.stringify(load("double_review_state_blinded.json"))).not.toContain("synthetic first read note");

    const own = round("double_review_state_own.json").reads.r1!;
    expect([own.blinded, own.decision]).toEqual([false, "same"]);
  });

  it("tags every read as double_review in case detail and audit, never as a single-signer review", () => {
    const blindedDetail = CaseDetail.parse(load("double_review_blinded_case_detail.json"));
    expect(blindedDetail.api_reviews.map((e) => e.ledger)).toEqual(["double_review"]);
    const first = blindedDetail.api_reviews[0];
    expect(first.ledger === "double_review" && [first.read, first.blinded, first.decision]).toEqual(["r1", true, undefined]);

    const detail = CaseDetail.parse(load("double_review_case_detail.json"));
    const reads = detail.api_reviews.flatMap((e) => (e.ledger === "double_review" ? [[e.read, e.decision, e.actor, e.phase_after, e.final ?? ""]] : []));
    expect(reads).toEqual([
      ["r1", "same", "reviewer-one", "awaiting_reads", ""],
      ["r2", "different", "reviewer-two", "disagreement", ""],
      ["r3", "same", "reviewer-three", "resolved", "same"],
    ]);
    expect(detail.api_reviews.some((e) => e.ledger === "review")).toBe(false);

    const audit = Audit.parse(load("double_review_audit.json"));
    expect(audit.history.map((e) => e.ledger)).toEqual(["double_review", "double_review", "double_review"]);
    for (const e of load("double_review_audit.json").history) {
      for (const k of ["actor", "note", "nonce"]) expect(e).not.toHaveProperty(k);
    }
    const blindAudit = load("double_review_blinded_audit.json").history;
    expect(blindAudit[0]).not.toHaveProperty("decision");
    expect(blindAudit[0].blinded).toBe(true);
  });

  it("parses the refusals with their machine-readable reason", () => {
    expect(DoubleReviewError.parse(load("double_review_refused_same_actor.json")).reason).toBe("same_actor");
    const head = DoubleReviewError.parse(load("double_review_refused_head.json"));
    expect([head.conflict, head.current_head]).toEqual(["head", round("double_review_state_blinded.json").head]);
  });

  it("an unknown or malformed double_review entry becomes a labelled ledger line instead of failing the page", () => {
    const future = LedgerEvent.parse({ type: "double_review", kind: "withdrawal", hash: "h" });
    expect(future.ledger).toBe("double_review");
    if (future.ledger !== "double_review") throw new Error("narrow");
    expect(ledgerWords(future)).toBe("Double review event (withdrawal)");
    expect(ledgerId(future)).toBe("double_review/withdrawal");
    // Even with every single-signer field present, a double-review read is not a review the client could count as signed.
    const lookalike = LedgerEvent.parse({ ...load("double_review_case_detail.json").api_reviews[0], decision: "same", note: "" });
    expect(lookalike.ledger).toBe("double_review");
  });

  it("does not count a double-review read as a signed single-signer answer in the client projections", () => {
    const detail = CaseDetail.parse(load("double_review_case_detail.json"));
    const withItem = { ...detail, review_items: [{ ...detail.review_items[0] }] };
    // The canonical review item stays open: the reads are a demo overlay, not the pipeline's sign-off.
    expect(caseStage(withItem).technical).toBe("review · link");
    const link = { ...detail.links[0], loop: detail.review_items[0].loop };
    expect(linkTrust(withItem, link)).toBe("held");
  });
});

describe("round state per item and read role", () => {
  it("the second reader sees r1 as given but hidden, and is offered only r2", () => {
    const m = roundModel(round("double_review_state_blinded.json"), "r2", "reviewer-two", "crc");
    expect(m.cells.map((c) => [c.read, c.state])).toEqual([
      ["r1", "hidden"],
      ["r2", "empty"],
    ]);
    expect(m.form).toEqual({ read: "r2", senior: false, choices: [expect.objectContaining({ value: "same" }), expect.objectContaining({ value: "different" })] });
  });

  it("senior resolution is never offered before a disagreement, even if a response claimed it was", () => {
    const awaiting = round("double_review_state_blinded.json");
    const m = roundModel(awaiting, "r3", "reviewer-three", "crc");
    expect([m.form, m.why]).toEqual([undefined, "Senior resolution opens only if the two initial reads disagree."]);
    const forged: DoubleReviewRound = { ...awaiting, viewer_may: ["r3"] };
    expect(roundModel(forged, "r3", "reviewer-three", "crc").form).toBeUndefined();
    expect(roundModel({ ...awaiting, phase: "agreed", viewer_may: ["r3"] }, "r3", "reviewer-three", "crc").form).toBeUndefined();
  });

  it("after a disagreement the senior seat gets the form and both initial answers are shown", () => {
    const m = roundModel(round("double_review_state_disagreement.json"), "r3", "reviewer-three", "crc");
    expect(m.phase).toBe("disagreement");
    expect(m.form).toMatchObject({ read: "r3", senior: true });
    expect(m.cells.map((c) => [c.read, c.state, c.state === "shown" ? c.decision : ""])).toEqual([
      ["r1", "shown", "same"],
      ["r2", "shown", "different"],
      ["r3", "empty", ""],
    ]);
    // An initial reader's seat has nothing to do now.
    expect(roundModel(round("double_review_state_disagreement.json"), "r1", "reviewer-one", "crc").form).toBeUndefined();
  });

  it("explains a credential bound to the person who already read, instead of offering a second initial read", () => {
    const sameActor: DoubleReviewRound = { ...round("double_review_state_blinded.json"), viewer_may: [] };
    const m = roundModel(sameActor, "r2", "reviewer-one", "crc");
    expect(m.form).toBeUndefined();
    expect(m.why).toContain("One person cannot give both initial reads");
  });

  it("a resolved round shows every read and the settled answer, with no form", () => {
    const m = roundModel(round("double_review_state_resolved.json"), "r3", "reviewer-three", "crc");
    expect([m.phase, m.final, m.form, m.cells.length]).toEqual(["resolved", "same", undefined, 3]);
  });

  it("picks the items the API double-reviews and parses the seat", () => {
    const detail = CaseDetail.parse(load("double_review_case_detail.json"));
    expect(doubleItems(detail)).toEqual([
      { item: "rv-dr-link-0001", question: "link" },
      { item: "rv-dr-evidence-01", question: "evidence" },
    ]);
    expect([parseSeat("r2"), parseSeat("r3"), parseSeat(undefined), parseSeat("admin"), parseSeat(["r2"])]).toEqual(["r2", "r3", "r1", "r1", "r1"]);
  });
});

describe("rendered reads", () => {
  const hrefFor = { r1: "?seat=r1", r2: "?seat=r2", r3: "?seat=r3" };

  it("the panel for a seat with nothing to submit shows who the API records and never the hidden answer", () => {
    const blinded = state("double_review_state_blinded.json");
    const html = renderToStaticMarkup(
      DoubleReviewPanel({
        caseId: blinded.case,
        programme: "crc",
        seat: "r3",
        hrefFor,
        items: [{ item: LINK, question: "link" }, { item: "rv-dr-evidence-01", question: "evidence" }],
        views: [
          { role: "reviewer", credential: "WASL_API_REVIEWER_3_TOKEN", status: "ok", state: { ...blinded, viewer: { role: "reviewer", actor: "reviewer-three" } } },
          { role: "clinician", credential: "WASL_API_CLINICIAN_3_TOKEN", status: "missing" },
        ],
      }),
    );
    expect(html).toContain("does not make one person into independent reviewers");
    expect(html).toContain("recorded by the API as <strong>reviewer-three</strong>");
    expect(html).toContain("given by reviewer-one");
    expect(html).toContain("Answer hidden until your own read is in.");
    expect(html).not.toContain("Same patient");
    expect(html).toContain("Senior resolution opens only if the two initial reads disagree.");
    expect(html).toContain("WASL_API_CLINICIAN_3_TOKEN");
    expect(html).toContain("Nothing falls back to another seat");
    expect(html).not.toContain("<form");
  });

  it("the case page lists each read separately, blinded ones without their answer", () => {
    const blinded = renderToStaticMarkup(DoubleReviewReadsCard({ detail: CaseDetail.parse(load("double_review_blinded_case_detail.json")) })!);
    expect(blinded).toContain("answer hidden until both initial reads are in");
    expect(blinded).not.toContain("Same patient");

    const html = renderToStaticMarkup(DoubleReviewReadsCard({ detail: CaseDetail.parse(load("double_review_case_detail.json")) })!);
    for (const s of ["First read (r1)", "Second read (r2)", "Senior read (r3)", "reviewer-one", "reviewer-two", "reviewer-three", "senior read is open", "Settled answer: Same patient"]) {
      expect(html).toContain(s);
    }
    expect(html.match(/<li /g)).toHaveLength(3);
    // The single-signer card does not present reads as its own sign-offs.
    expect(renderToStaticMarkup(SignoffsCard({ detail: CaseDetail.parse(load("double_review_case_detail.json")) }))).not.toContain("reviewer-two");
  });

  it("the audit history names each read and its transition without an actor", () => {
    const html = renderToStaticMarkup(HistoryCard({ audit: Audit.parse(load("double_review_audit.json")), programme: "crc" }));
    expect(html).toContain("Second read: Different; the initial reads disagree, senior read opened");
    expect(html).toContain("Senior read: Same; resolved by the senior read");
    expect(html).not.toContain("reviewer-one");
  });
});

describe("seat credentials", () => {
  const meta = { api: "wasl-api/1", data_mode: "simulated", run: "r", policy: "p", reader_model: "m", loaded_at: "2026-11-10T00:00:00Z", note: "synthetic" };
  const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

  it("each seat sends its own credential and no identity header", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-one-secret-123456");
    vi.stubEnv("WASL_API_REVIEWER_2_TOKEN", "reviewer-two-secret-123456");
    vi.stubEnv("WASL_API_REVIEWER_3_TOKEN", "reviewer-three-secret-1234");
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(json(200, { meta, ...load("double_review_state_blinded.json") })));
    vi.stubGlobal("fetch", fetchMock);
    for (const seat of ["r1", "r2", "r3"] as const) await getDoubleReview("case/1", seat, "reviewer");
    const auth = fetchMock.mock.calls.map(([, init]) => new Headers(init.headers).get("authorization"));
    expect(auth).toEqual(["Bearer reviewer-one-secret-123456", "Bearer reviewer-two-secret-123456", "Bearer reviewer-three-secret-1234"]);
    for (const [, init] of fetchMock.mock.calls) {
      expect(new Headers(init.headers).has("x-wasl-actor")).toBe(false);
      expect(new Headers(init.headers).has("x-wasl-role")).toBe(false);
    }
    expect(fetchMock.mock.calls[0][0]).toBe("http://127.0.0.1:8088/api/cases/case%2F1/double-review");
    expect([seatCredentialName("r2", "clinician"), seatCredentialName("r1", "reviewer")]).toEqual(["WASL_API_CLINICIAN_2_TOKEN", "WASL_API_REVIEWER_TOKEN"]);
  });

  it("a missing seat credential fails instead of borrowing another seat's", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-one-secret-123456");
    vi.stubEnv("WASL_API_REVIEWER_2_TOKEN", "");
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    await expect(getDoubleReview("c", "r2", "reviewer")).rejects.toThrow("WASL_API_REVIEWER_2_TOKEN is required");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("returns the API's refusals and the recorded actor of a 201", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_REVIEWER_2_TOKEN", "reviewer-two-secret-123456");
    const body = { item: LINK, read: "r2" as const, decision: "different", expected_head: "genesis" };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(403, load("double_review_refused_same_actor.json"))));
    expect(await postDoubleRead("c", "r2", "reviewer", body)).toMatchObject({ ok: false, status: 403, reason: "same_actor" });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(409, load("double_review_refused_head.json"))));
    expect(await postDoubleRead("c", "r2", "reviewer", body)).toMatchObject({ ok: false, status: 409, conflict: "head" });
    const created = load("double_review_post_r3.json");
    const fetchMock = vi.fn().mockResolvedValue(json(201, created));
    vi.stubGlobal("fetch", fetchMock);
    const res = await postDoubleRead("c", "r2", "reviewer", body);
    expect(res.ok && [res.response.event.actor, res.response.round.phase]).toEqual(["reviewer-three", "resolved"]);
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ ...body, note: "" });
  });

  it("fixtures mode cannot read or append double review", async () => {
    vi.stubEnv("WASL_API_URL", "");
    await expect(getDoubleReview("c", "r1", "reviewer")).rejects.toThrow("needs WASL_API_URL");
  });
});
