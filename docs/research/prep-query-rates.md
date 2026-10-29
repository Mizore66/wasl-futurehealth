# Ungraded bowel prep: how the simulated endoscopy unit answers

A colonoscopy report can record the caecum but no bowel-prep grade. Wasl then does not close the loop. It sends a query to the endoscopy-unit lead. After 5 working days it escalates to the programme coordinator. At day 15 a clinician decides. The code is in `pipeline/prepquery.go`.

No published study measures how endoscopy units answer a query about bowel prep after the event. We found none from NHS BCSP, JAG, or the Dutch or Canadian screening registries. So the rates below come from proxies. Each one is labelled with how well it transfers. **They are assumptions for the simulation, not measurements.**

## Evidence

| # | Value | Source | What was measured | Transfer |
|---|---|---|---|---|
| E1 | 69% of US CDI programmes get 91–100% of queries answered in their required time; the most common required time is 2 days | [ACDIS CDI Week survey 2023](https://acdis.org/sites/acdis/files/cdi-week/CR-7838%20CDI%20Week%20Industry%20Survey%202023_final.pdf), [2024 preview](https://acdis.org/articles/2024-cdi-week-qa-preview-query-metrics-and-technology) | Queries to the treating physician in the same hospital, usually mandatory | Weak; upper bound only |
| E2 | Response rate 64.7% with an escalation policy, 51.2% without | [ACDIS Q&A](https://acdis.org/articles/qa-%E2%80%98unable-determine%E2%80%99-query-responses-and-kpis) | As E1 | Size of the escalation effect |
| E3 | About 34% of programmes report "clinically undetermined" answers | ACDIS 2024 preview | As E1 | Answered but not usable |
| E4 | 80.5% of data queries resolved within 2 weeks; median 5.1 calendar days (IQR 1.9–8.9) | [Ghanim et al.](https://pmc.ncbi.nlm.nih.gov/articles/PMC13354148/) | Queries about missing fields to 4 community oncology sites | Best proxy for answer rate and delay |
| E5 | Median 23 days to resolve; 21% needed more than one round | [Tolmie et al., Trials 2011](https://pmc.ncbi.nlm.nih.gov/articles/PMC3115878/) | 782 trial source-data queries | Long tail under weak escalation |
| E6 | 35% response | [Cunningham et al. 2015](https://link.springer.com/article/10.1186/s12874-015-0016-z) | Voluntary survey of specialists | Floor for requests that are not mandatory |
| E7 | Prep grade missing in 65.5% of reports | [Ontario audit](https://pmc.ncbi.nlm.nih.gov/articles/PMC4992524/) | 557 reports | How often the query is triggered |
| E8 | Missing in 27% (Maryland) and 13.9% (CORI, ~430k reports) | [Li et al. 2010](https://pmc.ncbi.nlm.nih.gov/articles/PMC2948883/) | As stated | How often the query is triggered |
| E9 | Missing in 1.4% (Portugal) and 3% (UCLA); an audit raised documentation from 59.8% to 98.6% | [UCLA audit](https://www.uclahealth.org/sites/default/files/documents/5f/tamairobert.pdf?f=f005edf1), [GIE abstract](https://www.giejournal.org/article/S0016-5107(11)01111-4/pdf) | Single centres | How often the query is triggered |
| E10 | BBPS scored from recorded video: ICC 0.74; split into adequate/inadequate, kappa 0.25–0.48 | [Lai et al. 2009](https://pmc.ncbi.nlm.nih.gov/articles/PMC2763922/) and video-reliability studies | Scoring after the event | A late grade is unreliable without good images |

## Simulation settings

| Setting | Value | Range to test | Basis |
|---|---|---|---|
| Unit answers within 5 working days | 55% | 35–80% | E4, discounted because the request is not mandatory and goes to another organisation (E1, E6) |
| Unit answers only after the coordinator escalation, before day 15 | 20% | 10–30% | Assumption, sized by E2 and E4 |
| No answer by day 15 | 25% | 10–45% | Remainder; E5 shows a heavy tail |
| Delay to answer | lognormal, median about 4 working days | median 2–8 | E4 |
| Answer is a usable grade (else "cannot determine") | 50% | 25–80% | **Assumption.** E10 and E3 |
| Per-read error of the day-15 clinician decision | 25% | — | **Assumption.** The clinician decides without a grade. This is a stress-test rate, not evidence that clinicians make errors. |

With these settings about 75% of queries get an answer by day 15, about 37% get a usable grade, and about 63% go to the day-15 clinician decision.

How often a report has no grade is not a setting. It comes from the report corpus (MTSamples and the synthetic corpora). E7–E9 show the real rate can be anywhere from 1% to 65%, depending on the unit.
