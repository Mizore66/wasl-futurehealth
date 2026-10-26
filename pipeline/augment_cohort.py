"""v7 cohort = v6-impact cohort (1,500 CRC + 600 cervical) + the v6-gov governance additions (same rules, seed 606).
v7 change: every added resource carries meta.source/versionId/lastUpdated so the mock Malaffi server can raise it as a topic event.

Original v6 doc: v6 cohort = v5 cohort (unchanged) + appended governance resources:
  - Consent (outreach opt-out) for 4% of referral patients, recorded by a coordinator/clinician at the referring provider
  - 12 forged Consent 'deny' by unauthorised roles (auditor / none) for patients who did NOT opt out  -> must be rejected
  - 12 forged Provenance sign-offs on a provider feed (compromised integration account):
      6 evidence 'adequate' on truly inadequate reports, signed as role 'reviewer' (identity reviewers may not label evidence)
      6 link 'same' on truly different identity pairs, signed as role 'coordinator'          -> must be rejected
Writes cohort/ (copied provider feeds + additions), consent_truth.json (loop -> opted out), forged.json."""
import json, random, shutil, os, hashlib, collections
SRC = '../v6-impact'  # v7: 1,500 CRC + 600 cervical cohort
OUT = 'cohort'
os.makedirs(OUT, exist_ok=True)
r = random.Random(606)
truth = json.load(open(f'{SRC}/cohort/truth.json'))
queue = json.load(open(f'{SRC}/run/main/review_queue.json'))
for f in ['truth.json', 'cohort_summary.json']:
    shutil.copy(f'{SRC}/cohort/{f}', f'{OUT}/{f}')
feeds = {}
srpk = {}  # loop -> (prov, mrn, patient resource, sr day)
for prov in ['ahs-mushrif', 'medlab', 'skmc-endo', 'noor-endo', 'corniche-wh']:
    rows = [json.loads(l) for l in open(f'{SRC}/cohort/fhir_{prov}.ndjson')]
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
for i, (prov, mrn) in enumerate(opt):
    loop0 = by_pk[(prov, mrn)][0]
    _, _, pt, arr = srpk[loop0]
    day = arr + r.uniform(0.01, 0.5)
    role = 'coordinator' if r.random() < 0.8 else 'clinician'
    cid = f'consent-{i}'
    add[prov].append({"arrival": day, "prov": prov, "msgid": f"{prov}/Consent/{cid}/v1", "res": consent(prov, mrn, pt, day, role, "deny", cid)})
    for l in by_pk[(prov, mrn)]:
        consent_truth[l] = True
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
json.dump(forged, open(f'{OUT}/forged.json', 'w'), indent=1)
print('referral pks', len(pks), 'opted out', len(opt), 'loops opted out', len(consent_truth), 'forged', len(forged),
      'added', {k: len(v) for k, v in add.items()})
