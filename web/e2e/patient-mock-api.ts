import { createHash, randomUUID } from "node:crypto";
import { createServer, type IncomingMessage, type ServerResponse } from "node:http";

const port = Number(process.env.PATIENT_MOCK_PORT ?? 31987);
const crcOffers = {
  procedure: "colonoscopy",
  version: createHash("sha256").update(["skmc-endo:2026-10-08:11:00", "skmc-endo:2026-10-09:09:30", "skmc-endo:2026-10-12:13:00"].join("\n")).digest("hex"),
  slots: [
    { slot_id: "skmc-endo:2026-10-09:09:30", unit: "skmc-endo", starts_at: "2026-10-09T09:30:00+04:00" },
    { slot_id: "skmc-endo:2026-10-08:11:00", unit: "skmc-endo", starts_at: "2026-10-08T11:00:00+04:00" },
    { slot_id: "skmc-endo:2026-10-12:13:00", unit: "skmc-endo", starts_at: "2026-10-12T13:00:00+04:00" },
  ],
};

/** A cervical case's own simulated booking candidates, as pipeline/api_patient_offers.go serves them: date only, region, no preparation. */
const cervicalOffers = {
  procedure: "colposcopy",
  source: "simulated_booking",
  version: createHash("sha256").update("simulated_booking\ncolposcopy\n\nsim:colposcopy:2:1\nsim:colposcopy:5:1").digest("hex"),
  slots: [
    { slot_id: "sim:colposcopy:2:1", unit: "AD-2", region: "AD", starts_at: "2026-06-25" },
    { slot_id: "sim:colposcopy:5:1", unit: "DH-1", region: "DH", starts_at: "2026-07-01" },
  ],
};
const noOffers = { procedure: "", version: "", slots: [] as { slot_id: string; unit: string; starts_at: string }[] };
/** The served next step and wrong answer per programme, as pipeline/api_patient.go patientPlanFor projects them. */
const PLANS: Record<string, { next: string; distractor: string }> = {
  crc: { next: "colonoscopy", distractor: "blood_test" },
  cervical: { next: "colposcopy", distractor: "wait_for_symptoms" },
  breast: { next: "recall", distractor: "wait_for_symptoms" },
  lung_ldct: { next: "annual", distractor: "urgent_visit" },
};

type Journey = {
  schema: "wasl-patient-journey/1";
  head: string;
  seq: number;
  ledger_status: "ok";
  programme: string;
  next_step: string;
  teach_back: {
    question_id: string;
    answers: string[];
    status: string;
    completed_at: string;
    event: string;
    understanding: string;
    attempts: number;
    callback?: { id: string; source: "patient_link"; state: string; owner: string; status: "open" | "resolved"; opened_at: string; due_at: string; overdue: boolean };
  };
  contact_permission: { status: string; updated_at: string; event: string };
  slot_selection: { status: string; offers_version: string; slot_id: string; selected_at: string; event: string };
  preparation: { items: { item_id: string; completed: boolean; updated_at: string; event: string }[]; completed: number; total: 0 | 2 };
  last_action_at: string;
};
type RightsRow = { receipt: string; kind: string; decision: string; submitted_at: string; reason: string; status: string; owner?: string; due_at?: string; overdue?: boolean; closure_held?: boolean };
type Link = {
  token: string;
  expires: number;
  journey: Journey;
  requests: Map<string, string>;
  bodies: unknown[];
  failNext?: "empty201" | "500" | "network";
  otp?: string;
  rights: RightsRow[];
  handoff: { status: string; acknowledged_at?: string; verified?: boolean };
};
const links = new Map<string, Link>();
const hash = (value: string) => createHash("sha256").update(value).digest("hex");

function fresh(token: string, expires: number, programme = "crc"): Link {
  const plan = PLANS[programme];
  const crc = programme === "crc";
  return {
    token,
    expires,
    requests: new Map(),
    bodies: [],
    rights: [],
    handoff: { status: "pending_approval" },
    journey: {
      schema: "wasl-patient-journey/1",
      head: hash(`canonical:${token}`),
      seq: 0,
      ledger_status: "ok",
      programme,
      next_step: plan.next,
      teach_back: {
        question_id: crc ? "crc-next-step-v1" : `${programme}-next-step-v1`,
        answers: [plan.distractor, plan.next, "not_sure"],
        status: "pending", completed_at: "", event: "", understanding: "not_answered", attempts: 0,
      },
      contact_permission: { status: "unknown", updated_at: "", event: "" },
      slot_selection: { status: "unselected", offers_version: crc ? crcOffers.version : programme === "cervical" ? cervicalOffers.version : "", slot_id: "", selected_at: "", event: "" },
      preparation: crc
        ? {
            items: [
              { item_id: "clear_fluids_and_prep", completed: false, updated_at: "", event: "" },
              { item_id: "escort_home", completed: false, updated_at: "", event: "" },
            ],
            completed: 0,
            total: 2,
          }
        : { items: [], completed: 0, total: 0 },
      last_action_at: "",
    },
  };
}

