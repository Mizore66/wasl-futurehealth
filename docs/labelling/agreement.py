"""Inter-rater agreement for the 43 report texts: Cohen's kappa per field between two labellers, plus each vs the current key.
Usage: python docs/labelling/agreement.py labels_A.csv labels_B.csv   ->  prints table, writes agreement.json"""
import csv, json, os, sys, collections
here = os.path.dirname(os.path.abspath(__file__))
key = {r['uid']: dict(r['key_fields'], state=r['key_state']) for r in json.load(open(os.path.join(here, '..', '..', 'data', 'evidence', 'reports43.json')))['reports']}
FIELDS = ['caecum', 'prep', 'polyps', 'histology', 'state']


def load(p):
    rows = {}
    for r in csv.DictReader(open(p)):
        rows[r['uid']] = {f: next(v for k, v in r.items() if k.split(' ')[0] == f).strip().lower() for f in FIELDS}
    return rows


def kappa(a, b):
    n = len(a)
    if n == 0:
        return None
    po = sum(x == y for x, y in zip(a, b)) / n
    ca, cb = collections.Counter(a), collections.Counter(b)
    pe = sum(ca[k] * cb[k] for k in set(a) | set(b)) / n / n
    return 1.0 if pe == 1 else round((po - pe) / (1 - pe), 3)


A, B = load(sys.argv[1]), load(sys.argv[2])
uids = sorted(u for u in A if u in B and all(A[u][f] and B[u][f] for f in FIELDS))
res = {"n_reports": len(uids), "fields": {}}
print(f"{len(uids)} reports labelled by both\n{'field':10} {'kappa A-B':>10} {'agree A-B':>10} {'kappa A-key':>12} {'kappa B-key':>12}")
for f in FIELDS:
    a, b, k = [A[u][f] for u in uids], [B[u][f] for u in uids], [key[u][f] for u in uids]
    row = {"kappa_ab": kappa(a, b), "agree_ab": round(sum(x == y for x, y in zip(a, b)) / max(len(uids), 1), 3),
           "kappa_a_key": kappa(a, k), "kappa_b_key": kappa(b, k),
           "disagreements": [u for u, x, y in zip(uids, a, b) if x != y]}
    res["fields"][f] = row
    print(f"{f:10} {row['kappa_ab']!s:>10} {row['agree_ab']!s:>10} {row['kappa_a_key']!s:>12} {row['kappa_b_key']!s:>12}")
json.dump(res, open(os.path.join(here, 'agreement.json'), 'w'), indent=1)
print("\nAdjudicate the listed disagreements together; the adjudicated sheet becomes the new key.")
