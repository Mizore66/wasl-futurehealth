#!/usr/bin/env python3
"""Sanitized evidence store and tool API for the r1agent8 investigator (spec 2026-11-04-crc-r1agent8-evidence-design.md).

The controller (EvidenceStore) reads review_queue.json, link_funnel.ndjson, prep_log.ndjson, cohort fhir_*.ndjson and an
optional cached image-verdict file. The tool consumer (the model) only ever sees HMAC aliases and the sanitized tool
outputs below: never raw pk/item ids, file paths, truth, reviewer decisions, answer keys, report text or photo classes.
Raw item ids and pairs reappear only in the final NDJSON record built by terminal_record.
"""
import datetime, hashlib, hmac, json, os, re

ALIAS_RE = re.compile(r"(item|reg|rep|photo)_[a-f0-9]{32}")
MAX_TOOL_CALLS = 6
MAX_CITATIONS = 12
TOOLS = ("get_pair_context", "get_record", "get_identifiers", "get_report_fields", "get_photo_verdict")
LABELS = ("same", "different", "uncertain")
FORBIDDEN_REPORT_KEYS = frozenset({"quote", "quotes", "text", "raw", "truth", "answer"})
REPORT_FIELDS = ("kind", "agree", "label", "regex", "gemma")
REPORT_READER_FIELDS = ("state", "caecum", "prep", "polyps", "histology")
# stable reasons a terminal record may carry (valid same/different records carry "")
STABLE_REASONS = frozenset({
    "model_uncertain", "invalid_json", "invalid_output", "unsupported_label", "no_citations", "too_many_citations",
    "malformed_citation", "unknown_tool", "fabricated_citation", "value_mismatch", "tool_budget", "per_item_budget",
    "provider_timeout_abstain", "controller_abstain", "ineligible_item",
})
TOOL_SCHEMA_VERSION = "agent-evidence-tools-v1"


class ToolError(ValueError):
    """A tool call that the sandbox refuses (bad alias, out-of-scope alias, forbidden key)."""


class ToolBudgetError(ToolError):
    """More than six tool calls were attempted for one item."""


def canon(v):
    return json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def _norm_sex(s):
    s = (s or "").strip().lower()
    return {"male": "M", "m": "M", "female": "F", "f": "F"}.get(s, "")


def _real_dob(d):
    if not isinstance(d, str) or len(d) != 10:
        return False
    try:
        t = datetime.datetime.strptime(d, "%Y-%m-%d")
    except ValueError:
        return False
    return 1900 <= t.year <= 2100


def _placeholder_dob(d):
    return len(d) == 10 and (d.endswith("-01-01") or d.endswith("-07-01"))


def dob_status(d):
    if not d:
        return "missing"
    if not _real_dob(d):
        return "invalid"
    return "placeholder" if _placeholder_dob(d) else "real"


def _luhn_ok(s):
    if not (isinstance(s, str) and len(s) == 15 and s.isascii() and s.isdigit()):
        return False
    t = 0
    for i, c in enumerate(reversed(s)):
        d = int(c)
        if i % 2:
            d = d * 2 - 9 if d > 4 else d * 2
        t += d
    return t % 10 == 0


def eid_status(a, b):
    """Raw Emirates-ID comparison status for a pair: equal | different | missing | invalid (digits never leave)."""
    if not a or not b:
        return "missing"
    if not (_luhn_ok(a) and _luhn_ok(b)):
        return "invalid"
    return "equal" if a == b else "different"


def hard_conflict(a, b):
    """Controller recomputation of the hard conflicts (real DOB differs, known sex differs, two valid EIDs differ)."""
    if dob_status(a.get("dob")) == "real" and dob_status(b.get("dob")) == "real" and a["dob"] != b["dob"]:
        return "dob_conflict"
    if a.get("sex") and b.get("sex") and a["sex"] != b["sex"]:
        return "sex_conflict"
    if eid_status(a.get("eid"), b.get("eid")) == "different":
        return "eid_conflict"
    return ""


def _auto_ok(a, b):
    return (dob_status(a.get("dob")) == "real" and dob_status(b.get("dob")) == "real" and a["dob"] == b["dob"]
            and a.get("sex") and a.get("sex") == b.get("sex"))


