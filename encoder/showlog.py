"""Print a compact table of a train.py log (stdout file or log.json). Usage: python encoder/showlog.py LOG [LOG ...]"""
import json
import sys

for path in sys.argv[1:]:
    print(path)
    rows = []
    for line in open(path):
        line = line.strip()
        if line.startswith('{"epoch'):
            rows.append(json.loads(line))
        elif line.startswith("["):
            rows = json.load(open(path))
            break
    for r in rows:
        d = {k[4:]: round(v, 4) for k, v in r.items() if k.startswith("dev_")}
        print(r["epoch"], round(r["loss"], 3), r["sec"], d, round(r.get("score", 0), 4), "*" if r.get("saved") else "")