function send(res: ServerResponse, status: number, body?: unknown) {
  res.writeHead(status, { "content-type": "application/json", "cache-control": "no-store", "referrer-policy": "no-referrer" });
  res.end(body === undefined ? undefined : JSON.stringify(body));
}
const offersFor = (link: Link) => (link.journey.programme === "crc" ? crcOffers : link.journey.programme === "cervical" ? cervicalOffers : noOffers);
const payload = (link: Link) => ({ meta: { api: "wasl-api/1", data_mode: "simulated" }, journey: link.journey, offers: offersFor(link), expires_at: new Date(link.expires).toISOString() });
const conflict = (link: Link, kind: string, error: string) => ({ error, conflict: kind, current_head: link.journey.head, journey: link.journey });

function authorized(req: IncomingMessage): Link | null {
  const token = req.headers.authorization?.replace(/^Bearer /, "") ?? "";
  return links.get(token) ?? null;
}

function append(link: Link, kind: string, data: Record<string, unknown>) {
  const j = link.journey;
  const at = new Date().toISOString();
  const event = hash(`${link.token}:${j.seq + 1}:${randomUUID()}`);
  j.seq += 1;
  j.head = event;
  j.last_action_at = at;
  if (kind === "teach_back_completed") j.teach_back = { ...j.teach_back, status: "completed", completed_at: at, event, understanding: "understood", attempts: j.teach_back.attempts + 1 };
  if (kind === "teach_back_answered") {
    const state = data.answer_id === "not_sure" ? "needs_human" : "misunderstood";
    const callback = j.teach_back.callback?.status === "open" ? j.teach_back.callback : { id: event, source: "patient_link" as const, state, owner: "coordinator", status: "open" as const, opened_at: at, due_at: new Date(Date.parse(at) + 2 * 864e5).toISOString(), overdue: false };
    j.teach_back = { ...j.teach_back, status: "callback", event, understanding: state, attempts: j.teach_back.attempts + 1, callback };
  }
  if (kind === "contact_permission_changed") j.contact_permission = { status: String(data.permission), updated_at: at, event };
  if (kind === "slot_selected") j.slot_selection = { status: "selected", offers_version: String(data.offers_version), slot_id: String(data.slot_id), selected_at: at, event };
  if (kind === "preparation_changed") {
    const item = j.preparation.items.find((candidate) => candidate.item_id === data.item_id)!;
    item.completed = Boolean(data.completed);
    item.updated_at = at;
    item.event = event;
    j.preparation.completed = j.preparation.items.filter((candidate) => candidate.completed).length;
  }
  return { hash: event, kind, at, seq: j.seq, data };
}

async function body(req: IncomingMessage): Promise<string> {
  let raw = "";
  for await (const chunk of req) raw += chunk;
  return raw;
}

/** Mirrors pipeline/api_patient_request.go: a simulated code shown to the caller, consumed once by a case-bound request. */
async function rights(req: IncomingMessage, res: ServerResponse, path: string) {
  const link = authorized(req);
  if (!link) return send(res, 401, { error: "patient link is missing or invalid" });
  const decision = link.journey.head;
  if (path === "/api/patient/verification") {
    link.otp = String(100000 + Math.floor(Math.random() * 900000));
    return send(res, 201, { delivery: "simulated", code: link.otp, expires_at: new Date(Date.now() + 600_000).toISOString() });
  }
  if (req.method === "GET") return send(res, 200, { meta: {}, requests: link.rights, current_decision: decision });
  const input = JSON.parse(await body(req)) as { kind: string; otp: string; reason: string; decision: string };
  if (!link.otp || input.otp !== link.otp) return send(res, 403, { error: "a valid one-time code for this link is required" });
  link.otp = undefined;
  const at = new Date().toISOString();
  const row: RightsRow = { receipt: hash(`${link.token}:${randomUUID()}`), kind: input.kind, decision: input.decision, submitted_at: at, reason: input.reason, status: "answered" };
  if (input.kind === "contest_submitted") Object.assign(row, { status: "open", owner: "senior_clinician", due_at: "2026-10-08T08:00:00Z", overdue: false, closure_held: true });
  link.rights.push(row);
  const explanation = input.kind === "explanation_requested"
    ? { decision, state: "closed_normal", closed: true, closing_report: "ahs/DiagnosticReport/COLO-1", matched_records: [{ provider: "ahs", role: "referral" }], closure_held: false }
    : undefined;
  return send(res, 201, { meta: {}, request: row, replayed: false, explanation });
}