def _pairkey(a, b):
    return tuple(sorted((a, b)))


def _within(root, path):
    r, p = os.path.realpath(root), os.path.realpath(path)
    return p == r or p.startswith(r + os.sep)


def _safe_file(root, rel):
    if os.path.isabs(rel) or ".." in rel.replace("\\", "/").split("/"):
        raise ToolError("path_rejected")
    p = os.path.join(root, rel)
    if not _within(root, p):
        raise ToolError("path_escapes_lane")
    return p


def _read_ndjson(path):
    out = []
    with open(path, encoding="utf-8") as f:
        for line in f:
            if line.strip():
                out.append(json.loads(line))
    return out


def _script(name):
    return "arabic" if any("؀" <= c <= "ۿ" or "ݐ" <= c <= "ݿ" for c in name or "") else "latin"


class _Reg:
    """One registration's demographics accumulated across messages; two distinct non-empty values mark it ambiguous."""
    def __init__(self, provider):
        self.v = {"provider": provider, "name": "", "dob": "", "sex": "", "eid": ""}
        self.ambiguous = False

    def add(self, name="", dob="", sex="", eid=""):
        if name and not self.v["name"]:
            self.v["name"] = name
        for k, x in (("dob", dob), ("sex", sex), ("eid", eid)):
            if x:
                if self.v[k] and self.v[k] != x:
                    self.ambiguous = True
                self.v[k] = x


