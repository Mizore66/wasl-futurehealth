import { readFileSync } from "node:fs";
import path from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { HistoryCard } from "@/components/case/audit-view";
import { SignoffsCard } from "@/components/case/sections";
import { ledgerId, ledgerWords } from "@/components/case/words";
import { ownView, roundModel, slotFor } from "@/components/review/double-review/model";
import { DoubleReviewPanel } from "@/components/review/double-review/panel";
import { DoubleReviewReadsCard } from "@/components/review/double-review/reads-card";
import { doubleItems } from "@/components/review/double-review/section";
import { submitDoubleRead } from "@/app/app/review/[id]/double-review-actions";
import { deskCredentialName, getDoubleReview, postDoubleRead } from "@/lib/api/double-review";
import { Audit, CaseDetail, DoubleReviewError, DoubleReviewRound, DoubleReviewState, LedgerEvent } from "@/lib/api/schema";
import { caseStage } from "@/lib/domain/stage";
import { linkTrust } from "@/lib/people/identity";

vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));

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

  it("case detail and audit re-blind a read whose item lacks either initial read, even if the response leaked it", () => {
    const raw = load("double_review_blinded_case_detail.json");
    const leak = (e: Record<string, unknown>) => (e.type === "double_review" ? { ...e, blinded: false, decision: "same", note: "LEAKED", nonce: "f".repeat(32) } : e);
    const detail = CaseDetail.parse({ ...raw, api_reviews: raw.api_reviews.map(leak) });
    const read = detail.api_reviews[0];
    expect(read.ledger === "double_review" && [read.blinded, read.decision, read.note, "nonce" in read]).toEqual([true, undefined, undefined, false]);
    const audit = load("double_review_blinded_audit.json");
    const history = Audit.parse({ ...audit, history: audit.history.map(leak) }).history;
    expect(JSON.stringify(history)).not.toContain("LEAKED");
    expect(history[0].ledger === "double_review" && history[0].decision).toBeUndefined();
    // Once both initial reads exist the answers are served to queue readers, and kept.
    const full = CaseDetail.parse(load("double_review_case_detail.json")).api_reviews;
    expect(full.map((e) => (e.ledger === "double_review" ? e.decision : ""))).toEqual(["same", "different", "same"]);
  });

  it("an open read keeps only the allow-listed fields: no final, no unknown extras, no transition it cannot have made", () => {
    const raw = load("double_review_blinded_case_detail.json");
    const leak = (e: Record<string, unknown>) =>
      e.type === "double_review" ? { ...e, final: "same", phase_after: "agreed", extra_answer: "LEAKED-EXTRA", decision_hint: "same" } : e;
    const detail = CaseDetail.parse({ ...raw, api_reviews: raw.api_reviews.map(leak) });
    const read = detail.api_reviews[0];
    if (read.ledger !== "double_review") throw new Error("narrow");
    expect([read.final, read.phase_after, read.blinded, read.actor, read.read]).toEqual([undefined, "", true, "reviewer-one", "r1"]);
    expect(Object.keys(read).sort()).toEqual(
      ["action", "actor", "at", "blinded", "case", "hash", "head_signed", "item", "kind", "ledger", "mode", "phase_after", "prev", "question", "read", "role", "round_head", "round_seq", "type"].sort(),
    );
    expect(JSON.stringify(detail.api_reviews)).not.toMatch(/LEAKED-EXTRA|decision_hint/);
    const audit = load("double_review_blinded_audit.json");
    const history = Audit.parse({ ...audit, history: audit.history.map(leak) }).history;
    expect(history[0].ledger === "double_review" && [history[0].final, history[0].phase_after]).toEqual([undefined, ""]);
    // The real awaiting_reads transition is kept on an open read.
    const plain = CaseDetail.parse(raw).api_reviews[0];
    expect(plain.ledger === "double_review" && plain.phase_after).toBe("awaiting_reads");
    // Once both initial reads exist, final stays where the API put it.
    const settled = CaseDetail.parse(load("double_review_case_detail.json")).api_reviews;
    expect(settled.map((e) => (e.ledger === "double_review" ? (e.final ?? "") : ""))).toEqual(["", "", "same"]);
  });

  it("the case card shows a settled answer only for a settled round", () => {
    const raw = load("double_review_blinded_case_detail.json");
    // A final on an open read: blindOpenReads drops it, and the card would not show it even if it did not.
    const leaked = { ...raw, api_reviews: raw.api_reviews.map((e: Record<string, unknown>) => ({ ...e, final: "same", phase_after: "agreed" })) };
    expect(renderToStaticMarkup(DoubleReviewReadsCard({ detail: CaseDetail.parse(leaked) })!)).not.toContain("Settled answer");
    const parsed = CaseDetail.parse(leaked);
    const forced = { ...parsed, api_reviews: parsed.api_reviews.map((e) => (e.ledger === "double_review" ? { ...e, final: "same", phase_after: "agreed" } : e)) };
    expect(renderToStaticMarkup(DoubleReviewReadsCard({ detail: forced })!)).not.toContain("Settled answer");
    // Both initial reads present, but a final on the read that only opened a disagreement is not a settled answer.
    const full = CaseDetail.parse(load("double_review_case_detail.json"));
    const r2final = { ...full, api_reviews: full.api_reviews.map((e) => (e.ledger === "double_review" && e.read === "r2" ? { ...e, final: "different" } : e)) };
    const html = renderToStaticMarkup(DoubleReviewReadsCard({ detail: r2final })!);
    expect(html).not.toContain("Settled answer: Not the same patient");
    expect(html).toContain("Settled answer: Same patient");
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

describe("round state per item and read role (one deployment, one credential per role)", () => {
  it("a deployment that has not read sees r1 as given but hidden, and the API's open initial slot is its form", () => {
    const m = roundModel(round("double_review_state_blinded.json"), "reviewer-two", "crc");
    expect(m.cells.map((c) => [c.read, c.state])).toEqual([
      ["r1", "hidden"],
      ["r2", "empty"],
    ]);
    expect(m.form).toEqual({ read: "r2", senior: false, choices: [expect.objectContaining({ value: "same" }), expect.objectContaining({ value: "different" })] });
  });

  it("the author sees its own read as 'your read' and gets no second initial read, even if a response offered one", () => {
    const own = round("double_review_state_own.json");
    const m = roundModel(own, "reviewer-one", "crc");
    expect(m.cells[0]).toMatchObject({ read: "r1", state: "shown", decision: "same", mine: true });
    expect([m.form, m.why]).toEqual([undefined, "You gave r1 as reviewer-one. The other initial read must come from a different person."]);
    // The form follows the API's viewer_may, and the API refuses the second seat (403 same_actor) if a response lies.
    expect(slotFor({ ...own, viewer_may: ["r2"] }, false)).toBe("r2");
    expect(slotFor({ ...own, viewer_may: [] }, false)).toBeUndefined();
  });

  it("ownView drops every answer that is not the deployment's own while the round awaits its initial reads", () => {
    const blinded = state("double_review_state_blinded.json");
    const r = blinded.rounds.find((x) => x.item === LINK)!;
    // A misbehaving API that leaks the first reader's answer, note, nonce and a final to reviewer-two.
    const leaky: DoubleReviewState = {
      ...blinded,
      rounds: [{ ...r, final: "same", reads: { ...r.reads, r1: { ...r.reads.r1!, blinded: false, decision: "same", note: "LEAKED-NOTE", nonce: "f".repeat(32) } } }],
    };
    const view = ownView(leaky);
    const r1 = view.rounds[0].reads.r1!;
    expect([r1.blinded, r1.decision, r1.note, "nonce" in r1, view.rounds[0].final]).toEqual([true, undefined, undefined, false, undefined]);
    expect(JSON.stringify(view)).not.toContain("LEAKED-NOTE");
    // roundModel repeats the rule for a round that did not pass through ownView.
    expect(roundModel(leaky.rounds[0], "reviewer-two", "crc").cells[0].state).toBe("hidden");
    // The deployment's own answer survives, and every answer survives once the API reveals the round.
    expect(ownView(state("double_review_state_own.json")).rounds.find((x) => x.item === LINK)!.reads.r1!.decision).toBe("same");
    const revealed = ownView(state("double_review_state_disagreement.json")).rounds.find((x) => x.item === LINK)!;
    expect([revealed.reads.r1!.decision, revealed.reads.r2!.decision]).toEqual(["same", "different"]);
  });

  it("senior resolution is never offered before a disagreement, even if a response claimed it was", () => {
    const awaiting = round("double_review_state_blinded.json");
    const m = roundModel({ ...awaiting, viewer_may: [] }, "reviewer-three", "crc");
    expect(m.form).toBeUndefined();
    const forged: DoubleReviewRound = { ...awaiting, viewer_may: ["r3"] };
    expect(roundModel(forged, "reviewer-three", "crc").form).toBeUndefined();
    expect(roundModel({ ...awaiting, phase: "agreed", viewer_may: ["r3"] }, "reviewer-three", "crc").form).toBeUndefined();
    expect(slotFor(forged, true)).toBeUndefined();
  });

  it("after a disagreement a third person gets the senior form and both initial answers are shown", () => {
    const m = roundModel(round("double_review_state_disagreement.json"), "reviewer-three", "crc");
    expect(m.phase).toBe("disagreement");
    expect(m.form).toMatchObject({ read: "r3", senior: true });
    expect(m.cells.map((c) => [c.read, c.state, c.state === "shown" ? c.decision : ""])).toEqual([
      ["r1", "shown", "same"],
      ["r2", "shown", "different"],
      ["r3", "empty", ""],
    ]);
    // An initial reader has nothing to do now: the API does not offer it r3.
    const initial = roundModel({ ...round("double_review_state_disagreement.json"), viewer_may: [] }, "reviewer-one", "crc");
    expect([initial.form, initial.why]).toEqual([undefined, "This deployment gave r1 as reviewer-one. The senior read must come from a third person."]);
  });

  it("a resolved round shows every read and the settled answer, with no form", () => {
    const m = roundModel(round("double_review_state_resolved.json"), "reviewer-three", "crc");
    expect([m.phase, m.final, m.form, m.cells.length]).toEqual(["resolved", "same", undefined, 3]);
  });

  it("picks the items the API double-reviews", () => {
    const detail = CaseDetail.parse(load("double_review_case_detail.json"));
    expect(doubleItems(detail)).toEqual([
      { item: "rv-dr-link-0001", question: "link" },
      { item: "rv-dr-evidence-01", question: "evidence" },
    ]);
  });
});

describe("rendered reads", () => {
  const items = [
    { item: LINK, question: "link" as const },
    { item: "rv-dr-evidence-01", question: "evidence" as const },
  ];

  it("the panel has no seat switcher, says who this deployment is, and never renders another person's blinded answer", () => {
    const blinded = state("double_review_state_blinded.json");
    const r = blinded.rounds.find((x) => x.item === LINK)!;
    // Even a response that leaks r1's answer to reviewer-two renders it hidden.
    // (viewer_may is emptied so no client form renders outside an app router.)
    const leaky = { ...blinded, rounds: [{ ...r, viewer_may: [], reads: { ...r.reads, r1: { ...r.reads.r1!, blinded: false, decision: "same", note: "LEAKED-NOTE" } } }] };
    const html = renderToStaticMarkup(
      DoubleReviewPanel({
        caseId: blinded.case,
        programme: "crc",
        items,
        views: [
          { role: "reviewer", credential: "WASL_API_REVIEWER_TOKEN", status: "ok", state: leaky },
          { role: "clinician", credential: "WASL_API_CLINICIAN_TOKEN", status: "missing" },
        ],
      }),
    );
    expect(html).toContain("This deployment is one reader");
    expect(html).toContain("designed, not built");
    expect(html).toContain("this deployment is recorded by the API as <strong>reviewer-two</strong>");
    expect(html).toContain("given by reviewer-one");
    expect(html).toContain("Answer hidden until both initial reads are in.");
    expect(html).not.toContain("Same patient ·");
    expect(html).not.toContain("LEAKED-NOTE");
    expect(html).not.toMatch(/Read as|\?seat=|_2_TOKEN|_3_TOKEN/);
    expect(html).toContain("WASL_API_CLINICIAN_TOKEN</code> is not set on the server");
    expect(html).not.toContain("<form");
  });

  it("the author's own blinded read is shown to it as 'your read'", () => {
    const own = state("double_review_state_own.json");
    const html = renderToStaticMarkup(
      DoubleReviewPanel({ caseId: own.case, programme: "crc", items: [items[0]], views: [{ role: "reviewer", credential: "WASL_API_REVIEWER_TOKEN", status: "ok", state: own }] }),
    );
    expect(html).toContain("Same patient · your read, as reviewer-one");
    expect(html).toContain("synthetic first read note");
  });

  it("the case page lists each read separately, blinded ones without their answer", () => {
    const blinded = renderToStaticMarkup(DoubleReviewReadsCard({ detail: CaseDetail.parse(load("double_review_blinded_case_detail.json")) })!);
    expect(blinded).toContain("answer hidden until both initial reads are in");
    expect(blinded).not.toContain("Same patient");
    // A case detail that leaked an answer before both initial reads exist still renders it hidden.
    const raw = load("double_review_blinded_case_detail.json");
    const leaked = { ...raw, api_reviews: raw.api_reviews.map((e: Record<string, unknown>) => ({ ...e, blinded: false, decision: "same" })) };
    expect(renderToStaticMarkup(DoubleReviewReadsCard({ detail: CaseDetail.parse(leaked) })!)).not.toContain("Same patient");

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

  it("offers no undo or contest control on a double-review read, even live with every read active", () => {
    const audit = Audit.parse(load("double_review_audit.json"));
    const events = audit.history.map((e) => ({ hash: e.hash, kind: "review", actor: "", action: "signoff_link", target: "", status: "active" as const, at: "" }));
    const projection = { meta: audit.meta, case: audit.case, case_status: "active" as const, canonical_state: "open" as const, note: "", events };
    const html = renderToStaticMarkup(HistoryCard({ audit, programme: "crc", governance: { caseId: audit.case, projection } }));
    expect(html).toContain("Senior read: Same");
    expect(html).not.toMatch(/Append compensation|Contest review|Resolve contest/);
  });
});

describe("one deployment credential per question role", () => {
  const meta = { api: "wasl-api/1", data_mode: "simulated", run: "r", policy: "p", reader_model: "m", loaded_at: "2026-11-10T00:00:00Z", note: "synthetic" };
  const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

  it("reads with the role's own credential, never a _2_/_3_ credential, and sends no identity header", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-one-secret-123456");
    vi.stubEnv("WASL_API_REVIEWER_2_TOKEN", "reviewer-two-secret-123456");
    vi.stubEnv("WASL_API_REVIEWER_3_TOKEN", "reviewer-three-secret-1234");
    vi.stubEnv("WASL_API_CLINICIAN_TOKEN", "clinician-one-secret-12345");
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(json(200, { meta, ...load("double_review_state_blinded.json") })));
    vi.stubGlobal("fetch", fetchMock);
    await getDoubleReview("case/1", "reviewer");
    await getDoubleReview("case/1", "clinician");
    const auth = fetchMock.mock.calls.map(([, init]) => new Headers(init.headers).get("authorization"));
    expect(auth).toEqual(["Bearer reviewer-one-secret-123456", "Bearer clinician-one-secret-12345"]);
    for (const [, init] of fetchMock.mock.calls) {
      expect(new Headers(init.headers).has("x-wasl-actor")).toBe(false);
      expect(new Headers(init.headers).has("x-wasl-role")).toBe(false);
    }
    expect(fetchMock.mock.calls[0][0]).toBe("http://127.0.0.1:8088/api/cases/case%2F1/double-review");
    expect([deskCredentialName("clinician"), deskCredentialName("reviewer")]).toEqual(["WASL_API_CLINICIAN_TOKEN", "WASL_API_REVIEWER_TOKEN"]);
  });

  it("a missing credential fails instead of borrowing another", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_CLINICIAN_TOKEN", "");
    vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-one-secret-123456");
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    await expect(getDoubleReview("c", "clinician")).rejects.toThrow("WASL_API_CLINICIAN_TOKEN is required");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("returns the API's refusals and the recorded actor of a 201", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-two-secret-123456");
    const body = { item: LINK, read: "r2" as const, decision: "different", expected_head: "genesis" };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(403, load("double_review_refused_same_actor.json"))));
    expect(await postDoubleRead("c", "reviewer", body)).toMatchObject({ ok: false, status: 403, reason: "same_actor" });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(409, load("double_review_refused_head.json"))));
    expect(await postDoubleRead("c", "reviewer", body)).toMatchObject({ ok: false, status: 409, conflict: "head" });
    const created = load("double_review_post_r3.json");
    const fetchMock = vi.fn().mockResolvedValue(json(201, created));
    vi.stubGlobal("fetch", fetchMock);
    const res = await postDoubleRead("c", "reviewer", body);
    expect(res.ok && [res.response.event.actor, res.response.round.phase]).toEqual(["reviewer-three", "resolved"]);
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ ...body, note: "" });
  });

  it("fixtures mode cannot read or append double review", async () => {
    vi.stubEnv("WASL_API_URL", "");
    await expect(getDoubleReview("c", "reviewer")).rejects.toThrow("needs WASL_API_URL");
  });
});

