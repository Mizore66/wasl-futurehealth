import { createHash, randomUUID } from "node:crypto";
import { createServer, type IncomingMessage, type ServerResponse } from "node:http";

const port = Number(process.env.PATIENT_MOCK_PORT ?? 31987);
const offers = {
  version: createHash("sha256").update(["skmc-endo:2026-10-08:11:00", "skmc-endo:2026-10-09:09:30", "skmc-endo:2026-10-12:13:00"].join("\n")).digest("hex"),
  slots: [
    { slot_id: "skmc-endo:2026-10-09:09:30", unit: "skmc-endo", starts_at: "2026-10-09T09:30:00+04:00" },
    { slot_id: "skmc-endo:2026-10-08:11:00", unit: "skmc-endo", starts_at: "2026-10-08T11:00:00+04:00" },
    { slot_id: "skmc-endo:2026-10-12:13:00", unit: "skmc-endo", starts_at: "2026-10-12T13:00:00+04:00" },
  ],
};

type Journey = {
  schema: "wasl-patient-journey/1";
  head: string;
  seq: number;
  ledger_status: "ok";
  teach_back: { question_id: "crc-next-step-v1"; status: string; completed_at: string; event: string };
  contact_permission: { status: string; updated_at: string; event: string };
  slot_selection: { status: string; offers_version: string; slot_id: string; selected_at: string; event: string };
  preparation: { items: { item_id: string; completed: boolean; updated_at: string; event: string }[]; completed: number; total: 2 };
  last_action_at: string;
};
type Link = { token: string; expires: number; journey: Journey; requests: Map<string, string>; bodies: unknown[]; failNext?: "empty201" | "500" | "network" };
const links = new Map<string, Link>();
const hash = (value: string) => createHash("sha256").update(value).digest("hex");

function fresh(token: string, expires: number): Link {
  return {
    token,
    expires,
    requests: new Map(),
    bodies: [],
    journey: {
      schema: "wasl-patient-journey/1",
      head: hash(`canonical:${token}`),
      seq: 0,
      ledger_status: "ok",
      teach_back: { question_id: "crc-next-step-v1", status: "pending", completed_at: "", event: "" },
      contact_permission: { status: "unknown", updated_at: "", event: "" },
      slot_selection: { status: "unselected", offers_version: offers.version, slot_id: "", selected_at: "", event: "" },
      preparation: {
        items: [
          { item_id: "clear_fluids_and_prep", completed: false, updated_at: "", event: "" },
          { item_id: "escort_home", completed: false, updated_at: "", event: "" },
        ],
        completed: 0,
        total: 2,
      },
      last_action_at: "",
    },
  };
}

function send(res: ServerResponse, status: number, body?: unknown) {
  res.writeHead(status, { "content-type": "application/json", "cache-control": "no-store", "referrer-policy": "no-referrer" });
  res.end(body === undefined ? undefined : JSON.stringify(body));
}
const payload = (link: Link) => ({ meta: { api: "wasl-api/1", data_mode: "simulated" }, journey: link.journey, offers, expires_at: new Date(link.expires).toISOString() });
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
  if (kind === "teach_back_completed") j.teach_back = { ...j.teach_back, status: "completed", completed_at: at, event };
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

async function handle(req: IncomingMessage, res: ServerResponse) {
  const url = new URL(req.url ?? "/", `http://127.0.0.1:${port}`);
  if (url.pathname === "/__health") return send(res, 200, { ok: true });
  if (url.pathname === "/__mock/links" && req.method === "POST") {
    const input = JSON.parse(await body(req)) as { token: string; expired?: boolean };
    const expires = Math.floor((Date.now() + (input.expired ? -60_000 : 30 * 60_000)) / 1000) * 1000;
    links.set(input.token, fresh(input.token, expires));
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
  if (url.pathname === "/__mock/stale" && req.method === "POST") {
    const input = JSON.parse(await body(req)) as { token: string };
    const link = links.get(input.token);
    if (!link) return send(res, 404, { error: "unknown test link" });
    link.journey.head = hash(randomUUID());
    link.journey.seq += 1;
    return send(res, 200, { head: link.journey.head });
  }
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
  if (input.kind === "teach_back_completed" && (input.data.question_id !== "crc-next-step-v1" || input.data.answer_id !== "colonoscopy")) return send(res, 400, { error: "wrong answer" });
  if (input.kind === "slot_selected") {
    if (input.data.offers_version !== offers.version) return send(res, 409, conflict(link, "offers", "stale offers"));
    if (!offers.slots.some((slot) => slot.slot_id === input.data.slot_id)) return send(res, 400, { error: "unoffered slot" });
  }
  if (input.kind === "contact_permission_changed" && link.journey.contact_permission.status === "opted_out" && input.data.permission !== "opted_out") return send(res, 409, conflict(link, "consent", "terminal opt-out"));
  const j = link.journey;
  const noop =
    (input.kind === "teach_back_completed" && j.teach_back.status === "completed") ||
    (input.kind === "contact_permission_changed" && j.contact_permission.status === input.data.permission) ||
    (input.kind === "slot_selected" && j.slot_selection.slot_id === input.data.slot_id) ||
    (input.kind === "preparation_changed" && j.preparation.items.find((item) => item.item_id === input.data.item_id)?.completed === input.data.completed);
  if (noop) return send(res, 200, { event: null, journey: j, replayed: true });
  if (!["teach_back_completed", "contact_permission_changed", "slot_selected", "preparation_changed"].includes(input.kind)) return send(res, 400, { error: "unknown kind" });
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