class EvidenceStore:
    """records: pk -> {provider,name,dob,sex,eid}; identifiers: pk -> raw EID string (controller-only);
    reports: report id -> structured fields; photos: sha -> {landmark_p,prep_p}; targets: item -> {pair,p,report,photos,
    trace_ok}. Targets are candidates; eligibility (hard conflicts) is recomputed in need_items and tools_for."""

    def __init__(self, records, identifiers, reports, photos, targets, salt):
        if not isinstance(salt, (bytes, bytearray)) or len(salt) < 8:
            raise ValueError("salt must be run-local bytes")
        self._salt = bytes(salt)
        self.records = {k: dict(v) for k, v in records.items()}
        for k, v in identifiers.items():
            self.records.setdefault(k, {})["eid"] = v
        self.reports = {k: _clean_report(v) for k, v in reports.items()}
        self.photos = {k: dict(v) for k, v in photos.items()}
        self.targets = {k: dict(v) for k, v in targets.items()}
        self._calls = {}
        self._over = set()
        self._alias = {}
        for item, t in self.targets.items():
            self._reg(("item", item))
            for pk in t["pair"]:
                self._reg(("reg", pk))
            if t.get("report"):
                self._reg(("rep", t["report"]))
            for sha, _ in t.get("photos", []):
                self._reg(("photo", sha))

    # --- aliasing -------------------------------------------------------------------------------------------------
    def alias(self, kind, raw):
        return kind + "_" + hmac.new(self._salt, (kind + "\0" + raw).encode("utf-8"), hashlib.sha256).hexdigest()[:32]

    def _reg(self, key):
        a = self.alias(*key)
        self._alias[a] = key
        return a

    def _resolve(self, kind, alias):
        if not isinstance(alias, str) or not ALIAS_RE.fullmatch(alias) or not alias.startswith(kind + "_"):
            raise ToolError("bad_alias")
        key = self._alias.get(alias)
        if key is None:
            raise ToolError("unknown_alias")
        return key[1]

    # --- construction ---------------------------------------------------------------------------------------------
    @classmethod
    def from_lane(cls, lane_dir, salt=None, image_cache=None):
        lane = os.path.realpath(lane_dir)
        run = lane
        for rel in ("run/main", "run"):
            if os.path.isfile(os.path.join(lane, rel, "review_queue.json")):
                run = os.path.join(lane, rel)
                break
        cohort = os.path.join(lane, "cohort") if os.path.isdir(os.path.join(lane, "cohort")) else lane

        def rd(root, name):
            return _safe_file(root, name)

        with open(rd(run, "review_queue.json"), encoding="utf-8") as f:
            queue = json.load(f)
        trace = _read_ndjson(rd(run, "link_funnel.ndjson"))
        prep = _read_ndjson(rd(run, "prep_log.ndjson"))

        regs = {}
        for name in sorted(os.listdir(cohort)):
            if not (name.startswith("fhir_") and name.endswith(".ndjson")):
                continue
            for row in _read_ndjson(_safe_file(cohort, name)):
                res = row.get("res") or {}
                pats = [c for c in res.get("contained", []) if c.get("resourceType") == "Patient"]
                if res.get("resourceType") == "Patient":
                    pats.append(res)
                for p in pats:
                    mrn, eid = "", ""
                    for i in p.get("identifier", []):
                        s = i.get("system", "")
                        if s.endswith(":mrn"):
                            mrn = i.get("value", "")
                        elif "emirates-id" in s:
                            eid = i.get("value", "")
                    if not mrn:
                        continue
                    pk = row.get("prov", "") + "|" + mrn
                    nm = (p.get("name") or [{}])[0].get("text", "")
                    regs.setdefault(pk, _Reg(row.get("prov", ""))).add(nm, p.get("birthDate", "") or "", _norm_sex(p.get("gender")), eid)

        reports, report_photos, fallback = {}, {}, {}
        for row in prep:
            ev = (row.get("data") or {}).get("ev") if (row.get("data") or {}).get("kind") == "event" else None
            if not ev:
                continue
            pk = ev.get("pk")
            if pk and pk not in regs:
                fallback.setdefault(pk, _Reg(ev.get("provider", ""))).add(
                    ev.get("name", ""), ev.get("dob", "") or "", _norm_sex(ev.get("sex")), ev.get("eid", "") or "")
            evv = ev.get("evv")
            if evv and ev.get("cat") in ("colonoscopy", "pathology"):
                rid = "%s/DiagnosticReport/%s" % (ev.get("provider"), ev.get("id"))
                for k in {rid, ev.get("report"), ev.get("id")} - {None, ""}:
                    reports[k] = evv
                    report_photos[k] = [(i["sha"], i.get("station", "")) for i in ev.get("imgs") or [] if i.get("sha")]
        for pk, r in fallback.items():
            regs[pk] = r

        photo_cache = {}
        icp = image_cache
        if icp is None:
            for rel in ("image_verdicts.json", "models/image_verdicts.json"):
                if os.path.isfile(os.path.join(lane, rel)):
                    icp = _safe_file(lane, rel)
                    break
        if icp:
            with open(icp, encoding="utf-8") as f:
                for sha, v in (json.load(f).get("images") or {}).items():  # hk_class (the true class) is never copied
                    photo_cache[sha] = {"landmark_p": (v.get("landmark") or {}).get("p"), "prep_p": (v.get("prep") or {}).get("p")}

        near_ok, rejected = set(), set()
        for t in trace:
            if t.get("kind") != "pair":
                continue
            k = _pairkey(t.get("a"), t.get("b"))
            if t.get("stage") == "near" and t.get("reason") == "demographic_missing":
                near_ok.add(k)
            elif t.get("stage") in ("rejected_demographic", "unlinked"):
                rejected.add(k)

        records = {pk: dict(r.v, ambiguous=r.ambiguous) for pk, r in regs.items()}
        targets = {}
        for it in queue:
            if "link" not in (it.get("kinds") or []) or it.get("reason") or len(it.get("pair") or []) != 2:
                continue
            k = _pairkey(*it["pair"])
            if k not in near_ok or k in rejected:
                continue
            rep = it.get("report") or ""
            targets[it["item"]] = {"pair": list(it["pair"]), "p": it.get("p"), "report": rep if rep in reports else "",
                                   "photos": report_photos.get(rep, []) if rep in reports else []}
        pr = {r: v for r, v in reports.items()}
        ph = {sha: photo_cache.get(sha, {}) for t in targets.values() for sha, _ in t["photos"]}
        return cls(records, {}, pr, ph, targets, salt if salt is not None else os.urandom(32))

    # --- controller-side eligibility ------------------------------------------------------------------------------
    def _eligible(self, item):
        t = self.targets.get(item)
        if t is None:
            return False
        a, b = (self.records.get(pk) for pk in t["pair"])
        if not a or not b or a.get("ambiguous") or b.get("ambiguous") or t["pair"][0] == t["pair"][1]:
            return False
        if hard_conflict(a, b) or _auto_ok(a, b):
            return False
        return True

    def need_items(self, need_path):
        out, seen = [], set()
        for row in _read_ndjson(need_path):
            item = row.get("item")
            if item in seen or not self._eligible(item):
                continue
            if sorted(row.get("pair") or []) != sorted(self.targets[item]["pair"]):
                continue
            seen.add(item)
            out.append({"item": item, "pair": list(self.targets[item]["pair"]), "p": self.targets[item]["p"]})
        return out

    def tools_for(self, item_id):
        if not self._eligible(item_id):
            raise ToolError("item_not_eligible")
        return ToolAPI(self, item_id, metered=True)

    # --- per-item accounting --------------------------------------------------------------------------------------
    def _charge(self, item):
        n = self._calls.get(item, 0)
        if n >= MAX_TOOL_CALLS:
            self._over.add(item)
            raise ToolBudgetError("tool_budget")
        self._calls[item] = n + 1

    def calls_used(self, item):
        return self._calls.get(item, 0)

    # --- terminal records -----------------------------------------------------------------------------------------
    def terminal_record(self, item_id, model_output, metadata=None):
        """Always returns a record for an eligible item: a validated same/different, or uncertain with a stable reason."""
        if item_id not in self.targets:
            raise ToolError("item_not_eligible")
        t = self.targets[item_id]
        meta = _json_safe(metadata)
        label, reason, cites = "uncertain", "", []
        forced = meta.get("uncertain_reason") if isinstance(meta, dict) else None
        if not self._eligible(item_id):
            reason = "ineligible_item"
        elif item_id in self._over:
            reason = "tool_budget"
        elif forced in STABLE_REASONS:
            reason = forced
        else:
            label, reason, cites = self._validate(item_id, model_output)
        if label == "uncertain" and not reason:
            reason = "model_uncertain"
        return {"item": item_id, "pair": list(t["pair"]), "label": label, "reason": reason, "citations": cites,
                "calls": self.calls_used(item_id), "meta": meta}

    def _validate(self, item, out):
        if isinstance(out, (bytes, bytearray)):
            out = out.decode("utf-8", "replace")
        if isinstance(out, str):
            s = out.strip()
            if s.startswith("```"):
                s = re.sub(r"^```[a-zA-Z]*\s*|\s*```$", "", s)
            try:
                out = json.loads(s)
            except ValueError:
                return "uncertain", "invalid_json", []
        if not isinstance(out, dict):
            return "uncertain", "invalid_output", []
        label = out.get("label")
        if label not in LABELS:
            return "uncertain", "unsupported_label", []
        if label == "uncertain":
            return "uncertain", "model_uncertain", []
        cites = out.get("citations")
        if not isinstance(cites, list) or not cites:
            return "uncertain", "no_citations", []
        if len(cites) > MAX_CITATIONS:
            return "uncertain", "too_many_citations", []
        api = ToolAPI(self, item, metered=False)
        good = []
        for c in cites:
            if not (isinstance(c, dict) and all(k in c for k in ("tool", "arg", "field", "value")) and isinstance(c["field"], str)):
                return "uncertain", "malformed_citation", []
            if c["tool"] not in TOOLS:
                return "uncertain", "unknown_tool", []
            try:
                res = api.call(c["tool"], c["arg"])
            except ToolError:
                return "uncertain", "fabricated_citation", []
            cur = res
            for part in c["field"].split("."):
                if not isinstance(cur, dict) or part not in cur:
                    return "uncertain", "fabricated_citation", []
                cur = cur[part]
            if canon(cur) != canon(c["value"]):
                return "uncertain", "value_mismatch", []
            good.append({"tool": c["tool"], "arg": c["arg"], "field": c["field"], "value": cur})
        return label, "", good


