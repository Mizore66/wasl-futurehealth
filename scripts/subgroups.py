"""Review load and CRC recall by age band and by sex (DoH AI Policy 3.1.5.1: representativeness and bias).

The origin check (origin_null.py) covers name origin. This script does the same for two more attributes the cohort
carries through the person record: age band at the start of the run and recorded sex. Ages come from the Wikidata
persons' birth dates, so they are not a screening-age mix (some exceed 100); the check shows whether the pipeline treats
birth-year groups differently, not how it would treat a real screening population. Both are compared on CRC loops only:
the programmes carry very different review loads (lung about 10 items per 100 loops, CRC about 60), and lung loops are
mostly men while cervical loops are all women, so a mixed comparison measures the programme, not the person. Unknown sex
is left out.

Per group: loops, review items (one per item, as in report7), CRC loops truly closed, of those closed by Wasl, and CRC
false closures. A no-bias reference shuffles the group labels across loops (keeping each loop's item count), as in
origin_null.py. Per-loop item histograms are written so final_aggregate.py can pool the test over seeds.

usage: subgroups.py <cohort/truth.json> <run/reviewer_decisions.ndjson> <run/out_log.ndjson> <data/persons3.json> > subgroups.json
"""
import json, random, sys, collections

truth = json.load(open(sys.argv[1]))['loops']
persons = {p['qid']: p for p in json.load(open(sys.argv[4]))}
items, seen = collections.Counter(), set()
for line in open(sys.argv[2]):
    d = json.loads(line)
    if d['item'] in seen or d['loop'] not in truth:
        continue
    seen.add(d['item'])
    items[d['loop']] += 1
final = {}
for line in open(sys.argv[3]):
    dec = (json.loads(line).get('data') or {}).get('dec')
    if dec:
        final[dec['loop']] = dec['state']


def age_band(dob):
    a = 2026 - int(dob[:4])
    return "<40" if a < 40 else "40-59" if a < 60 else "60-79" if a < 80 else "80+"


groupings = {
    "age_band": lambda v, p: age_band(p['dob']) if v['prog'] == 'crc' else None,
    "sex": lambda v, p: p['sex'] if v['prog'] == 'crc' and p['sex'] in ('F', 'M') else None,
}
rng, out = random.Random(20261031), {}
for name, key in groupings.items():
    lab = {}
    for lid, v in truth.items():
        g = key(v, persons[v['qid']])
        if g is not None:
            lab[lid] = g
    ids = sorted(lab)
    groups = sorted(set(lab.values()))
    counts = [items[i] for i in ids]

    def ratio(labels):
        n, it = collections.Counter(labels), collections.Counter()
        for g, c in zip(labels, counts):
            it[g] += c
        r = [it[g] / n[g] for g in groups]
        return max(r) / min(r)

    obs, perm, null = ratio([lab[i] for i in ids]), [lab[i] for i in ids], []
    for _ in range(1000):
        rng.shuffle(perm)
        null.append(ratio(perm))
    null.sort()
    per = {}
    for g in groups:
        L = [i for i in ids if lab[i] == g]
        crc = [i for i in L if truth[i]['prog'] == 'crc']
        closed = [i for i in crc if truth[i]['state'] == 'closed']
        per[g] = {"loops": len(L), "items": sum(items[i] for i in L),
                  "items_per_loop_hist": {str(k): c for k, c in sorted(collections.Counter(items[i] for i in L).items())},
                  "crc_truly_closed": len(closed), "crc_closed_by_wasl": sum(final.get(i) == 'closed' for i in closed),
                  "crc_false_closures": sum(final.get(i) == 'closed' and truth[i]['state'] != 'closed' for i in crc)}
    out[name] = {"observed_ratio": round(obs, 4), "null_median": round(null[500], 4), "null_p95": round(null[950], 4),
                 "p_value": round(sum(x >= obs for x in null) / 1000, 4), "groups": per}
json.dump(out, sys.stdout, indent=1)
print()
