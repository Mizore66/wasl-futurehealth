# Independent double review and senior resolution (r1 / r2 / r3)

Status: **built** in the demo API (synthetic data, demo overlay). Code: `pipeline/api_double_review.go`; tests: `pipeline/api_double_review_test.go`; served-JSON fixtures for the web client: `web/tests/fixtures/double_review_*.json`.

The pipeline has always **simulated** independent reads: `ReviewerCfg.humanDouble` in `pipeline/governance.go` draws a second read for high-risk answers and a senior third read when the first two disagree. This route is the interactive counterpart: real, separately authenticated people give r1 and r2 blind to each other, and a third authorised person resolves a disagreement.

`POST /api/cases/{id}/review` is unchanged. It remains the single-signer path: one actor's answer per event, with no read role and no blinding. One signer answering in two roles is **not** independent double review, and nothing on `/review` claims to be. `/double-review` is its documented successor for the double-review workflow; the two share the ledger but not state (a `/review` event is never counted as a read).

## Endpoints

| Call | Purpose |
|---|---|
| `GET /api/cases/{id}/double-review` | Every double-reviewable item of the case, as the calling principal may see it |
| `POST /api/cases/{id}/double-review` | Submit one read (r1, r2 or r3) for one review item |

Both need `read_review_queue` (coordinator, clinician, reviewer), like every case route; the auditor gets 403 and reads the redacted history through `/audit` and `/export` instead. Any other method gets 405 with `Allow: GET, POST`.

## Identity and roles

- **Identity comes from the bearer token only.** The role and actor are the `WASL_API_TOKENS` entry (`token:role:actor`) the request authenticated with (`pipeline/api_security.go`). `X-Wasl-Role`/`X-Wasl-Actor` sent by a client are overwritten, and the body has no identity field (an unknown field is a 400).
- **Actors are compared trimmed and case-insensitive**, as undo does. Two tokens configured with the same actor (for example `reviewer:Reviewer-One` and `reviewer:reviewer-one`) are one person.
- **Independence depends on the operator.** The API can only refuse an actor *name* it has seen in the round. It cannot tell that two different names belong to one human, and with `ToLower(TrimSpace)` matching it cannot detect lookalikes either (`reviewer-one` vs `reviewer_one`, or a Unicode homoglyph such as Cyrillic `е` for Latin `e`). Operators must issue exactly one canonical actor name per human (for example a staff identifier in plain ASCII) and bind every credential that person holds to that name. Giving one person two names, or two credentials with different names, silently turns one person into two "independent" readers.
- **Question permission.** Only single-kind `link` and `evidence` review items take part. Every read of a link item needs `signoff_link` (role `reviewer`); every read of an evidence item needs `signoff_evidence` (role `clinician`). This mirrors `governance.go`, where the sign-off role follows the item kind.
- **Senior resolver (r3).** r3 needs the same question permission **and** must be a third actor, distinct from both initial readers. The demo token configuration has no seniority tier, so "senior" here means *a third authorised person who did not give either initial read*. A dedicated senior role would be a new RBAC role and token type: that is **designed, not built**. Deployments must give the r3 credential only to people they treat as senior.
- The demo credentials (`scripts/demo.sh`) bind `smoke-reviewer-1`, `-2` and `-3` to three reviewer tokens, so the link workflow can be run end to end against the API. The web page uses only the first of them (see "Web client" below). There is one clinician token by default, so a full evidence round needs two or three more clinician entries in `WASL_API_TOKENS`.

## Request

```json
{"item": "<review item id>", "read": "r1|r2|r3", "decision": "<answer>", "note": "…", "expected_head": "<round head>"}
```

- `decision`: the `/review` vocabulary for the item's question: `same|different` for a link; `adequate|inadequate` or the case programme's evidence labels (lung, DR, breast, LDCT, IFHAS; see "POST review") for evidence. A link answer on an evidence item is a 400.
- `note`: optional, at most 1000 characters, no surrounding whitespace, control or invisible format characters (newlines allowed).
- `expected_head`: the round's `head` from GET (`genesis` before the round's first read). Body at most 16 KiB, one strict JSON object, duplicate or unknown keys rejected.