def _json_safe(meta):
    if meta is None:
        return {}
    try:
        return json.loads(json.dumps(meta, sort_keys=True))
    except (TypeError, ValueError):
        return {"meta_error": True}


def _clean_report(v):
    """Structured reader fields only; any quote/text/raw/truth/answer key is dropped at every depth."""
    out = {}
    for k in REPORT_FIELDS:
        if k not in v or k in FORBIDDEN_REPORT_KEYS:
            continue
        x = v[k]
        if isinstance(x, dict):
            x = {kk: x[kk] for kk in REPORT_READER_FIELDS if kk in x and kk not in FORBIDDEN_REPORT_KEYS}
        out[k] = x
    return out


class ToolAPI:
    """The only object a tool consumer touches. Holds no paths or files; aliases resolve through the store."""

    def __init__(self, store, item, metered=True):
        self._s, self._item, self._metered = store, item, metered
        self.trace = []

    @property
    def item_alias(self):
        return self._s.alias("item", self._item)

    def call(self, tool, arg, **kw):
        if tool not in TOOLS:
            raise ToolError("unknown_tool")
        return getattr(self, tool)(arg, **kw)

    def _enter(self, tool, arg):
        if self._metered:
            self.trace.append({"tool": tool, "arg": arg if isinstance(arg, str) else "?"})
            self._s._charge(self._item)

    def _t(self):
        return self._s.targets[self._item]

    def get_pair_context(self, item_alias):
        self._enter("get_pair_context", item_alias)
        if self._s._resolve("item", item_alias) != self._item:
            raise ToolError("out_of_scope")
        s, t = self._s, self._t()
        out = {"item": item_alias, "pair": sorted(s.alias("reg", pk) for pk in t["pair"]), "p": t["p"]}
        if t.get("report"):
            out["report"] = s.alias("rep", t["report"])
        out["photos"] = [s.alias("photo", sha) for sha, _ in t.get("photos", [])]
        return out

    def _reg_rec(self, tool, alias):
        self._enter(tool, alias)
        pk = self._s._resolve("reg", alias)
        if pk not in self._t()["pair"]:
            raise ToolError("out_of_scope")
        return pk

    def get_record(self, reg_alias):
        pk = self._reg_rec("get_record", reg_alias)
        r = self._s.records[pk]
        return {"reg": reg_alias, "provider": r.get("provider", ""), "name": r.get("name", ""), "script": _script(r.get("name", "")),
                "dob": r.get("dob", ""), "dob_status": dob_status(r.get("dob")), "sex": r.get("sex") or "unknown"}

    def get_identifiers(self, reg_alias):
        pk = self._reg_rec("get_identifiers", reg_alias)
        a, b = (self._s.records[x] for x in self._t()["pair"])
        mine = self._s.records[pk]
        e = mine.get("eid", "")
        return {"reg": reg_alias, "eid_present": bool(e), "eid_checksum_valid": _luhn_ok(e), "eid_vs_other": eid_status(a.get("eid"), b.get("eid"))}

    def get_report_fields(self, report_alias, fields=None):
        self._enter("get_report_fields", report_alias)
        if fields is not None:
            if not isinstance(fields, (list, tuple)) or any(not isinstance(f, str) for f in fields):
                raise ToolError("bad_fields")
            if FORBIDDEN_REPORT_KEYS & {f.split(".")[0] for f in fields}:
                raise ToolError("forbidden_key")
            if any(f.split(".")[0] not in REPORT_FIELDS for f in fields):
                raise ToolError("unknown_field")
        rid = self._s._resolve("rep", report_alias)
        if rid != self._t().get("report"):
            raise ToolError("out_of_scope")
        data = dict(self._s.reports[rid])
        if fields is not None:
            data = {k: v for k, v in data.items() if k in {f.split(".")[0] for f in fields}}
        data["report"] = report_alias
        return data

    def get_photo_verdict(self, photo_alias):
        self._enter("get_photo_verdict", photo_alias)
        sha = self._s._resolve("photo", photo_alias)
        stations = dict(self._t().get("photos", []))
        if sha not in stations:
            raise ToolError("out_of_scope")
        v = self._s.photos.get(sha) or {}
        return {"photo": photo_alias, "station": stations[sha], "available": v.get("landmark_p") is not None,
                "landmark_p": v.get("landmark_p"), "prep_p": v.get("prep_p")}
