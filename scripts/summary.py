"""Print the headline numbers from a report7.json (what the demo/submission quotes)."""
import json, sys
r = json.load(open(sys.argv[1]))
c, k = r['closure'], r['crc_kpi_31d']
row = lambda d: f"P {d['precision']:.3f}  R {d['recall']:.3f}  false closures {d['false_closures']}/{d['closed_pred']}"
print("── Wasl baseline (simulated cohort; one seed) ──")
for prog in ('crc', 'cervical'):
    print(f"{prog:9s} with review : {row(c[prog]['wasl_live_with_review'])}")
    print(f"{prog:9s} no review   : {row(c[prog]['wasl_no_review'])}")
print(f"CRC ≤31-day KPI (all referrals): EID-only {k['eid_only']['kpi_all_referrals']:.3f} | truth {k['truth']['kpi_all_referrals']:.3f} | Wasl {k['wasl_live']['kpi_all_referrals']:.3f}")
cr = r['crash_recovery']; f = r['forged']; p = r['provenance']; ct = r['consent_timing']
print(f"exactly-once: duplicates {cr['duplicate_outputs']} / {cr['expected_outputs']} outputs; final-state mismatches {cr['final_state_mismatch_loops']}")
print(f"provenance chains verified {p['hash_verified']}/{p['closed_chains']}; forged events with effect {f['forged_with_any_effect']}/{f['forged_events']}")
print(f"wrong-patient messages released {sum(v['wrong_released'] for v in r['outreach_live']['by_origin'].values())}; "
      f"messages before late opt-out recorded {ct['released_before_opt_out_recorded']}")
if 'released_without_granted_consent' in ct:
    print(f"consent hold: released {ct['released_total']}, without granted consent {ct['released_without_granted_consent']}; "
          f"held-for-consent {ct['intents_ever_held_for_consent']}, cancelled {ct['intents_cancelled_by_consent']}")
print(f"throughput {r['throughput_ev_s']:.0f} ev/s; reviewer-h/1k loops {r['reviewer']['by_kind']['all']['reviewer_hours_per_1k_loops']:.1f}")
