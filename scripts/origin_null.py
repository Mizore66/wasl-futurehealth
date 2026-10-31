"""Review load by name origin against a no-bias reference.

The headline subgroup metric is max/min over the four name origins of review items per 100 loops. With four groups of
unequal size (about 250 to 1,500 loops) this ratio sits above 1 even when origin has no effect at all. This script
shuffles the origin labels across loops (keeping each loop's item count), recomputes the ratio, and reports where the
observed value falls in that permutation distribution. It also writes the per-loop item-count histogram, so
final_aggregate.py can run the same test pooled over seeds (labels shuffled within each seed).

Programmes carry very different review loads (CRC about 60 items per 100 loops, DR about 13), and the loop mix differs
by origin, so the overall ratio partly measures the mix. Two programme-aware checks follow:
- standardised: per origin, observed items / items expected from each of its loops' programme mean (indirect
  standardisation); max/min of that ratio, against a no-bias reference that shuffles origin labels within each programme;
- by programme: the same max/min and shuffled-label test inside each programme.
Per-programme, per-origin item histograms are written so final_aggregate.py can pool both over seeds.

usage: origin_null.py <cohort/truth.json> <run/reviewer_decisions.ndjson> [permutations] > origin_null.json
"""
import json, random, sys, collections

truth = json.load(open(sys.argv[1]))['loops']
N = int(sys.argv[3]) if len(sys.argv) > 3 else 1000
items, seen = collections.Counter(), set()
for line in open(sys.argv[2]):
    d = json.loads(line)
    if d['item'] in seen or d['loop'] not in truth:  # one count per review item, as in report7
        continue
    seen.add(d['item'])
    items[d['loop']] += 1
ids = sorted(truth)
origins = [truth[i]['origin'] for i in ids]
counts = [items[i] for i in ids]
groups = sorted(set(origins))


def ratio(orig):
    loops, it = collections.Counter(orig), collections.Counter()
    for o, c in zip(orig, counts):
        it[o] += c
    rates = [it[o] / loops[o] for o in groups]
    return max(rates) / min(rates)


progs = [truth[i]['prog'] for i in ids]


def standardised(orig):
    pl, pi = collections.Counter(progs), collections.Counter()
    for pg, c in zip(progs, counts):
        pi[pg] += c
    obs_, exp_ = collections.Counter(), collections.Counter()
    for o, pg, c in zip(orig, progs, counts):
        obs_[o] += c
        exp_[o] += pi[pg] / pl[pg]
    r = [obs_[o] / exp_[o] for o in groups]
    return max(r) / min(r)


def shuffle_within(orig, rng):  # permute origin labels among the loops of each programme
    out = orig[:]
    for pg in set(progs):
        idx = [k for k, x in enumerate(progs) if x == pg]
        lab = [out[k] for k in idx]
        rng.shuffle(lab)
        for k, o in zip(idx, lab):
            out[k] = o
    return out


obs = ratio(origins)
rng, perm, null = random.Random(20261030), origins[:], []
for _ in range(N):
    rng.shuffle(perm)
    null.append(ratio(perm))
null.sort()
q = lambda p: null[min(N - 1, int(p * N))]
std_obs, rng2 = standardised(origins), random.Random(20261101)
std_null = sorted(standardised(shuffle_within(origins, rng2)) for _ in range(N))
by_prog = {}
for pg in sorted(set(progs)):
    oo = [o for o, x in zip(origins, progs) if x == pg]
    cc = [c for c, x in zip(counts, progs) if x == pg]
    G = sorted(set(oo))
    def rt(lab):
        L, I = collections.Counter(lab), collections.Counter()
        for o, c in zip(lab, cc):
            I[o] += c
        r = [I[o] / L[o] for o in G]
        return max(r) / min(r) if min(r) > 0 else float('inf')
    ob, lab, nl, r3 = rt(oo), oo[:], [], random.Random(20261102)
    for _ in range(N):
        r3.shuffle(lab)
        nl.append(rt(lab))
    by_prog[pg] = {"observed": round(ob, 4), "p_value": round(sum(x >= ob for x in nl) / N, 4),
                   "items_per_100": {o: round(100 * sum(c for x, c in zip(oo, cc) if x == o) / oo.count(o), 1) for o in G},
                   "hist_by_origin": {o: {str(k): v for k, v in sorted(collections.Counter(c for x, c in zip(oo, cc) if x == o).items())} for o in G}}
json.dump({"observed": round(obs, 4), "null_median": round(q(.5), 4), "null_p95": round(q(.95), 4),
           "p_value": round(sum(x >= obs for x in null) / N, 4), "permutations": N,
           "loops": dict(collections.Counter(origins)),
           "items": {o: sum(c for oo, c in zip(origins, counts) if oo == o) for o in groups},
           "items_per_loop_hist": {str(k): v for k, v in sorted(collections.Counter(counts).items())},  # for the pooled test
           "standardised": {"observed": round(std_obs, 4), "null_p95": round(std_null[int(.95 * N)], 4),
                            "p_value": round(sum(x >= std_obs for x in std_null) / N, 4)},
           "by_programme": by_prog},
          sys.stdout, indent=1)
print()