const HANDOFF_VERSION = hash("outcome-handoff:routine_screening");
function handoffView(link: Link) {
  const { status, acknowledged_at, verified = false } = link.handoff;
  const releasable = status === "released" || status === "acknowledged";
  const shown = releasable && verified;
  return {
    status,
    delivery: "simulated",
    ...(releasable && { verified }),
    ...(shown && {
      version: HANDOFF_VERSION,
      approved_at: "2026-10-01T08:00:00Z",
      acknowledged_at,
      content: { outcome: "follow_up_complete", next_step: "routine_screening", safety_net: "safety_net_v1" },
    }),
  };
}

/**
 * Mirrors pipeline/api_outcome_handoff.go patientOutcomeRoute: content only once released and the link's one-time code is
 * verified ({otp}); acknowledgement ({version}) needs that verification or a fresh code.
 */
async function outcome(req: IncomingMessage, res: ServerResponse) {
  const link = authorized(req);
  if (!link) return send(res, 401, { error: "patient link is missing or invalid" });
  if (req.method === "GET") return send(res, 200, { meta: {}, handoff: handoffView(link) });
  const input = JSON.parse(await body(req)) as { version?: string; otp?: string };
  const releasable = link.handoff.status === "released" || link.handoff.status === "acknowledged";
  const codeOK = () => {
    const ok = !!link.otp && input.otp === link.otp;
    if (ok) link.otp = undefined;
    return ok;
  };
  if (!input.version) {
    if (!releasable) return send(res, 409, { error: "there is no released outcome to show", handoff: handoffView(link) });
    if (!codeOK()) return send(res, 403, { error: "a valid one-time code for this link is required" });
    link.handoff.verified = true;
    return send(res, 200, { meta: {}, handoff: handoffView(link) });
  }
  if (link.handoff.status === "acknowledged" && input.version === HANDOFF_VERSION) return send(res, 200, { meta: {}, replayed: true, handoff: handoffView(link) });
  if (link.handoff.status !== "released" || input.version !== HANDOFF_VERSION) return send(res, 409, { error: "there is no released outcome with that version to acknowledge" });
  if (!link.handoff.verified && !codeOK()) return send(res, 403, { error: "a valid one-time code for this link is required" });
  link.handoff = { status: "acknowledged", acknowledged_at: new Date().toISOString(), verified: true };
  return send(res, 201, { meta: {}, replayed: false, handoff: handoffView(link) });
}

