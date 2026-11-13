#!/usr/bin/env python3
"""Project redacted case exports and non-clinical API reads into frontend snapshot JSON.

No raw detail or audit response is accepted. Fields required by the frontend but deliberately removed by redaction are
empty strings; source values are never copied back into a case snapshot. Health, dashboard, fixture and list responses
use a separate projection that removes owner identities and rebuilds log-derived metadata.
"""
import argparse
import base64
import binascii
import copy
import hashlib
import json
from pathlib import Path
import re
import sys
import urllib.parse

SCHEMA = "wasl-case-export/1"
SENSITIVE_KEYS = {"quote", "actor", "target_actor", "registration", "text", "reasoning", "prompt", "raw_text", "excerpt"}
MAX_CANARY_INPUT = 16384


def _canary_string(value):
    if len(value) > MAX_CANARY_INPUT:
        return True  # fail closed rather than allocating an unbounded decoding buffer
    frontier = [value]
    seen = set()
    for _ in range(5):
        following = []
        found = False
        for candidate in frontier:
            if candidate in seen:
                continue
            if len(seen) >= 64:
                return True
            seen.add(candidate)
            if "CANARY" in candidate.upper():
                return True

            def append_decoded(decoded):
                nonlocal found
                if "CANARY" in decoded.upper():
                    found = True
                if decoded != candidate:
                    following.append(decoded)

            def append_decoded_bytes(decoded):
                nonlocal found
                try:
                    text = decoded.decode("utf-8")
                except UnicodeDecodeError:
                    # ASCII canary bytes remain detectable even when another byte is invalid UTF-8.
                    found = found or "CANARY" in decoded.decode("utf-8", errors="replace").upper()
                    return  # binary output is terminal for further text decoding
                append_decoded(text)

            if "%" in candidate:
                append_decoded(urllib.parse.unquote(candidate))
            if "\\u" in candidate:
                try:
                    append_decoded(json.loads('"' + candidate.replace('"', '\\"') + '"'))
                except ValueError:
                    pass
            compact = "".join(candidate.split())
            if len(compact) >= 8:
                padded = compact + "=" * (-len(compact) % 4)
                for encoded in (padded, padded.replace("-", "+").replace("_", "/")):
                    try:
                        append_decoded_bytes(base64.b64decode(encoded, validate=False))
                    except (ValueError, UnicodeDecodeError, binascii.Error):
                        pass
            if len(compact) >= 8 and len(compact) % 2 == 0:
                try:
                    append_decoded_bytes(bytes.fromhex(compact))
                except ValueError:
                    pass
        if found:
            return True
        frontier = following
        if not frontier:
            return False
    # A still-decodable chain beyond the depth limit is not evidence of safety.
    return bool(frontier)


_REGISTRATION_ALIAS = re.compile(r"^reg_[0-9a-f]{64}$")


def _registration_alias(value):
    """Return the Go projector's deterministic join alias; this is not semantic anonymity."""
    if not value or _REGISTRATION_ALIAS.fullmatch(value):
        return value
    digest = hashlib.sha256(b"wasl-registration-pseudonym-v1\0" + value.encode("utf-8")).hexdigest()
    return "reg_" + digest


def _pseudonymize_registration_fields(value, parent=""):
    if isinstance(value, dict):
        for key, child in value.items():
            registration_scalar = (key in {"referral_patient", "registration"} or
                                   parent == "weakest_edge" and key in {"a", "b"})
            if registration_scalar and isinstance(child, str):
                value[key] = _registration_alias(child)
            else:
                value[key] = _pseudonymize_registration_fields(child, key)
    elif isinstance(value, list):
        for index, child in enumerate(value):
            value[index] = _pseudonymize_registration_fields(child, parent)
    elif isinstance(value, str) and parent == "pair":
        return _registration_alias(value)
    return value