describe("the server action acts through one credential and fills only the slot the API offers", () => {
  const meta = { api: "wasl-api/1", data_mode: "simulated", run: "r", policy: "p", reader_model: "m", loaded_at: "2026-11-10T00:00:00Z", note: "synthetic" };
  const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

  function live() {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-one-secret-123456");
    vi.stubEnv("WASL_API_REVIEWER_2_TOKEN", "reviewer-two-secret-123456");
    vi.stubEnv("WASL_API_REVIEWER_3_TOKEN", "reviewer-three-secret-1234");
  }
  const auths = (m: ReturnType<typeof vi.fn>) => m.mock.calls.map(([, init]) => new Headers(init.headers).get("authorization"));

  it("an initial read takes the open slot the API lists, with the role's one credential, whatever extra fields the browser sends", async () => {
    live();
    const blinded = load("double_review_state_blinded.json");
    const fetchMock = vi.fn().mockImplementation((_url: string, init: RequestInit) =>
      Promise.resolve(init.method === "POST" ? json(201, load("double_review_post_r1.json")) : json(200, { meta, ...blinded })),
    );
    vi.stubGlobal("fetch", fetchMock);
    const head = blinded.rounds.find((r: { item: string }) => r.item === LINK).head;
    const input = { caseId: "c", item: LINK, question: "link" as const, senior: false, decision: "different", expectedHead: head };
    // A browser naming a seat or credential is ignored: the action has no input that selects either.
    const res = await submitDoubleRead({ ...input, ...({ seat: "r1", read: "r1", credential: "reviewer_2" } as object) });
    expect(res.ok).toBe(true);
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toMatchObject({ item: LINK, read: "r2", decision: "different", expected_head: head });
    expect(auths(fetchMock)).toEqual(["Bearer reviewer-one-secret-123456", "Bearer reviewer-one-secret-123456"]);
  });

  it("the author of r1 is offered no second seat, so nothing is posted", async () => {
    live();
    const own = load("double_review_state_own.json");
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(json(200, { meta, ...own })));
    vi.stubGlobal("fetch", fetchMock);
    const head = own.rounds.find((r: { item: string }) => r.item === LINK).head;
    for (const senior of [false, true]) {
      const res = await submitDoubleRead({ caseId: "c", item: LINK, question: "link", senior, decision: "different", expectedHead: head });
      expect(res).toMatchObject({ ok: false, status: 0 });
    }
    expect(fetchMock.mock.calls.every(([, init]) => (init.method ?? "GET") === "GET")).toBe(true);
    expect(new Set(auths(fetchMock))).toEqual(new Set(["Bearer reviewer-one-secret-123456"]));
  });
});