## State machine (per review item)

| Phase | Meaning | Who may submit next |
|---|---|---|
| `awaiting_reads` | Fewer than two initial reads | r1 / r2: an authorised actor who has not given the other initial read |
| `agreed` | r1 and r2 gave the same answer; `final` is that answer | nobody (r3 → 409) |
| `disagreement` | r1 and r2 differ: **r3 is open** (`r3_open: true`) | r3: an authorised third actor |
| `resolved` | r3 recorded; `final` is the r3 answer | nobody |

r1 and r2 are independent seats, not an order: either may be submitted first. Each slot can be filled once. The senior answer may be any valid answer to the question; every read stays in the ledger whatever the outcome.

## Blinding

While a round is `awaiting_reads`, an initial read's `decision` and `note` are withheld from **every** response: this GET (except for the actor who wrote it), case detail `api_reviews`, `/audit` `history` and `/export`. The served event carries `"blinded": true` instead.

The stored event also carries a random 128-bit `nonce`, withheld together with the answer. Without it, the event `hash`, which is served, could be matched by hashing the event under each of the few possible answers. Once both initial reads exist, everything is revealed to queue readers (the `nonce` included, so the hash can be recomputed). The auditor never receives `actor`, `note` or `nonce`.

## Responses

**GET 200:**

```json
{"protocol": "wasl-double-review/1", "case": "…", "viewer": {"role": "reviewer", "actor": "smoke-reviewer-2"},
 "canonical_state": "open", "canonical_review": "open", "note": "…",
 "rounds": [{"item": "rv-…", "question": "link", "action": "signoff_link", "phase": "awaiting_reads",
             "head": "<hash or genesis>", "seq": 1, "r3_open": false, "viewer_may": ["r2"],
             "reads": {"r1": {"read": "r1", "actor": "smoke-reviewer-1", "role": "reviewer", "at": "…", "hash": "…", "blinded": true},
                       "r2": null, "r3": null}}]}
```

`final` is present only once `agreed` or `resolved`. `viewer_may` lists the reads this principal could submit now; the server applies the same checks as POST. A visible read has `decision` and `note`. `viewer.actor` is the actor the API will record for this credential, so a client can say who is recorded before anything is submitted.

**POST:**