def _checked_export(export):
    if not isinstance(export, dict) or export.get("schema") != SCHEMA or export.get("redacted") is not True:
        raise ValueError("snapshot input must be a wasl-case-export/1 document marked redacted")
    for key in ("meta", "detail", "audit"):
        if not isinstance(export.get(key), dict):
            raise ValueError(f"redacted export is missing object {key}")

    def check(value, path="export"):
        if isinstance(value, dict):
            for key, child in value.items():
                # meta.note is fixed server text; every other note is a user narrative field.
                if key == "note" and path != "export.meta" and child not in (None, ""):
                    raise ValueError(f"unredacted narrative field at {path}.{key}")
                if key in SENSITIVE_KEYS and child not in (None, ""):
                    raise ValueError(f"unredacted sensitive field at {path}.{key}")
                if key in {"arg", "value"} and ".citations" in path and child not in (None, ""):
                    raise ValueError(f"unredacted citation identifier at {path}.{key}")
                if isinstance(child, str) and _canary_string(child):
                    raise ValueError(f"test canary at {path}.{key}")
                check(child, f"{path}.{key}")
        elif isinstance(value, list):
            for index, child in enumerate(value):
                check(child, f"{path}[{index}]")

    check(export)


def _review_defaults(events):
    for event in events if isinstance(events, list) else []:
        if isinstance(event, dict):
            event["actor"] = ""
            event["note"] = ""


def detail_snapshot(export):
    _checked_export(export)
    detail = copy.deepcopy(export["detail"])
    _pseudonymize_registration_fields(detail)
    detail["meta"] = copy.deepcopy(export["meta"])
    for identity in detail.get("identities", []):
        identity["registration"] = ""
    for item in detail.get("review_items", []):
        item["quote"] = ""
    for signoff in detail.get("signoffs", []):
        signoff.setdefault("verdict", "")
    _review_defaults(detail.get("api_reviews"))
    for investigation in detail.get("agent_investigations", []):
        for citation in investigation.get("citations", []):
            citation["arg"] = ""
            citation["value"] = ""
    if isinstance(detail.get("agent_provenance"), dict):
        detail["agent_provenance"]["prompt_ver"] = ""
    return detail


def audit_snapshot(export):
    _checked_export(export)
    audit = copy.deepcopy(export["audit"])
    _pseudonymize_registration_fields(audit)
    audit["meta"] = copy.deepcopy(export["meta"])
    _review_defaults(audit.get("history"))
    return audit


