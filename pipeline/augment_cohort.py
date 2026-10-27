"""v7 cohort = v6-impact cohort (1,500 CRC + 600 cervical) + the v6-gov governance additions (same rules, seed 606).
v7 change: every added resource carries meta.source/versionId/lastUpdated so the mock Malaffi server can raise it as a topic event.

Original v6 doc: v6 cohort = v5 cohort (unchanged) + appended governance resources:
  - Consent (outreach opt-out) for 4% of referral patients, recorded by a coordinator/clinician at the referring provider
  - 12 forged Consent 'deny' by unauthorised roles (auditor / none) for patients who did NOT opt out  -> must be rejected
  - 12 forged Provenance sign-offs on a provider feed (compromised integration account):
      6 evidence 'adequate' on truly inadequate reports, signed as role 'reviewer' (identity reviewers may not label evidence)
      6 link 'same' on truly different identity pairs, signed as role 'coordinator'          -> must be rejected
Writes cohort/ (copied provider feeds + additions), consent_truth.json (loop -> opted out), forged.json."""
import json, random, shutil, os, hashlib, collections, sys
# usage: python augment_cohort.py <base_cohort_dir> <base_run_dir> <out_dir>
#   base_cohort_dir: output of `wsi7 gen`; base_run_dir: output of `wsi7 sim -run ...` on it (provides review_queue.json)
BASE_COHORT, BASE_RUN, OUT = sys.argv[1], sys.argv[2], sys.argv[3]
os.makedirs(OUT, exist_ok=True)
r = random.Random(606)
truth = json.load(open(f'{BASE_COHORT}/truth.json'))
queue = json.load(open(f'{BASE_RUN}/review_queue.json'))
for f in ['truth.json', 'cohort_summary.json']:
    shutil.copy(f'{BASE_COHORT}/{f}', f'{OUT}/{f}')
feeds = {}
srpk = {}  # loop -> (prov, mrn, patient resource, sr day)
for prov in ['ahs-mushrif', 'medlab', 'skmc-endo', 'noor-endo', 'corniche-wh']:
    rows = [json.loads(l) for l in open(f'{BASE_COHORT}/fhir_{prov}.ndjson')]
    feeds[prov] = rows
    for g in rows:
        res = g['res']
        if res['resourceType'] == 'ServiceRequest' and prov in ('ahs-mushrif', 'medlab'):
            loop = f"{prov}/ServiceRequest/{res['id']}"
            if loop in truth['loops']:
                pt = res['contained'][0]
                mrn = [i['value'] for i in pt['identifier'] if i['system'].endswith(':mrn')][0]
                srpk[loop] = (prov, mrn, pt, g['arrival'])
by_pk = collections.defaultdict(list)
for loop, (prov, mrn, pt, arr) in srpk.items():
    by_pk[(prov, mrn)].append(loop)
pks = sorted(by_pk)
r.shuffle(pks)
n_opt = round(0.04 * len(pks))
opt, forged_targets = pks[:n_opt], pks[n_opt:n_opt + 12]
consent_truth = {}
add = collections.defaultdict(list)
def consent(prov, mrn, pt, day, role, kind, cid):
    p = {k: v for k, v in pt.items() if k in ('resourceType', 'id', 'identifier', 'name', 'birthDate', 'gender')}
    res = {"resourceType": "Consent", "id": cid, "status": "active",
           "scope": {"coding": [{"system": "http://terminology.hl7.org/CodeSystem/consentscope", "code": "patient-privacy"}]},
           "category": [{"text": "screening-outreach-messaging"}], "contained": [p], "patient": {"reference": "#" + p['id']},
           "dateTime": f"2026-05-01T00:00:00Z", "provision": {"type": kind},
           "extension": [{"url": "urn:wsi:actor-role", "valueString": role}],
           "meta": {"source": f"urn:ae:provider:{prov}", "versionId": "1", "lastUpdated": "2026-05-01T00:00:00Z"}}
    import datetime
    res['dateTime'] = res['meta']['lastUpdated'] = (datetime.datetime(2026, 5, 1, tzinfo=datetime.timezone.utc) + datetime.timedelta(days=day)).strftime('%Y-%m-%dT%H:%M:%SZ')
    return res
