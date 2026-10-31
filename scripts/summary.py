"""Print the headline numbers from a report7.json (what the demo/submission quotes)."""
import json, sys
r = json.load(open(sys.argv[1]))
c, k = r['closure'], r['crc_kpi_31d']
f3 = lambda x: "n/a" if x is None else f"{x:.3f}"  # null = undefined ratio (e.g. no closures without review in shadow mode)
row = lambda d: f"P {f3(d['precision'])}  R {f3(d['recall'])}  false closures {d['false_closures']}/{d['closed_pred']}"
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
lung = r.get('programmes_live', {}).get('lung_nodule')
if lung and 'closure' in lung:  # second loop type (WASL_LUNG=N)
    lc, rec, af = lung['closure'], lung['recommendation'], lung['appropriate_followup_within_window']
    print(f"lung_nodule (Fleischner 2017): P {lc['precision']:.3f}  R {lc['recall']:.3f}  false closures {lc['false_closures']}/{lc['pred_closed']}; "
          f"escalation recall {lung['escalation']['recall']:.3f}; overdue acc {lung['overdue_flag_accuracy']:.3f}; "
          f"recommendation acc {rec['accuracy']:.3f} (coverage {rec['coverage']:.3f}); appropriate follow-up truth {af['truth']:.3f} vs Wasl {af['wasl_measured']:.3f}")
dr = r.get('programmes_live', {}).get('diabetic_retinopathy')
if dr and 'closure' in dr:  # third loop type (WASL_DR=N)
    dc, rc, ex = dr['closure'], dr['referral_closure'], dr['referral_exam_within_deadline']
    print(f"diabetic_retinopathy (doh2024dr): P {dc['precision']:.3f}  R {dc['recall']:.3f}  false closures {dc['false_closures']}/{dc['pred_closed']}; "
          f"referral closures P {rc['precision']:.3f} R {rc['recall']:.3f}; triage acc {dr['triage']['accuracy']:.3f} (coverage {dr['triage']['coverage']:.3f}); "
          f"overdue acc {dr['overdue_flag_accuracy']:.3f}; exam within deadline truth {ex['truth']:.3f} vs Wasl {ex['wasl_measured']:.3f}")
pl = r.get('pilot')  # pilot switches (WASL_SHADOW / WASL_AUTOAUDIT); absent when off
if pl and 'shadow' in pl:
    s = pl['shadow']
    print(f"shadow mode: items {s['items_raised']}, signed {s['signed']}, confirmed {s['confirmed']}, rejected {s['rejected']} "
          f"(reader errors caught {s['rejected_reader_error']}, reviewer errors {s['rejected_reviewer_error']}); "
          f"extra reviewer-h {s['reviewer_hours']} ({s['reviewer_hours_per_1k_loops']}/1k loops); "
          f"{s['streak_target']} in a row reached on day {s['first_day_streak_reached']} (longest streak {s['longest_confirmation_streak']})")
if pl and 'autoaudit' in pl:
    s = pl['autoaudit']
    print(f"auto-audit p={pl['autoaudit_p']}: audits {s['items_raised']} of {s['evidence_automatic_closures_final']} automatic closures, signed {s['signed']}, "
          f"re-opened {s['reopened_loops']} (truly false {s['reopened_truly_false_closures']}); audited truly false {s['audited_truly_false_closures']}; "
          f"reviewer-h {s['reviewer_hours']}")