_PUBLIC_META_NOTE = "All patients, providers and outcomes are synthetic (Wikidata names, MTSamples report texts). Not real patient data."
_FIXED_READ_TEXT = {
    _PUBLIC_META_NOTE,
    "slots exist in another region: network pooling (cross-site booking) recovers them",
    "synthetic App 9 fields; Dik 2015 published score weights",
    "booking/RESULTS.md, Queue levers (simulated)",
    "Synthetic endoscopy calendar and constraint/no-show assumptions; not live scheduling. Optimiser ~= greedy (no optimisation claim).",
    "DoH CRC spec App. 2: referral after FIT+ -> colonoscopy conducted within 31 days; acceptable >90%, desirable >95%. Denominator: all FIT+ referrals with >=31 days of follow-up by day 120.",
    "counts 1-4 shown as <5",
    "CT at 18-24 months",
    "LDCT in 6 months",
    "comparison with prior chest CT, additional imaging and/or LDCT in 1-3 months",
    "consider CT at 18-24 months",
    "consider CT at 3 months, PET/CT or tissue sampling; manage by result",
    "continue annual screening: LDCT in 12 months",
    "diagnostic chest CT, PET/CT and/or tissue sampling and/or clinical evaluation; MDT before referral (DoH 7.6.1)",
    "if stable, consider CT at 2 and 4 years",
    "if unchanged and solid component <6 mm, annual CT for 5 years; solid component >=6 mm is highly suspicious",
    "managed domains continue with the receiving clinic; not-confirmed domains return to routine IFHAS screening in 3 years",
    "no further follow-up if stable",
    "no routine follow-up",
    "routine IFHAS rescreen in 3 years (Appendix 7)",
    "routine IFHAS rescreen in 3 years (Appendix 7); prediabetes: lifestyle advice and a yearly HbA1c",
}
_PUBLIC_OBJECT_KEYS = set("""
AA AD DH abnormal_domains accepted_eligible ack_days acknowledged action actions api at
awaiting_histology awaiting_tissue baseline_any_proc_120d baseline_eid_only basis booked bp breast
by_programme by_region capacity capacity_share_by_region cases cell cervical clinic clinic_note
clinician_decision_no_fundal_view clinician_decision_outside_fleischner closed closed_chains closed_day
closed_pred closed_within_31d closure coded_birads coded_grade coded_lungrads coded_results_agree
colonoscopy_days condition consent consent_records_accepted consent_timing crc crc_kpi_31d cross_region
data_mode deadline_day deadline_days decisions definition dependencies diabetic_retinopathy direct
domain_deadline_days domain_status dr dr_exam_closure dr_recall_overdue dropoff due_day due_days
duplicate_outputs econsult_re_refer econsult_referrer_action econsult_rejected_clinician_review
econsult_unanswered_3d eid_only eligible_referrals escalated escalations event evidence_closure
evidenced_closed exactly_once examined false_closure_rate_upper95 false_closure_trap false_closures
female_list finding_label finding_report fixture fixtures followup_ct_seen followup_documents
followup_ldct_not_done followup_needs_tracking followup_report forged forged_consent_target_genuinely_opted_out
forged_events forged_target_pair_falsely_merged_by_linker forged_with_any_effect from_day glyc grade_day
handoff_medical_director handoff_urgent_14d hash_verified head high id ifhas in_progress inadequate
incomplete_scan_not_completed ingest_adapter ingest_duplicates ingest_lost inputs intents_cancelled_by_consent
intents_ever_held_for_consent interval_days kind kpi_accepted_referrals_only kpi_all_referrals
kpi_among_procedures_seen last_action_at ldct ledger_status levers limit loaded_at loops lost_outputs
low lung lung_ldct lung_nodule materialised_run median_wait_days meta metrics misallocated_scenario missed
nats never_notified next_due next_from next_hi_days next_lo_days next_optional next_step no_ack_7d
no_acknowledgement no_assessment_outcome no_eye_exam no_followup_ct no_followup_documented
no_followup_needed no_recall_needed no_referral_needed no_show no_slot noncompliant_message
noncompliant_message_flagged none not_assessable not_booked not_notified_21d note notification_days
notified notified_within_21d nurse_call offset open open_cases_needing_slot open_econsult opted_out_loops
optional origin origins out_of_scope outcome overbooked overdue overdue_breast_recall
overdue_cervical_deadline overdue_crc_deadline overdue_dr_referral overdue_ifhas_followup
overdue_ldct_followup overdue_lung_nodule_window overdue_lung_surveillance_ct owner pathway
per_loop_agreement_with_truth policy pooling_note pooling_relevant precision prediabetes
prep_grade_unanswered_5wd prep_risk_band priority procedure_done procedures_seen programme protect
provenance provider providers rbac_denied_consent rbac_denied_signoff reader reader_model rebook_after_dna
rec recall refer_specialist_mdt referral_accepted referral_day referral_rejected referral_response_days
referral_unanswered referral_unanswered_7d rejected_inadequate rejected_no_show release_days
released_after_opt_out_on_file released_before_opt_out_recorded released_total
released_without_granted_consent repeat_ct_not_assessable repeat_photo_overdue requires_clinician_order
resolved_day review review_hold review_ledger risk risk_not_recorded_treated_high risk_recorded run scan_day
scan_label scan_report schema scope screen_day screen_label screen_report seed shortfall_slots signed
simulated_effect sla slots slots_per_case slots_per_open_case small_cell_rule snooze state states status
suggested summary surveillance_after_workup tamper_detected teachback_clarify
teachback_misunderstood_callback teachback_needs_human_callback technical_repeat_not_done
technical_repeat_overdue throughput_ev_s tissue_after_avid_pet_overdue total tp truth ungradable_no_repeat
unowned until urgency urgent_dr_referral_open urgent_ifhas_result_open wasl_live wasl_live_with_review
wasl_no_review weight win_hi_days win_lo_days window_end_day window_start_day with_human_signoff
within_31d within_31d_share within_31d_share_everyone_else
within_31d_share_women_requiring_female_list within_deadline within_window workup_document
""".split()) | {""}
_OPERATIONAL_KEY = re.compile(r"^[A-Za-z0-9_./:-]{0,160}$")
_OPERATIONAL_STRING = re.compile(r"^[A-Za-z0-9_.:/|@=+#%<>;\-]{0,512}$")
_READ_DROPPED = SENSITIVE_KEYS | {"owner", "canonical_owner", "reason"}