async function handle(req: IncomingMessage, res: ServerResponse) {
  const url = new URL(req.url ?? "/", `http://127.0.0.1:${port}`);
  if (url.pathname === "/__health") return send(res, 200, { ok: true });
  if (url.pathname === "/__mock/links" && req.method === "POST") {
    const input = JSON.parse(await body(req)) as { token: string; expired?: boolean; programme?: string };
    if (input.programme && !PLANS[input.programme]) return send(res, 400, { error: "unknown programme" });
    const expires = Math.floor((Date.now() + (input.expired ? -60_000 : 30 * 60_000)) / 1000) * 1000;
    links.set(input.token, fresh(input.token, expires, input.programme));
    return send(res, 201, { expires_at: new Date(expires).toISOString() });
  }
  if (url.pathname === "/__mock/fail-next" && req.method === "POST") {
    const input = JSON.parse(await body(req)) as { token: string; mode: "empty201" | "500" | "network" };
    const link = links.get(input.token);
    if (!link) return send(res, 404, { error: "unknown test link" });
    link.failNext = input.mode;
    return send(res, 200, { ok: true });
  }
  if (url.pathname === "/__mock/expire" && req.method === "POST") {
    const input = JSON.parse(await body(req)) as { token: string };
    const link = links.get(input.token);
    if (!link) return send(res, 404, { error: "unknown test link" });
    link.expires = Date.now() - 1000;
    return send(res, 200, { ok: true });
  }
  if (url.pathname === "/__mock/requests" && req.method === "GET") {
    const link = links.get(url.searchParams.get("token") ?? "");
    if (!link) return send(res, 404, { error: "unknown test link" });
    return send(res, 200, link.bodies);
  }
  if (url.pathname === "/__mock/state" && req.method === "GET") {
    const link = links.get(url.searchParams.get("token") ?? "");
    if (!link) return send(res, 404, { error: "unknown test link" });
    return send(res, 200, payload(link));
  }
  if (url.pathname === "/__mock/handoff" && req.method === "POST") {
    const input = JSON.parse(await body(req)) as { token: string; mode: string };
    const link = links.get(input.token);
    if (!link) return send(res, 404, { error: "unknown test link" });
    link.handoff = { status: input.mode };
    return send(res, 200, { ok: true });
  }
  if (url.pathname === "/__mock/stale" && req.method === "POST") {
    const input = JSON.parse(await body(req)) as { token: string };
    const link = links.get(input.token);
    if (!link) return send(res, 404, { error: "unknown test link" });
    link.journey.head = hash(randomUUID());
    link.journey.seq += 1;
    return send(res, 200, { head: link.journey.head });
  }
  if (url.pathname === "/api/patient/verification" || url.pathname === "/api/patient/requests") return rights(req, res, url.pathname);
  if (url.pathname === "/api/patient/outcome") return outcome(req, res);
  if (url.pathname !== "/api/patient/journey") return send(res, 404, { error: "not found" });
  const link = authorized(req);
  if (!link) return send(res, 401, { error: "patient link is missing or invalid" });
  if (Date.now() >= link.expires) return send(res, 410, { error: "patient link has expired" });
  if (req.method === "GET") return send(res, 200, payload(link));
  if (req.method !== "POST") return send(res, 405, { error: "method not allowed" });
  let input: { kind: string; expected_head: string; request_id: string; data: Record<string, unknown> };
  try {
    input = JSON.parse(await body(req));
  } catch {
    return send(res, 400, { error: "malformed JSON" });
  }
  const fingerprint = JSON.stringify([input.kind, input.expected_head, input.data]);
  link.bodies.push(input);
  const previous = link.requests.get(input.request_id);
  if (previous) {
    if (previous !== fingerprint) return send(res, 409, conflict(link, "request", "request id changed"));
    return send(res, 200, { event: null, journey: link.journey, replayed: true });
  }
  if (input.expected_head !== link.journey.head) return send(res, 409, conflict(link, "head", "stale head"));
  const tb = link.journey.teach_back;
  if (input.kind === "teach_back_completed" && (input.data.question_id !== tb.question_id || input.data.answer_id !== link.journey.next_step)) return send(res, 400, { error: "wrong answer" });
  if (input.kind === "teach_back_answered" && (input.data.question_id !== tb.question_id || input.data.answer_id === link.journey.next_step || !tb.answers.includes(String(input.data.answer_id)))) return send(res, 400, { error: "unoffered answer" });
  const offers = offersFor(link);
  if ((input.kind === "slot_selected" || input.kind === "preparation_changed") && offers.slots.length === 0) return send(res, 400, { error: "no patient offers" });
  if (input.kind === "slot_selected") {
    if (input.data.offers_version !== offers.version) return send(res, 409, conflict(link, "offers", "stale offers"));
    if (!offers.slots.some((slot) => slot.slot_id === input.data.slot_id)) return send(res, 400, { error: "unoffered slot" });
  }
  if (input.kind === "contact_permission_changed" && link.journey.contact_permission.status === "opted_out" && input.data.permission !== "opted_out") return send(res, 409, conflict(link, "consent", "terminal opt-out"));
  const j = link.journey;
  const noop =
    ((input.kind === "teach_back_completed" || input.kind === "teach_back_answered") && j.teach_back.status === "completed") ||
    (input.kind === "contact_permission_changed" && j.contact_permission.status === input.data.permission) ||
    (input.kind === "slot_selected" && j.slot_selection.slot_id === input.data.slot_id) ||
    (input.kind === "preparation_changed" && j.preparation.items.find((item) => item.item_id === input.data.item_id)?.completed === input.data.completed);
  if (noop) return send(res, 200, { event: null, journey: j, replayed: true });
  if (!["teach_back_completed", "teach_back_answered", "contact_permission_changed", "slot_selected", "preparation_changed"].includes(input.kind)) return send(res, 400, { error: "unknown kind" });
  if (link.failNext === "500") {
    link.failNext = undefined;
    return send(res, 500, { error: "append failed; nothing recorded" });
  }
  if (link.failNext === "network") {
    link.failNext = undefined;
    res.socket?.destroy();
    return;
  }
  const event = append(link, input.kind, input.data);
  link.requests.set(input.request_id, fingerprint);
  if (link.failNext === "empty201") {
    link.failNext = undefined;
    return send(res, 201);
  }
  return send(res, 201, { event, journey: link.journey, replayed: false });
}

createServer((req, res) => {
  void handle(req, res).catch(() => send(res, 500, { error: "mock failure" }));
}).listen(port, "127.0.0.1");
