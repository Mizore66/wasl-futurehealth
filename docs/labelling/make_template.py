"""Write a blind labelling sheet for the 43 report texts (no model/key answers shown). One copy per labeller.
Usage: python docs/labelling/make_template.py <labeller_name>  ->  docs/labelling/labels_<name>.csv"""
import csv, json, os, sys
here = os.path.dirname(os.path.abspath(__file__))
reps = json.load(open(os.path.join(here, '..', '..', 'data', 'evidence', 'reports43.json')))['reports']
out = os.path.join(here, f"labels_{sys.argv[1]}.csv")
with open(out, 'w', newline='') as f:
    w = csv.writer(f)
    w.writerow(['uid', 'text', 'caecum (yes|no|not_stated)', 'prep (adequate|inadequate|not_stated)', 'polyps (yes|no)',
                'histology (yes|no|pending)', 'state (closed|not_closed)', 'reason (free text)'])
    for r in reps:
        w.writerow([r['uid'], r['text'], '', '', '', '', '', ''])
print("wrote", out, len(reps), "rows. Label blind: do not look at the other labeller's sheet or at reports43.json key_* fields.")