def _operational_read(value, path):
    """Project numeric maps and fixed operational strings, never arbitrary prose."""
    if isinstance(value, dict):
        out = {}
        for key, child in value.items():
            if not isinstance(key, str) or not _OPERATIONAL_KEY.fullmatch(key) or _canary_string(key):
                raise ValueError(f"invalid public snapshot key at {path}")
            if key not in _PUBLIC_OBJECT_KEYS:
                continue
            if key not in _READ_DROPPED:
                out[key] = _operational_read(child, f"{path}.{key}")
        return out
    if isinstance(value, list):
        return [_operational_read(child, f"{path}[{index}]") for index, child in enumerate(value)]
    if isinstance(value, str):
        if value in _FIXED_READ_TEXT:
            return value
        if _canary_string(value) or not _OPERATIONAL_STRING.fullmatch(value):
            raise ValueError(f"non-operational string at {path}")
        return value
    if value is None or not isinstance(value, (bool, int, float)):
        raise ValueError(f"unsupported public snapshot value at {path}")
    return value


def _read_fields(value, fields, path, required=()):
    if not isinstance(value, dict):
        raise ValueError(f"public snapshot object required at {path}")
    missing = set(required) - value.keys()
    if missing:
        raise ValueError(f"public snapshot fields missing at {path}: {', '.join(sorted(missing))}")
    out = {}
    for key, rule in fields.items():
        if key not in value:
            continue
        child = value[key]
        child_path = f"{path}.{key}"
        if rule is None:
            out[key] = _operational_read(child, child_path)
        elif isinstance(rule, dict):
            out[key] = _read_fields(child, rule, child_path)
        else:
            out[key] = rule(child, child_path)
    return out


def _read_list(rule):
    def convert(value, path):
        if not isinstance(value, list):
            raise ValueError(f"public snapshot list required at {path}")
        return [rule(child, f"{path}[{index}]") for index, child in enumerate(value)]
    return convert


_SUMMARY_FIELDS = {
    key: None for key in (
        "id", "programme", "state", "dropoff", "provider", "referral_day", "deadline_day", "overdue", "priority",
        "consent", "review", "closed", "closed_day", "fixture", "decisions", "lung", "dr", "breast", "ldct", "ifhas", "api_overlay",
    )
}
_GROUP_FIELDS = {
    key: None for key in (
        "acknowledged", "closed_within_31d", "dropoff", "escalations", "evidenced_closed", "loops",
        "noncompliant_message_flagged", "notified", "notified_within_21d", "provider", "origin",
        "referral_accepted", "referral_rejected",
    )
}


def _read_summary(value, path):
    return _read_fields(value, _SUMMARY_FIELDS, path)


def _read_summary_map(value, path):
    if not isinstance(value, dict):
        raise ValueError(f"public snapshot map required at {path}")
    out = {}
    for key, child in value.items():
        if not isinstance(key, str) or not _OPERATIONAL_KEY.fullmatch(key) or _canary_string(key):
            raise ValueError(f"invalid public snapshot map key at {path}")
        out[key] = _read_summary(child, f"{path}.{key}")
    return out


def _strict_read(fields, optional=()):
    """Read required fields and discard every unlisted field, including nested secrets."""
    required = set(fields) - set(optional)
    return lambda value, path: _read_fields(value, fields, path, required=required)


_HEALTH_DEPENDENCIES = {key: None for key in ("materialised_run", "nats", "reader", "ingest_adapter", "review_ledger")}


def _read_health_dependencies(value, path):
    return _read_fields(value, _HEALTH_DEPENDENCIES, path, required=("materialised_run", "nats", "reader", "ingest_adapter"))