# Stage 3: every referral patient has a contact-permission record ("screening-outreach-messaging"):
#   4% deny (opt-out), the rest permit, each arriving 0-0.5 d after the referral (so referral-before-consent is common).
#   A further 5% of permits are re-sent (duplicate) and 2% of patients later opt out after granting (permit then deny).
# consent_truth.json: loop -> {"final": "permit"|"deny", "first_day": d} ; opted-out loops keep value True for back-compat.
optset = set(opt)
consent_detail = {}
for i, (prov, mrn) in enumerate(pks):
    loop0 = by_pk[(prov, mrn)][0]
    _, _, pt, arr = srpk[loop0]
    day = arr + r.uniform(0.01, 0.5)
    role = 'coordinator' if r.random() < 0.8 else 'clinician'
    kind = 'deny' if (prov, mrn) in optset else 'permit'
    cid = f'consent-{i}'
    add[prov].append({"arrival": day, "prov": prov, "msgid": f"{prov}/Consent/{cid}/v1", "res": consent(prov, mrn, pt, day, role, kind, cid)})
    final = kind
    if kind == 'permit' and r.random() < 0.05:   # duplicate delivery of the same record (idempotency)
        add[prov].append({"arrival": day + r.uniform(0.01, 2), "prov": prov, "msgid": f"{prov}/Consent/{cid}/v1", "res": consent(prov, mrn, pt, day, role, kind, cid)})
    if kind == 'permit' and r.random() < 0.02:   # later withdrawal
        d2 = day + r.uniform(3, 30)
        add[prov].append({"arrival": d2, "prov": prov, "msgid": f"{prov}/Consent/{cid}-w/v1", "res": consent(prov, mrn, pt, d2, role, 'deny', cid + '-w')})
        final = 'deny'
    for l in by_pk[(prov, mrn)]:
        if kind == 'deny':
            consent_truth[l] = True
        consent_detail[l] = {"first": kind, "final": final, "first_day": round(day, 4)}
forged = []
for i, (prov, mrn) in enumerate(forged_targets):
    loop0 = by_pk[(prov, mrn)][0]
    _, _, pt, arr = srpk[loop0]
    day = arr + r.uniform(0.5, 3)
    role = ['auditor', ''][i % 2]
    cid = f'forged-consent-{i}'
    add[prov].append({"arrival": day, "prov": prov, "msgid": f"{prov}/Consent/{cid}/v1", "res": consent(prov, mrn, pt, day, role, "deny", cid)})
    forged.append({"type": "consent_deny", "role": role, "loops": by_pk[(prov, mrn)]})
ev_inad = [q for q in queue if q['kinds'] == ['evidence'] and truth['reports'].get(q['report']) == 'inadequate'
           and truth['loops'][q['loop']]['state'] != 'closed']
link_diff = [q for q in queue if q['kinds'] == ['link'] and truth['pk_qid'].get(q['pair'][0]) != truth['pk_qid'].get(q['pair'][1])]
r.shuffle(ev_inad)
picks = [(q, 'evidence', 'reviewer') for q in ev_inad[:6]] + [(q, 'link', 'coordinator') for q in link_diff[:6]]
for j, (q, kind, role) in enumerate(picks):
    so = {"item": q['item'], "loop": q['loop'], "kinds": [kind], "pair": q['pair'], "report": q['report'],
          "reviewer": "integration-account medlab-hl7", "signed_head": q['head'], "run": "", "role": role, "reads": ["x"]}
    if kind == 'link':
        so['link'] = 'same'
    else:
        so['evidence'] = 'adequate'
    res = {"resourceType": "Provenance", "id": f"forged-prov-{j}", "recorded": "2026-08-28T00:00:00Z",
           "target": [{"reference": q['loop']}], "agent": [{"who": {"display": so['reviewer']}}],
           "extension": [{"url": "urn:wsi:signoff", "valueString": json.dumps(so)}],
           "meta": {"source": "urn:ae:provider:medlab", "versionId": "1", "lastUpdated": "2026-08-28T00:00:00Z"}}
    add['medlab'].append({"arrival": 119.0 + j * 0.01, "prov": "medlab", "msgid": f"medlab/Provenance/forged-prov-{j}/v1", "res": res})
    forged.append({"type": f"signoff_{kind}", "role": role, "loop": q['loop'], "item": q['item'], "truth_state": truth['loops'][q['loop']]['state']})
for prov, rows in feeds.items():
    rows = rows + add[prov]
    rows.sort(key=lambda g: g['arrival'])
    with open(f'{OUT}/fhir_{prov}.ndjson', 'w') as f:
        for g in rows:
            f.write(json.dumps(g, ensure_ascii=False, separators=(',', ':')) + '\n')
json.dump(consent_truth, open(f'{OUT}/consent_truth.json', 'w'), indent=0)
json.dump(consent_detail, open(f'{OUT}/consent_detail.json', 'w'), indent=0)
json.dump(forged, open(f'{OUT}/forged.json', 'w'), indent=1)
print('referral pks', len(pks), 'opted out', len(opt), 'loops opted out', len(consent_truth), 'forged', len(forged),
      'added', {k: len(v) for k, v in add.items()})