| Status | When | Body |
|---|---|---|
| 201 | Read appended | `event` (the stored event, unblinded for its author), `round` (the author's view after the append), `canonical_unchanged: true`, `note` |
| 400 | Malformed body, bad `read`, `decision`, `note` or `expected_head`, an item that is not a single link/evidence question, or a missing verified actor | `error` |
| 403 | The role lacks the question permission (`reason: "rbac"`), or the actor already gave the other initial read, or gave r1 or r2 and tries r3 (`reason: "same_actor"`) | `error`, `reason` |
| 404 | Unknown case, or an item not raised for this case | `error` |
| 409 | `conflict: "head"`: stale `expected_head` (with `current_head`); `conflict: "read"`: that initial slot is already filled (with `phase`), which is also what any r1 or r2 gets once both initial reads exist; `conflict: "phase"`: r3 outside `disagreement` (with `phase`) | `error`, `conflict`, … |
| 500 | Ledger failed validation at start-up, or the append failed (nothing recorded) | `error` |

Checks run in a fixed order, all under the server's ledger mutex: body fields (400); item (404) and its question (400); question permission (403 `rbac`); answer (400); head (409); phase and slot (409); identity (403 `same_actor`). The append itself then takes the interprocess file lock.

## Ledger event

Each read is one event in the shared hash-chained `<run>/api_reviews.ndjson`:

```json
{"type": "double_review", "kind": "read", "case": "…", "item": "rv-…", "question": "link|evidence", "read": "r1|r2|r3",
 "decision": "…", "note": "…", "nonce": "<32 hex>", "role": "reviewer", "actor": "smoke-reviewer-1", "action": "signoff_link",
 "at": "<RFC 3339>", "head_signed": "<case decision signature>", "round_head": "<expected_head>", "round_seq": 1,
 "phase_after": "awaiting_reads|agreed|disagreement|resolved", "final": "<only when this read settles the round>",
 "mode": "demo", "prev": "<previous ledger hash>", "hash": "<sha256 of the event>"}
```

`phase_after` makes every transition explicit: the second initial read records `agreed` (with `final`) or `disagreement` (r3 opened), and r3 records `resolved` with `final`.

- **Append:** through the same path as every other ledger writer (`appendEventLocked`): interprocess `flock`, re-verification of the on-disk chain against the served one, write, `fsync`, close, then memory. Concurrency is per round: `round_head` must equal the round's current head, so of two writers holding the same head exactly one gets 201 and the other 409. Different rounds, and different cases, do not conflict.
- **Replay:** at start-up `loadReviews` re-applies every read with the write path's own checks (`checkDoubleRead`), plus an exact key set, nonce format, `round_seq`, `phase_after` and `final`. Fields the handler fills in itself must be what it would have written: `head_signed` equal to the case's decision signature, `at` a canonical UTC RFC 3339 time no earlier than the round's previous read (the handler clamps a wall clock that stepped back to that read's time), and a `nonce` no other read in the ledger carries (the handler redraws a collision). A chain-valid but impossible history (the same actor as r1 and r2, an r3 before disagreement, an unauthorised role, an extra field, a reused nonce, a forged `head_signed`, an out-of-order or non-canonical `at`) stops the replay. `healthz` then reports `review_ledger: invalid` and every ledger write (`/review`, `/actions`, `/consent`, `/undo`, `/double-review`) is refused. Nothing is repaired or skipped.
- **Undo/contest:** a read is not a `/review` event, so `/undo` refuses to target it (409). Reads are append-only; a later correction would be a new protocol event (not built).
- **Redacted audit and export:** `exportReviewEventAllow` keeps `type`, `kind`, `case`, `item`, `question`, `read`, `decision` (absent while blinded), `role`, `action`, `at`, `head_signed`, `round_head`, `round_seq`, `phase_after`, `final`, `blinded`, `mode`, `prev` and `hash`. It never keeps `actor`, `note` or `nonce`.

## Web client

**Built:** the review page's "Independent double review" panel (`web/components/review/double-review/`) acts as **one reader**. It reads and answers with exactly one server-held credential per question role, `WASL_API_REVIEWER_TOKEN` for link items and `WASL_API_CLINICIAN_TOKEN` for evidence items, and never with any other credential. There is no seat switcher. The browser names no seat and no credential; the server action fills the slot the API offers that credential (`viewer_may`). One deployment can therefore give at most one read of an item, and the API refuses a second initial read by its actor (403 `same_actor`).

- **Blinding in the page.** Only this deployment's own initial answer is shown while a round is `awaiting_reads`, labelled "your read". Before rendering, the server rebuilds the GET response from known fields and drops every other author's decision, note and `final` until both initial reads exist. Case detail and audit lists drop them too (`blindOpenReads`), so a misbehaving API still cannot put another person's blinded answer into the HTML or the RSC payload. `web/e2e/double-review-live.spec.ts` checks both payloads with a mock API that deliberately leaks.
- **Other readers** use their own deployments, or call this API with their own credentials. In the mock end-to-end spec they are simulated by POSTing to the mock API directly.
- **Designed, not built:** per-person web sign-in. That would map each signed-in person to exactly one credential, so that one web deployment could serve r1, r2 and r3 for different people. Until it exists, a web deployment must not hold several credentials for double review.

## What does not change

Reads are a demo overlay on the review ledger. They never change the canonical decision, `out_log`, decision signatures, the case's `state` or its `review` status (`none|open|signed` stays the pipeline's), and they do not drive the linker or closure path. In a live deployment the settled answer would be published as a signed sign-off with `reads=[r1,r2]` or `[r1,r2,r3]` and replayed through the pipeline, as the simulated sign-offs are. That path is **designed, not built**.