_OPERATIONS_CONSUMER = _strict_read({
    "stream": None, "consumer": None, "status": None, "num_pending": None, "num_ack_pending": None, "reason": None,
}, optional=("num_pending", "num_ack_pending", "reason"))
_OPERATIONS_RECOVERY = _strict_read({key: None for key in (
    "generated_resources", "fhir_msgs", "expected_outputs", "live_outputs",
    "ingest_lost", "ingest_duplicates", "lost_outputs", "duplicate_outputs",
)})
_OPERATIONS_UNMEASURED = _strict_read({"status": None, "reason": None})


_OPERATIONS_SCHEMA = {
    "schema": None, "observed_at": None,
    "api": _strict_read({
        "status": None, "source": None,
        "dependencies": _strict_read(_HEALTH_DEPENDENCIES),
    }),
    "nats": _strict_read({"status": None, "source": None, "reason": None}, optional=("reason",)),
    "queue": _strict_read({
        "status": None, "source": None, "coverage": None, "reason": None,
        "consumers": _read_list(_OPERATIONS_CONSUMER),
    }, optional=("reason",)),
    "run": _strict_read({
        "status": None, "scope": None, "source": None, "reason": None,
        "latency_ms": _strict_read({key: None for key in ("n", "p50", "p90", "p99", "max")}),
        "recovery": _OPERATIONS_RECOVERY,
    }, optional=("reason", "latency_ms", "recovery")),
    "api_measurements": _strict_read({"latency": _OPERATIONS_UNMEASURED, "error_rate": _OPERATIONS_UNMEASURED}),
    "backup": _strict_read({
        "status": None, "source": None, "bundle_schema": None, "created_at": None,
        "age_seconds": None, "verification_status": None, "reason": None,
    }, optional=("bundle_schema", "created_at", "age_seconds", "verification_status", "reason")),
}


_READ_SCHEMAS = {
    "health": {
        "status": None, "cases": None,
        "dependencies": _read_health_dependencies,
    },
    "dashboard": {
        key: None for key in ("cases", "closed", "overdue", "states", "dropoff", "review", "by_programme", "fixtures", "small_cell_rule")
    } | {
        "sla": {key: None for key in ("ack_days", "colonoscopy_days", "notification_days", "referral_response_days")},
        "providers": _read_list(lambda value, path: _read_fields(value, _GROUP_FIELDS, path)),
        "origins": _read_list(lambda value, path: _read_fields(value, _GROUP_FIELDS, path)),
        "metrics": {key: None for key in ("closure", "consent_timing", "crc_kpi_31d", "exactly_once", "forged", "provenance", "throughput_ev_s")},
        "capacity": {key: None for key in ("condition", "data_mode", "inputs", "levers", "note", "status", "summary")},
    },
    "fixtures": {"fixtures": _read_summary_map},
    "cases": {"total": None, "offset": None, "limit": None, "cases": _read_list(_read_summary)},
    "operations": _OPERATIONS_SCHEMA,
}


