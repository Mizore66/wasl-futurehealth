"""Review load by name origin against a no-bias reference.

The headline subgroup metric is max/min over the four name origins of review items per 100 loops. With four groups of
unequal size (about 250 to 1,500 loops) this ratio sits above 1 even when origin has no effect at all. This script
shuffles the origin labels across loops (keeping each loop's item count), recomputes the ratio, and reports where the
observed value falls in that permutation distribution. It also writes the per-loop item-count histogram, so
final_aggregate.py can run the same test pooled over seeds (labels shuffled within each seed).

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


obs = ratio(origins)
rng, perm, null = random.Random(20261030), origins[:], []
for _ in range(N):
    rng.shuffle(perm)
    null.append(ratio(perm))
null.sort()
q = lambda p: null[min(N - 1, int(p * N))]
json.dump({"observed": round(obs, 4), "null_median": round(q(.5), 4), "null_p95": round(q(.95), 4),
           "p_value": round(sum(x >= obs for x in null) / N, 4), "permutations": N,
           "loops": dict(collections.Counter(origins)),
           "items": {o: sum(c for oo, c in zip(origins, counts) if oo == o) for o in groups},
           "items_per_loop_hist": {str(k): v for k, v in sorted(collections.Counter(counts).items())}},  # for the pooled test
          sys.stdout, indent=1)
print()