def _public_meta(source_meta):
    if not isinstance(source_meta, dict):
        raise ValueError("snapshot read projection meta must be an object")
    loaded_at = source_meta.get("loaded_at", "")
    if not isinstance(loaded_at, str) or not re.fullmatch(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z", loaded_at):
        loaded_at = ""
    return {
        "api": "wasl-api/1", "data_mode": "simulated", "run": "redacted-internal-audit",
        "policy": "redacted-internal-audit", "reader_model": "not-disclosed", "loaded_at": loaded_at,
        "note": _PUBLIC_META_NOTE,
    }


def _person_basis(value, path):
    if not isinstance(value, str) or len(value) > 4096 or "\n" in value or "\r" in value or _canary_string(value):
        raise ValueError(f"invalid canonical person basis at {path}")
    return value


def _person_registration(value, path):
    if not isinstance(value, str) or not _REGISTRATION_ALIAS.fullmatch(value):
        raise ValueError(f"raw or malformed registration at {path}")
    return value


_PERSON_OUTREACH = _strict_read({
    "kind": None, "ev": None, "basis": _person_basis, "p_cal": None, "released": None,
    "suppress": None, "guard": None, "pair": _read_list(_person_registration), "channel": None, "sender": None, "tpl": None,
}, optional=("suppress", "guard", "pair", "channel", "sender", "tpl"))
_PERSON_CONSENT_LOG = _strict_read({
    "ev": None, "kind": None, "from": None, "to": None, "contact": None,
}, optional=("ev", "kind", "from", "contact"))
_PERSON_DECISION = _strict_read({
    "decision_id": None, "in_seq": None, "state": None, "dropoff": None, "owner": None, "overdue": None,
    "escalations": None, "consent_log": _read_list(_PERSON_CONSENT_LOG), "outreach": _read_list(_PERSON_OUTREACH), "policy": None, "sig": None,
})
_PERSON_IDENTITY = _strict_read({"provider": None, "registration": _person_registration, "role": None})
_PERSON_TIMELINE = _strict_read({key: None for key in (
    "seq", "type", "id", "provider", "status", "day", "arrival_day", "category", "raw_sha", "source",
)}, optional=("arrival_day", "category", "raw_sha", "source", "status"))
_PERSON_LINK = _strict_read({
    "loop": None, "referral_patient": _person_registration, "downstream_event": None, "path_strength": None,
    "path_p_cal": None, "tau": None, "hops": None, "near_miss": None, "linker": None,
    "eid_contra": None, "review_pair_substituted": None,
    "weakest_edge": _strict_read({"a": _person_registration, "b": _person_registration, "p": None, "basis": None}),
}, optional=("eid_contra", "review_pair_substituted"))
_PERSON_HOLD = _strict_read({
    "loop": None, "state": None, "programme": None, "rule": None, "policy": None, "report": None,
    "age": None, "scope": None, "label": None, "due_day": None, "deadline_day": None,
    "reason": None, "why": None, "domain_status": None,
}, optional=("loop", "programme", "policy", "report", "age", "scope", "label", "due_day", "deadline_day", "reason", "why", "domain_status"))


def _person_summary(value, path):
    fields = dict(_SUMMARY_FIELDS)
    fields.pop("api_overlay", None)
    return _read_fields(value, fields, path)


_PERSON_DETAIL = _strict_read({
    "case": _person_summary,
    "identities": _read_list(_PERSON_IDENTITY),
    "timeline": _read_list(_PERSON_TIMELINE),
    "links": _read_list(_PERSON_LINK),
    "holds": _read_list(_PERSON_HOLD),
    "outreach": _read_list(_PERSON_OUTREACH),
    "decisions": _read_list(_PERSON_DECISION),
})
_PERSON_CASE = _strict_read({
    "case_id": None, "parent_case_id": None, "referral_registration": _person_registration, "detail": _PERSON_DETAIL,
    "contact": _strict_read({"canonical": None, "overlay_status": None, "contact_blocked": None, "canonical_unchanged": None}),
}, optional=("parent_case_id",))
_PERSON_OBLIGATION = _strict_read({
    "case_id": None, "programme": None, "kind": None, "due_day": None, "window_start_day": None,
    "status": None, "source_field": None, "basis": _person_basis,
}, optional=("window_start_day", "basis"))
_PERSON_INTENT = _strict_read({
    "case_id": None, "programme": None, "first_decision_id": None, "latest_decision_id": None,
    "latest_decision_sig": None, "lifecycle": None, "outreach": _PERSON_OUTREACH,
})
_PERSON_FLAG = _strict_read({"id": None, "match": None, "actionable": None, "intents": _read_list(_PERSON_INTENT)})
_PERSON_NEXT = _strict_read({
    "kind": None, "case_id": None, "outreach_flag_id": None, "priority": None, "deadline_day": None,
    "overdue": None, "reason_codes": None, "rule": None,
}, optional=("case_id", "outreach_flag_id", "deadline_day"))
_PERSON_SCHEMA = {
    "schema": None,
    "person": _strict_read({
        "id": None, "anchor_case_id": None, "as_of_day": None, "case_ids": None, "programmes": None,
        "registrations": _read_list(_strict_read({"registration": _person_registration, "providers": None})),
        "identity_provenance": _strict_read({"source": None, "artifact_schema": None, "artifact_sha256": None, "policy": None}),
    }),
    "cases": _read_list(_PERSON_CASE),
    "obligations": _read_list(_PERSON_OBLIGATION),
    "outreach_flags": _read_list(_PERSON_FLAG),
    "next_action": _PERSON_NEXT,
}


def person_snapshot(payload):
    """Default-deny public person journey; raw identity artifacts are not accepted."""
    if not isinstance(payload, dict) or payload.get("schema") != "wasl-person-journey/1":
        raise ValueError("person snapshot requires wasl-person-journey/1")
    output = _read_fields(payload, _PERSON_SCHEMA, "person", required=_PERSON_SCHEMA)
    output["meta"] = _public_meta(payload.get("meta", {}))
    _check_snapshot_canaries(output, "person")
    return output


def read_snapshot(payload, kind):
    """Project one documented public read through its default-deny response schema."""
    if not isinstance(payload, dict) or kind not in _READ_SCHEMAS:
        raise ValueError("snapshot read projection requires an object and a documented response kind")
    output = _read_fields(payload, _READ_SCHEMAS[kind], kind, required=_READ_SCHEMAS[kind])
    output["meta"] = _public_meta(payload.get("meta", {}))
    return output


def _check_snapshot_canaries(value, path):
    if isinstance(value, dict):
        for key, child in value.items():
            if isinstance(key, str) and _canary_string(key):
                raise ValueError(f"snapshot canary at {path}.{key}")
            _check_snapshot_canaries(child, f"{path}.{key}")
    elif isinstance(value, list):
        for index, child in enumerate(value):
            _check_snapshot_canaries(child, f"{path}[{index}]")
    elif isinstance(value, str) and _canary_string(value):
        raise ValueError(f"snapshot canary at {path}")


def scan_snapshot_directory(directory):
    """Fail before publication if any staged JSON string contains a raw or encoded canary."""
    root = Path(directory)
    if not root.is_dir():
        raise ValueError("snapshot staging directory is missing")
    for path in sorted(root.rglob("*")):
        if path.is_file():
            try:
                value = json.loads(path.read_text(encoding="utf-8"))
            except (OSError, UnicodeError, ValueError) as exc:
                raise ValueError(f"invalid staged JSON {path.name}: {exc}") from exc
            kind = None
            if path.name == "person_journey.json":
                kind = "person"
            elif path.name == "healthz.json":
                kind = "health"
            elif path.name == "operations.json":
                kind = "operations"
            elif path.name in {"dashboard.json", "fixtures.json"}:
                kind = path.stem
            elif re.fullmatch(r"cases_[a-z_]+_page1\.json", path.name):
                kind = "cases"
            if kind == "person":
                projected = person_snapshot(value)
            elif kind is not None:
                projected = read_snapshot(value, kind)
            elif not (re.fullmatch(r"case_[A-Za-z0-9_-]+(?:_audit)?\.json", path.name) or
                      re.fullmatch(r"[A-Za-z0-9_-]+\.export", path.name)):
                raise ValueError(f"unrecognized staged snapshot filename: {path.name}")
            if kind is not None:
                if projected != value:
                    raise ValueError(f"staged public snapshot failed its exact schema: {path.name}")
            _check_snapshot_canaries(value, str(path))


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("kind", choices=("detail", "audit", "health", "operations", "dashboard", "fixtures", "cases", "person", "scan-dir"))
    parser.add_argument("path", nargs="?")
    args = parser.parse_args(argv)
    try:
        if args.kind == "scan-dir":
            if not args.path:
                raise ValueError("scan-dir requires a staging directory")
            scan_snapshot_directory(args.path)
            return 0
        if args.path:
            raise ValueError("unexpected path argument")
        payload = json.load(sys.stdin)
        if args.kind == "detail":
            output = detail_snapshot(payload)
        elif args.kind == "audit":
            output = audit_snapshot(payload)
        elif args.kind == "person":
            output = person_snapshot(payload)
        else:
            output = read_snapshot(payload, args.kind)
    except (OSError, UnicodeError, ValueError, TypeError, KeyError) as exc:
        print(f"redacted snapshot refused: {exc}", file=sys.stderr)
        return 1
    json.dump(output, sys.stdout, ensure_ascii=False, indent=1)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
