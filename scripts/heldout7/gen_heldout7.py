#!/usr/bin/env python3
"""Deterministic generator for the heldout7 synthetic colonoscopy corpus."""

import hashlib
import json
import os
import random
from collections import Counter
from datetime import date, timedelta

SEED = 7079
OUTPUT_NAME = "reports_heldout7.json"
SOURCE = (
    "Seventh independent author (Kimi K3.1 Pro, Moonshot AI) - deterministic "
    "stdlib generator gen_heldout7.py, random seed 7079, 300 synthetic Abu "
    "Dhabi colonoscopy reports, 2026-10-29"
)

LAYOUTS = [
    "structured",
    "narrative",
    "impression_first",
    "tabulated",
    "discharge",
    "dictated",
    "compact",
    "clinical_letter",
]

FACILITIES = [
    "Corniche Day Surgery Unit",
    "Al Reem Endoscopy Suite",
    "Khalifa District Day Case Centre",
    "Al Ain Road Hospital Short Stay Unit",
    "Mussafah Ambulatory Surgery Centre",
    "Yas Island Day Procedure Unit",
    "Al Bateen Hospital Endoscopy Unit",
    "Saadiyat Day Surgery Centre",
]


def truth_from_reality(reality):
    if not reality["caecum"] or not reality["prep"]:
        return "inadequate"
    if reality["tissue"] and not reality["histology"]:
        return "awaiting_histology"
    return "adequate"


def derive_key_state(key):
    if key["caecum"] == "no":
        return "not_closed", "incomplete_exam"
    if key["prep"] == "inadequate":
        return "not_closed", "inadequate_prep"
    if key["caecum"] == "not_stated" or key["prep"] == "not_stated":
        return "needs_human", "missing_documentation"
    if key["tissue_taken"] == "yes" and key["histology"] == "no":
        return "not_closed", "awaiting_histology"
    return "closed", "criteria_met"


def yes_no(value):
    return "yes" if value else "no"


def make_specs(rng):
    truths = (
        ["adequate"] * 120
        + ["awaiting_histology"] * 90
        + ["inadequate"] * 90
    )
    rng.shuffle(truths)

    pools = {
        "adequate": ["none"] * 60 + ["polyp"] * 40 + ["biopsy"] * 20,
        "awaiting_histology": ["polyp"] * 60 + ["biopsy"] * 30,
        "inadequate": ["none"] * 45 + ["polyp"] * 25 + ["biopsy"] * 20,
    }
    for pool in pools.values():
        rng.shuffle(pool)
    pool_pos = {name: 0 for name in pools}

    specs = []
    inadequate_number = 0

    for n, planned_truth in enumerate(truths, 1):
        mode = pools[planned_truth][pool_pos[planned_truth]]
        pool_pos[planned_truth] += 1

        caecum = True
        prep = True
        if planned_truth == "inadequate":
            failure_kind = inadequate_number % 3
            if failure_kind == 0:
                caecum = False
            elif failure_kind == 1:
                prep = False
            else:
                caecum = False
                prep = False
            inadequate_number += 1

        tissue = mode != "none"
        if planned_truth == "adequate":
            histology = tissue
        elif planned_truth == "awaiting_histology":
            histology = False
        else:
            histology = tissue and ((n + inadequate_number) % 4 == 0)

        reality = {
            "caecum": caecum,
            "prep": prep,
            "tissue": tissue,
            "histology": histology,
        }
        assert truth_from_reality(reality) == planned_truth

        tissue_word = yes_no(tissue)
        key = {
            "caecum": "yes" if caecum else "no",
            "prep": "adequate" if prep else "inadequate",
            "polyps": tissue_word,
            "tissue_taken": tissue_word,
            "histology": yes_no(histology),
        }

        specs.append(
            {
                "n": n,
                "uid": "u%03d" % n,
                "truth": planned_truth,
                "reality": reality,
                "key": key,
                "tissue_mode": mode,
                "finding_kind": mode,
                "prep_mode": "verbal",
                "extent_mode": "direct",
                "histology_mode": "routine",
                "boston_scores": None,
                "add_type": None,
                "prior_type": None,
                "features": set(),
                "tags": set(),
                "spelling": "uk" if n % 2 else "us",
                "layout": LAYOUTS[(n - 1) % len(LAYOUTS)],
                "arabic": n % 9 == 0,
                "ocr": n % 12 == 0,
            }
        )
    return specs


def choose(specs, count, predicate, reserved):
    chosen = []
    for spec in specs:
        if spec["n"] in reserved or not predicate(spec):
            continue
        chosen.append(spec)
        if len(chosen) == count:
            break
    if len(chosen) != count:
        raise AssertionError("Unable to fill deterministic feature quota")
    reserved.update(spec["n"] for spec in chosen)
    return chosen


def add_hard(spec, name):
    spec["features"].add(name)
    spec["tags"].add("hard:" + name)


def assign_features(specs):
    # Reserve corrections first so each corrected field remains unambiguous.
    add_reserved = set()
    groups = [
        (
            3,
            lambda s: s["truth"] == "inadequate" and not s["reality"]["caecum"],
            "extent_complete_to_incomplete",
        ),
        (
            3,
            lambda s: s["truth"] in ("adequate", "awaiting_histology"),
            "extent_incomplete_to_complete",
        ),
        (
            3,
            lambda s: s["truth"] == "inadequate" and not s["reality"]["prep"],
            "prep_adequate_to_inadequate",
        ),
        (
            3,
            lambda s: s["truth"] in ("adequate", "awaiting_histology"),
            "prep_inadequate_to_adequate",
        ),
        (
            4,
            lambda s: s["truth"] == "adequate" and s["reality"]["tissue"],
            "histology_pending_to_result",
        ),
    ]
    for count, predicate, add_type in groups:
        for spec in choose(specs, count, predicate, add_reserved):
            spec["add_type"] = add_type
            add_hard(spec, "addendum_correction")

    # Fifty ungraded preparation reports; six conceal an actual prep failure.
    prep_missing_reserved = set()
    missing_prep = choose(
        specs,
        6,
        lambda s: not s["add_type"] and not s["reality"]["prep"],
        prep_missing_reserved,
    )
    missing_prep += choose(
        specs,
        44,
        lambda s: not s["add_type"] and s["reality"]["prep"],
        prep_missing_reserved,
    )
    for spec in missing_prep:
        spec["prep_mode"] = "missing"
        spec["key"]["prep"] = "not_stated"
        add_hard(spec, "missing_documentation")
        if not spec["reality"]["prep"]:
            spec["tags"].add("hidden:undocumented_failure")

    # Thirty-seven unstated extents, disjoint from the ungraded-prep set.
    extent_missing_reserved = set()
    missing_extent = choose(
        specs,
        6,
        lambda s: (
            not s["add_type"]
            and s["n"] not in prep_missing_reserved
            and not s["reality"]["caecum"]
        ),
        extent_missing_reserved,
    )
    missing_extent += choose(
        specs,
        31,
        lambda s: (
            not s["add_type"]
            and s["n"] not in prep_missing_reserved
            and s["reality"]["caecum"]
        ),
        extent_missing_reserved,
    )
    for spec in missing_extent:
        spec["extent_mode"] = "missing"
        spec["key"]["caecum"] = "not_stated"
        add_hard(spec, "missing_documentation")
        if not spec["reality"]["caecum"]:
            spec["tags"].add("hidden:undocumented_failure")

    # Forty-eight Boston scores; twelve have totals of at least six but a bad segment.
    boston_reserved = set()
    boston_bad = choose(
        specs,
        12,
        lambda s: (
            not s["add_type"]
            and s["n"] not in prep_missing_reserved
            and not s["reality"]["prep"]
        ),
        boston_reserved,
    )
    boston_good = choose(
        specs,
        36,
        lambda s: (
            not s["add_type"]
            and s["n"] not in prep_missing_reserved
            and s["reality"]["prep"]
        ),
        boston_reserved,
    )
    bad_scores = [(3, 3, 1), (3, 2, 1), (2, 3, 1), (3, 3, 0)]
    good_scores = [(2, 2, 2), (3, 2, 2), (2, 3, 3), (3, 3, 3), (2, 2, 3)]
    for j, spec in enumerate(boston_bad):
        spec["prep_mode"] = "boston"
        spec["boston_scores"] = bad_scores[j % len(bad_scores)]
        add_hard(spec, "bbps_total_trap")
    for j, spec in enumerate(boston_good):
        spec["prep_mode"] = "boston"
        spec["boston_scores"] = good_scores[j % len(good_scores)]

    prep_reserved = set()
    for spec in choose(
        specs,
        12,
        lambda s: (
            not s["add_type"]
            and s["prep_mode"] == "verbal"
            and s["reality"]["prep"]
        ),
        prep_reserved,
    ):
        spec["prep_mode"] = "rescue"
        add_hard(spec, "prep_rescue")
    for spec in choose(
        specs,
        10,
        lambda s: (
            not s["add_type"]
            and s["prep_mode"] == "verbal"
            and not s["reality"]["prep"]
        ),
        prep_reserved,
    ):
        spec["prep_mode"] = "segment"
        add_hard(spec, "prep_segment_overrides_overall")

    # Three alternatives to a stock caecal-completion sentence.
    extent_reserved = set()
    extent_groups = [
        (18, "landmark", "extent_by_landmarks"),
        (12, "ti", "ti_implies_caecum"),
        (12, "withdrawal", "withdrawal_phrase"),
    ]
    for count, mode, feature in extent_groups:
        selected = choose(
            specs,
            count,
            lambda s: (
                not s["add_type"]
                and s["extent_mode"] == "direct"
                and s["key"]["caecum"] == "yes"
            ),
            extent_reserved,
        )
        for spec in selected:
            spec["extent_mode"] = mode
            add_hard(spec, feature)

    # Tissue-related families.
    biopsy_reserved = set()
    for spec in choose(
        specs,
        36,
        lambda s: not s["add_type"] and s["tissue_mode"] == "biopsy",
        biopsy_reserved,
    ):
        add_hard(spec, "non_polyp_biopsy")
    for spec in choose(
        specs,
        12,
        lambda s: not s["add_type"] and s["tissue_mode"] == "biopsy",
        biopsy_reserved,
    ):
        spec["finding_kind"] = "no_polyps_but_biopsied"
        add_hard(spec, "no_polyps_but_biopsied")

    pending_reserved = set()
    for spec in choose(
        specs,
        8,
        lambda s: (
            not s["add_type"]
            and s["truth"] == "awaiting_histology"
            and s["reality"]["tissue"]
        ),
        pending_reserved,
    ):
        spec["histology_mode"] = "specimen_lost"
        add_hard(spec, "specimen_no_result")
    for spec in choose(
        specs,
        14,
        lambda s: (
            not s["add_type"]
            and s["truth"] == "awaiting_histology"
            and s["reality"]["tissue"]
        ),
        pending_reserved,
    ):
        spec["histology_mode"] = "request"
        add_hard(spec, "histology_request_not_result")

    none_reserved = set()
    for spec in choose(
        specs,
        28,
        lambda s: not s["add_type"] and s["tissue_mode"] == "none",
        none_reserved,
    ):
        add_hard(spec, "negated_tissue")
    for spec in choose(
        specs,
        12,
        lambda s: not s["add_type"] and s["tissue_mode"] == "none",
        none_reserved,
    ):
        spec["finding_kind"] = "polyp_not_removed"
        add_hard(spec, "polyp_seen_not_removed")
    for spec in choose(
        specs,
        10,
        lambda s: not s["add_type"] and s["tissue_mode"] == "none",
        none_reserved,
    ):
        spec["finding_kind"] = "planned_not_done"
        add_hard(spec, "planned_not_done")

    # Prior procedures are explicitly separated from the current procedure.
    prior_reserved = set()
    prior_groups = [
        (
            8,
            lambda s: not s["add_type"] and s["key"]["histology"] == "no",
            "histology",
            "prior_histology_not_current",
        ),
        (8, lambda s: not s["add_type"], "incomplete", "prior_incomplete_not_current"),
        (8, lambda s: not s["add_type"], "polyp", None),
    ]
    for count, predicate, prior_type, extra_feature in prior_groups:
        for spec in choose(specs, count, predicate, prior_reserved):
            spec["prior_type"] = prior_type
            add_hard(spec, "prior_procedure")
            if extra_feature:
                add_hard(spec, extra_feature)

    # The body defers to the impression, which alone records the failed extent.
    impression_reserved = set()
    for spec in choose(
        specs,
        12,
        lambda s: (
            not s["add_type"]
            and s["extent_mode"] == "direct"
            and s["key"]["caecum"] == "no"
        ),
        impression_reserved,
    ):
        spec["extent_mode"] = "impression_only"
        add_hard(spec, "impression_only_failure")

    # Required format, spelling, preparation, and tissue tags.
    for spec in specs:
        n = spec["n"]
        spec["tags"].add("fmt:" + spec["layout"])
        spec["tags"].add("spelling:" + spec["spelling"])
        if spec["add_type"]:
            spec["tags"].add("fmt:addendum")
        if spec["arabic"]:
            spec["tags"].add("fmt:arabic_bilingual")
        if spec["ocr"]:
            spec["tags"].add("fmt:ocr_scan")
            add_hard(spec, "ocr_noise")

        if spec["prep_mode"] == "missing":
            spec["tags"].add("prep:ungraded")
        elif spec["prep_mode"] == "boston":
            spec["tags"].add("prep:boston")
        else:
            spec["tags"].add("prep:verbal")

        if spec["finding_kind"] == "polyp_not_removed":
            spec["tags"].add("tissue:polyp_not_removed")
        elif spec["finding_kind"] == "planned_not_done":
            spec["tags"].add("tissue:planned_not_done")
        elif spec["tissue_mode"] == "none":
            spec["tags"].add("tissue:none")
        elif spec["tissue_mode"] == "polyp":
            spec["tags"].add("tissue:polypectomy")
        else:
            spec["tags"].add("tissue:biopsy")


def pick(spec, options, salt=0):
    return options[(spec["n"] * 7 + salt) % len(options)]


def caecal_terms(spec):
    if spec["spelling"] == "uk":
        return "caecum", "caecal", "ileocaecal", "faecal", "visualised", "diarrhoea"
    return "cecum", "cecal", "ileocecal", "fecal", "visualized", "diarrhea"


def extent_text(spec, key):
    value = key["caecum"]
    caecum, caecal, ileocaecal, _, visualised, _ = caecal_terms(spec)

    if value == "not_stated":
        return pick(
            spec,
            [
                "Proximal extent is not recorded in the available procedure note.",
                "Extent of insertion: no explicit entry; no completion statement can be verified.",
                "The report does not state the furthest point reached by the colonoscope.",
            ],
            2,
        )

    if value == "no":
        if spec["extent_mode"] == "impression_only":
            return pick(
                spec,
                [
                    "Technical extent is not separately dictated here; see the final impression.",
                    "The proximal limit is documented only in the concluding assessment.",
                ],
                3,
            )
        return pick(
            spec,
            [
                "The %s was not reached; a fixed sigmoid loop prevented further advancement." % caecum,
                "Examination stopped at the descending colon because of discomfort and looping; %s intubation was unsuccessful." % caecal,
                "The instrument could not be passed beyond the hepatic flexure, so the %s was not examined." % caecum,
                "Progress was halted at an angulated sigmoid. The procedure is incomplete short of the %s." % caecum,
            ],
            4,
        )

    if spec["extent_mode"] == "landmark":
        return pick(
            spec,
            [
                "At the proximal limit the %s valve and appendiceal orifice were identified and photographed." % ileocaecal,
                "Proximal landmark photographs show the appendiceal orifice beside the %s valve." % ileocaecal,
                "The %s valve, appendiceal opening and blind-ending pole were all seen." % ileocaecal,
            ],
            5,
        )
    if spec["extent_mode"] == "ti":
        return pick(
            spec,
            [
                "The endoscope passed through the %s valve into terminal ileum, where villi were seen." % ileocaecal,
                "Terminal ileal intubation was achieved for 6 cm; ileal mucosa was %s." % visualised,
                "A short terminal ileal examination was performed after entering through the %s valve." % ileocaecal,
            ],
            6,
        )
    if spec["extent_mode"] == "withdrawal":
        return pick(
            spec,
            [
                "Withdrawal began at the %s pole after landmark photographs had been taken." % caecal,
                "The timer was started on withdrawal from the %s pole." % caecal,
                "After photo-documentation of the %s pole, a careful withdrawal examination was performed." % caecal,
            ],
            7,
        )
    return pick(
        spec,
        [
            "The colonoscope was advanced to the %s; the appendiceal orifice and %s valve were seen." % (caecum, ileocaecal),
            "%s intubation was successful, with landmarks confirmed on still images." % caecal.capitalize(),
            "Complete insertion to the %s was achieved and followed by a slow withdrawal." % caecum,
            "The proximal limit was the %s, confirmed by two standard landmarks." % caecum,
        ],
        8,
    )


def prep_text(spec, key):
    value = key["prep"]
    _, _, _, faecal, visualised, _ = caecal_terms(spec)

    if spec["prep_mode"] == "missing":
        return pick(
            spec,
            [
                "No preparation grade, descriptive quality assessment or segmental score is documented.",
                "Bowel-preparation quality: not recorded in the available report.",
                "The preparation field is blank; no Boston score or verbal grade was entered.",
            ],
            10,
        )

    if spec["prep_mode"] == "boston":
        right, transverse, left = spec["boston_scores"]
        total = right + transverse + left
        forms = [
            "BBPS %d/%d/%d (right/transverse/left), total %d." % (right, transverse, left, total),
            "Boston score: total %d = %d+%d+%d (right+transverse+left)." % (total, right, transverse, left),
            "Right %d; transverse %d; left %d. BBPS total %d." % (right, transverse, left, total),
            "Boston Bowel Preparation Scale - R%d T%d L%d; sum %d." % (right, transverse, left, total),
            "BBPS(total=%d; segments %d, %d, %d)." % (total, right, transverse, left),
            "Segmental scores [right %d, transverse %d, left %d], Boston %d/9." % (right, transverse, left, total),
        ]
        line = forms[(spec["n"] + total) % len(forms)]
        if min(right, transverse, left) < 2:
            line += " At least one segment scored below 2, so preparation is inadequate."
        else:
            line += " Every segment scored at least 2."
        return line

    if spec["prep_mode"] == "rescue":
        return pick(
            spec,
            [
                "Initially residual stool obscured the right colon. After copious washing and suction the mucosa was clearly seen; final preparation was good.",
                "The pre-wash view was poor, but irrigation cleared the residual fluid. Final quality was adequate in every segment.",
                "Substantial washing was required. On the final assessment, preparation was fair and all mucosa could be inspected.",
            ],
            11,
        )

    if spec["prep_mode"] == "segment":
        return pick(
            spec,
            [
                "Overall preparation was called fair, but the right colon remained poor with adherent stool; the left colon was good.",
                "Although much of the colon was clean, solid %s residue persisted in the ascending colon. Preparation was inadequate for that segment." % faecal,
                "The left and transverse colon were well prepared; the right colon was poorly prepared and could not be reliably %s." % visualised,
            ],
            12,
        )

    if value == "adequate":
        return pick(
            spec,
            [
                "Bowel preparation was good, with clear effluent after washing.",
                "Preparation quality was fair and permitted adequate mucosal inspection.",
                "Excellent preparation; only small bubbles were present.",
                "Preparation was adequate throughout, with no segment requiring repeat examination for cleanliness.",
            ],
            13,
        )
    return pick(
        spec,
        [
            "Bowel preparation was poor; solid stool prevented reliable mucosal assessment.",
            "Preparation was inadequate because of extensive %s residue." % faecal,
            "Poor preparation with adherent stool, particularly in the right colon.",
            "The preparation did not permit a satisfactory examination despite washing and suction.",
        ],
        14,
    )


def polyp_details(spec):
    sites = ["sigmoid colon", "ascending colon", "transverse colon", "descending colon", "rectum"]
    site = sites[spec["n"] % len(sites)]
    size = 3 + (spec["n"] * 7) % 15
    technique = "cold snare" if size <= 9 else "hot snare"
    return site, size, technique


def findings_text(spec):
    kind = spec["finding_kind"]
    _, _, _, _, visualised, diarrhoea = caecal_terms(spec)

    if kind == "polyp_not_removed":
        site, _, _ = polyp_details(spec)
        return pick(
            spec,
            [
                "A 4 mm sessile polyp was seen in the %s but was left in situ; the attending plan is surveillance at a dedicated list." % site,
                "A small polyp was %s in the %s. It was neither excised nor sampled during this examination." % (visualised, site),
                "One diminutive polyp was photographed in the %s and deliberately left untreated pending review of the referral plan." % site,
            ],
            20,
        )

    if kind == "planned_not_done":
        return pick(
            spec,
            [
                "The indication was chronic %s. Random biopsies were considered before withdrawal, but none were taken because the list overran and sampling was deferred." % diarrhoea,
                "Biopsy to exclude microscopic colitis was discussed in the plan; no forceps were passed and the mucosa appeared normal.",
                "A possible rectal biopsy was considered, then abandoned after review of the anticoagulation record. No sampling occurred.",
            ],
            21,
        )

    if kind == "no_polyps_but_biopsied":
        return pick(
            spec,
            [
                "No polyps were seen. For chronic %s, separate right- and left-colon biopsies were obtained." % diarrhoea,
                "There were no polypoid lesions; four random mucosal biopsies were nevertheless taken to assess microscopic inflammation.",
                "No adenoma or mass was identified. Biopsies from the caecal and rectal mucosa were taken for the diarrhoea indication.",
            ],
            22,
        )

    if kind == "polyp":
        site, size, technique = polyp_details(spec)
        return pick(
            spec,
            [
                "A %d mm sessile polyp in the %s was resected by %s and retrieved." % (size, site, technique),
                "One %d mm polyp was found in the %s. Complete %s polypectomy was performed without immediate bleeding." % (size, site, technique),
                "A %d mm lesion in the %s was lifted where required and removed with a %s; the site was inspected." % (size, site, technique),
            ],
            23,
        )

    if kind == "biopsy":
        return pick(
            spec,
            [
                "Patchy erythema and granularity were present in the sigmoid colon; two biopsies were taken. No polyp was seen.",
                "The mucosa looked normal, but random right- and left-colon biopsies were obtained for chronic %s." % diarrhoea,
                "A small rectal ulcer was biopsied twice to exclude inflammation; there were no polypectomies.",
                "Non-specific caecal erythema was sampled with biopsy forceps. The remaining mucosa was unremarkable.",
            ],
            24,
        )

    return pick(
        spec,
        [
            "No polyps, mass lesions or bleeding source were identified.",
            "The examined mucosa was unremarkable apart from small internal haemorrhoids.",
            "No adenomas or ulcers were seen; diverticulosis was present in the sigmoid colon.",
            "Inspection on withdrawal showed no significant abnormality.",
        ],
        25,
    )


def tissue_text(spec, key):
    if key["tissue_taken"] == "no":
        if spec["finding_kind"] == "polyp_not_removed":
            return "The visualised polyp was not excised or biopsied; no specimen was obtained."
        if spec["finding_kind"] == "planned_not_done":
            return "The considered sampling was not performed: no biopsy forceps or snare was used."
        if "negated_tissue" in spec["features"]:
            return pick(
                spec,
                [
                    "Neither polypectomy nor biopsy was undertaken.",
                    "No tissue was removed, and no specimens were submitted.",
                    "There were no biopsies and no polyp resections during this procedure.",
                    "Specimen count: zero; sampling and polypectomy were both avoided.",
                ],
                30,
            )
        return pick(
            spec,
            [
                "Specimens: none.",
                "No specimen was collected during the examination.",
                "Tissue acquisition: none.",
            ],
            31,
        )

    if spec["tissue_mode"] == "polyp":
        site, _, technique = polyp_details(spec)
        return "Tissue obtained at this procedure: %s polyp, %s polypectomy, retrieved for pathology." % (site, technique)
    return "Tissue obtained at this procedure: colonic mucosal biopsies, placed in labelled specimen pots."


def current_histology_result(spec):
    if spec["tissue_mode"] == "polyp":
        return pick(
            spec,
            [
                "tubular adenoma with low-grade dysplasia",
                "hyperplastic polyp, no dysplasia",
                "tubulovillous adenoma with low-grade dysplasia",
                "sessile serrated lesion without dysplasia",
            ],
            35,
        )
    return pick(
        spec,
        [
            "mild chronic non-specific inflammation, with no dysplasia",
            "normal colonic mucosa with no significant pathological abnormality",
            "chronic inactive colitis; no granulomas or dysplasia identified",
        ],
        36,
    )


def histology_text(spec, key):
    if key["tissue_taken"] == "no":
        return "No histology specimen exists for the present procedure."

    if key["histology"] == "yes":
        return "Histology result for the tissue taken at this procedure: %s." % current_histology_result(spec)

    if spec["histology_mode"] == "specimen_lost":
        return pick(
            spec,
            [
                "The specimen was obtained but later lost in transit. No histological result is available for this procedure.",
                "Although tissue was retrieved, the specimen pot could not be traced; consequently no pathology result exists.",
                "The sample was not received by the laboratory. There is no current histology report to quote.",
            ],
            40,
        )

    if spec["histology_mode"] == "request":
        return pick(
            spec,
            [
                "A pathology request was raised and the specimen dispatched. The request is not a result; no report has been received.",
                "Histopathology request completed. At the time of signing, the laboratory result section remains empty.",
                "Specimen submitted for analysis; only the request number is available, not a histological diagnosis.",
            ],
            41,
        )

    return pick(
        spec,
        [
            "Histology is awaited.",
            "The specimen was sent for histopathology; the result is not available in this report.",
            "Pathology pending at the time of discharge.",
            "No current histology result has yet been appended.",
        ],
        42,
    )


def history_text(spec):
    if spec["prior_type"] == "histology":
        return pick(
            spec,
            [
                "Past endoscopy, not the present procedure: colonoscopy in 2021 produced a tubular adenoma on histology. That old result must not be assigned to today's specimens.",
                "Previous histology from a 2022 polypectomy showed low-grade dysplasia; it is reproduced here only as surveillance history.",
                "An earlier report records a hyperplastic polyp in 2020. No current diagnosis is inferred from that historical result.",
            ],
            45,
        )
    if spec["prior_type"] == "incomplete":
        return pick(
            spec,
            [
                "History: a colonoscopy in 2019 was incomplete because of looping. The present examination is a repeat and has its own extent entry above.",
                "The referral mentions poor preparation and an aborted procedure in 2023; those findings concern the earlier attempt only.",
                "Prior note: caecal intubation failed in 2021 at another unit. This historical failure does not describe the current extent.",
            ],
            46,
        )
    if spec["prior_type"] == "polyp":
        return pick(
            spec,
            [
                "Surveillance history: two sigmoid polyps were removed during a colonoscopy in 2018. No earlier specimen is being reported as current tissue.",
                "The patient attended after a polypectomy at another hospital in 2022; the old procedure is background only.",
                "Previous colonoscopy in 2020 found diverticulosis and a small adenoma. Today's findings are documented separately.",
            ],
            47,
        )
    return "Referral history reviewed; no earlier endoscopy result is imported into the current findings."


def impression_text(spec, key):
    _, reason = derive_key_state(key)
    caecum, caecal, _, _, _, _ = caecal_terms(spec)

    if reason == "incomplete_exam":
        if spec["extent_mode"] == "impression_only":
            return pick(
                spec,
                [
                    "Incomplete examination: the %s was not reached because of a fixed sigmoid loop." % caecum,
                    "Final assessment: insertion ended proximal to the sigmoid only in part; specifically, the %s was not intubated." % caecum,
                    "The procedure was incomplete, with failure to reach the %s after prolonged looping." % caecum,
                ],
                50,
            )
        return "Incomplete colonoscopy: the %s was not reached." % caecum
    if reason == "inadequate_prep":
        return "Preparation is inadequate; this is not a reliable complete mucosal assessment even if %s intubation was documented." % caecal
    if reason == "missing_documentation":
        return "Extent and/or preparation documentation is insufficient for closure; manual review of the source record is required."
    if reason == "awaiting_histology":
        return "Tissue was obtained during this procedure; the current histology result remains outstanding."
    if key["tissue_taken"] == "yes":
        return "Complete examination with adequate preparation; the result for the current specimen is included."
    return "Complete examination with adequate preparation; no current histology is outstanding."


def make_statements(spec, key):
    return {
        "prep": prep_text(spec, key),
        "extent": extent_text(spec, key),
        "findings": findings_text(spec),
        "tissue": tissue_text(spec, key),
        "histology": histology_text(spec, key),
        "history": history_text(spec),
        "impression": impression_text(spec, key),
    }


def common_metadata(spec):
    n = spec["n"]
    procedure_date = date(2026, 1, 1) + timedelta(days=(n * 11) % 270)
    facility = FACILITIES[(n - 1) % len(FACILITIES)]
    age = 19 + (n * 17) % 67
    sex = "F" if n % 2 else "M"
    doctor = ["Dr A. Rahman", "Dr L. Mathew", "Dr S. Haddad", "Dr N. Okafor"][n % 4]
    _, _, _, _, _, diarrhoea = caecal_terms(spec)
    indications = [
        "surveillance after previous adenoma",
        "iron-deficiency anaemia",
        "change in bowel habit",
        "chronic " + diarrhoea,
        "positive faecal immunochemical test",
        "rectal bleeding",
    ]
    indication = indications[n % len(indications)]
    sedation = ["fentanyl and midazolam", "propofol sedation", "conscious sedation", "no intravenous sedation"][n % 4]
    return facility, procedure_date.isoformat(), age, sex, doctor, indication, sedation


def render_layout(spec, statements):
    facility, pdate, age, sex, doctor, indication, sedation = common_metadata(spec)
    uid = spec["uid"]
    layout = spec["layout"]

    if layout == "structured":
        return """SYNTHETIC COLONOSCOPY REPORT | {facility}
Report ID: H7-{uid} | MRN: SY7-{number:04d}
Procedure date: {pdate} | Age/sex: {age}/{sex}
INDICATION: {indication}.
SEDATION: {sedation}.
HISTORY: {history}
PREPARATION: {prep}
EXTENT: {extent}
FINDINGS: {findings}
TISSUE: {tissue}
HISTOLOGY: {histology}
IMPRESSION: {impression}
ENDOSCOPIST: {doctor} (synthetic signatory).""".format(
            facility=facility,
            uid=uid,
            number=spec["n"],
            pdate=pdate,
            age=age,
            sex=sex,
            indication=indication,
            sedation=sedation,
            doctor=doctor,
            **statements
        )

    if layout == "narrative":
        return """{facility} - day-case endoscopy narrative
Synthetic record {uid}; {pdate}; patient {age}/{sex}; MRN SY7-{number:04d}.
Colonoscopy was performed for {indication} using {sedation}. {history}
{prep} {extent}
On inspection, {findings} {tissue} {histology}
ASSESSMENT: {impression}
Signed electronically: {doctor}.""".format(
            facility=facility,
            uid=uid,
            pdate=pdate,
            age=age,
            sex=sex,
            number=spec["n"],
            indication=indication,
            sedation=sedation,
            doctor=doctor,
            **statements
        )

    if layout == "impression_first":
        return """ENDOSCOPY IMPRESSION (synthetic preliminary document)
{impression}

IDENTIFIERS: {uid} / SY7-{number:04d}; {facility}; {pdate}; {age}/{sex}.
REFERRAL: {indication}. {history}
DETAILED REPORT
Sedation | {sedation}
Bowel preparation | {prep}
Insertion and extent | {extent}
Mucosal findings | {findings}
Specimen record | {tissue}
Pathology field | {histology}
Dictated by {doctor}.""".format(
            uid=uid,
            number=spec["n"],
            facility=facility,
            pdate=pdate,
            age=age,
            sex=sex,
            indication=indication,
            sedation=sedation,
            doctor=doctor,
            **statements
        )

    if layout == "tabulated":
        return """ABU DHABI SYNTHETIC ENDOSCOPY WORKSHEET
FIELD | ENTRY
Unit | {facility}
Case | {uid}; MRN SY7-{number:04d}; {age}/{sex}
Date | {pdate}
Indication | {indication}
Prior history | {history}
Sedation | {sedation}
Preparation | {prep}
Proximal extent | {extent}
Findings | {findings}
Tissue | {tissue}
Histology | {histology}
Conclusion | {impression}
Operator | {doctor}""".format(
            facility=facility,
            uid=uid,
            number=spec["n"],
            age=age,
            sex=sex,
            pdate=pdate,
            indication=indication,
            sedation=sedation,
            doctor=doctor,
            **statements
        )

    if layout == "discharge":
        return """DAY SURGERY DISCHARGE SUMMARY - SYNTHETIC
{facility} | {pdate} | discharge reference {uid}
Patient label: SY7-{number:04d}, {age}/{sex}. Procedure: colonoscopy for {indication}.
{history}
Recovery followed {sedation}; observations at discharge were stable.
Endoscopy account: {prep} {extent} {findings}
Specimen advice: {tissue} {histology}
PLAN: {impression}
Routine safety-net advice was given. Discharge signatory: {doctor}.""".format(
            facility=facility,
            pdate=pdate,
            uid=uid,
            number=spec["n"],
            age=age,
            sex=sex,
            indication=indication,
            sedation=sedation,
            doctor=doctor,
            **statements
        )

    if layout == "dictated":
        return """DICTATED ENDOSCOPY NOTE - SYNTHETIC TRAINING CASE {uid}
{facility}, {pdate}. Patient SY7-{number:04d}, {age}/{sex}.
Indication was {indication}; sedation was {sedation}. {history}
Regarding preparation, {prep}
Regarding insertion, {extent}
Regarding findings, {findings}
Regarding specimens, {tissue} {histology}
In summary, {impression}
Dictation attributed to synthetic operator {doctor}.""".format(
            uid=uid,
            facility=facility,
            pdate=pdate,
            number=spec["n"],
            age=age,
            sex=sex,
            indication=indication,
            sedation=sedation,
            doctor=doctor,
            **statements
        )

    if layout == "compact":
        return """Colonoscopy/{uid} [{facility}; {pdate}; SY7-{number:04d}; {age}/{sex}]
Indication={indication}; sedation={sedation}. History: {history}
Prep={prep}
Extent={extent}
Findings={findings}
Specimen/Histology={tissue} {histology}
Conclusion={impression} Operator={doctor}.""".format(
            uid=uid,
            facility=facility,
            pdate=pdate,
            number=spec["n"],
            age=age,
            sex=sex,
            indication=indication,
            sedation=sedation,
            doctor=doctor,
            **statements
        )

    return """ENDOSCOPY CLINICAL LETTER - SYNTHETIC
{facility}
Reference {uid} / SY7-{number:04d}; procedure {pdate}; patient {age}/{sex}.
Dear colleague,
This patient underwent colonoscopy for {indication} with {sedation}. {history}
{prep} {extent}
{findings} {tissue}
{histology}
Yours sincerely,
{doctor}
Clinical conclusion for the coding team: {impression}""".format(
        facility=facility,
        uid=uid,
        number=spec["n"],
        pdate=pdate,
        age=age,
        sex=sex,
        indication=indication,
        sedation=sedation,
        doctor=doctor,
        **statements
    )


def initial_key_for_render(spec):
    key = dict(spec["key"])
    add_type = spec["add_type"]
    if add_type == "extent_complete_to_incomplete":
        key["caecum"] = "yes"
    elif add_type == "extent_incomplete_to_complete":
        key["caecum"] = "no"
    elif add_type == "prep_adequate_to_inadequate":
        key["prep"] = "adequate"
    elif add_type == "prep_inadequate_to_adequate":
        key["prep"] = "inadequate"
    elif add_type == "histology_pending_to_result":
        key["histology"] = "no"
    return key


def addendum_text(spec):
    add_type = spec["add_type"]
    caecum, caecal, _, _, visualised, _ = caecal_terms(spec)

    if add_type == "extent_complete_to_incomplete":
        return pick(
            spec,
            [
                "CORRECTION (final): Image review shows that the proximal limit was the hepatic flexure. The %s was not reached; the original statement of successful intubation is withdrawn." % caecum,
                "ADDENDUM: The extent field was signed in error. This examination stopped short of the %s and must be coded as incomplete." % caecum,
            ],
            60,
        )
    if add_type == "extent_incomplete_to_complete":
        return pick(
            spec,
            [
                "ADDENDUM (final): The incomplete-exam sentence was dictated in error. The %s was reached and the %s landmarks were photographed; this supersedes the original extent statement." % (caecum, caecal),
                "CORRECTION: Recovery of the image set confirms successful %s intubation. The examination was complete to the %s." % (caecal, caecum),
            ],
            61,
        )
    if add_type == "prep_adequate_to_inadequate":
        return pick(
            spec,
            [
                "CORRECTION (final preparation assessment): Adherent solid stool remained in the right colon after washing. Overall preparation was inadequate, replacing the earlier adequate grade.",
                "ADDENDUM: The initial grade referred to the left colon only. The right colon was poorly prepared, so the final preparation assessment is inadequate.",
            ],
            62,
        )
    if add_type == "prep_inadequate_to_adequate":
        return pick(
            spec,
            [
                "ADDENDUM (final): The earlier poor-preparation comment described the view before irrigation. After washing, all segments were adequately %s; final preparation was good." % visualised,
                "CORRECTION: The final post-wash assessment was adequate in every segment. This replaces the preliminary inadequate-preparation wording.",
            ],
            63,
        )
    return pick(
        spec,
        [
            "PATHOLOGY ADDENDUM: Histology for the specimen collected at this procedure is now available: %s. This is the current result, not prior history." % current_histology_result(spec),
            "FINAL HISTOLOGY CORRECTION: The pending status is superseded. The laboratory reports %s for this procedure's tissue." % current_histology_result(spec),
        ],
        64,
    )


def arabic_summary(spec):
    key = spec["key"]
    lines = ["ملخص عربي للتقرير السريري الاصطناعي " + spec["uid"]]
    if key["caecum"] == "yes":
        lines.append("تم الوصول إلى الأعور مع توثيق المعالم التشريحية المناسبة.")
    elif key["caecum"] == "no":
        lines.append("لم يتم الوصول إلى الأعور، والفحص الحالي غير مكتمل.")
    else:
        lines.append("لم يُذكر مدى إدخال المنظار في التقرير المتاح.")

    if key["prep"] == "adequate":
        lines.append("كانت تهيئة الأمعاء كافية وسمحت بفحص الغشاء المخاطي.")
    elif key["prep"] == "inadequate":
        lines.append("كانت تهيئة الأمعاء غير كافية بسبب بقايا تمنع التقييم الموثوق.")
    else:
        lines.append("لم تُدرج درجة أو وصف واضح لجودة تهيئة الأمعاء.")

    if key["tissue_taken"] == "yes":
        lines.append("أُخذ نسيج خلال هذا الإجراء، إما بخزعة أو باستئصال سليلة.")
    else:
        lines.append("لم تؤخذ خزعات ولم تُجرَ إزالة نسيج خلال هذا الإجراء.")

    if key["histology"] == "yes":
        lines.append("نتيجة الفحص النسيجي الخاصة بنسيج هذا الإجراء موجودة في التقرير.")
    elif key["tissue_taken"] == "yes":
        lines.append("لا توجد نتيجة نسيجية لهذا الإجراء في النص الحالي.")
    else:
        lines.append("لا توجد عينة حالية للفحص النسيجي.")
    return "\n".join(lines)


def ocr_noisify(text, n):
    lines = text.splitlines()
    noisy = []
    for j, line in enumerate(lines):
        if line.startswith(("Report ID", "MRN", "ENDOSCOPIST", "Operator")):
            line = line.replace("0", "O").replace("1", "l")
        if j == 0:
            line = line.replace("O", "0", 1) + "  [scan]"
        if line and (j + n) % 13 == 0:
            line += "  ¦"
        noisy.append(line)
    noisy.insert(2, "[OCR artefact %03d: faint vertical streak / / /; clinical duplicate checked]" % n)
    return "\n".join(noisy)


def render_text(spec):
    initial_key = initial_key_for_render(spec)
    statements = make_statements(spec, initial_key)
    text = render_layout(spec, statements)
    if spec["add_type"]:
        text += "\n\n" + addendum_text(spec)
    if spec["arabic"]:
        text += "\n\n" + arabic_summary(spec)
    if spec["ocr"]:
        text = ocr_noisify(text, spec["n"])
    return text.rstrip() + "\n"


def build_reports(specs):
    reports = []
    for spec in specs:
        key = dict(spec["key"])
        key_state, key_reason = derive_key_state(key)
        text = render_text(spec)
        sha = hashlib.sha256(text.encode("utf-8")).hexdigest()
        report = {
            "uid": spec["uid"],
            "sha": sha,
            "text": text,
            "truth": spec["truth"],
            "key_fields": key,
            "key_state": key_state,
            "key_reason": key_reason,
            "tags": sorted(spec["tags"]),
        }
        reports.append(report)
    return reports


def validate(specs, reports):
    assert len(reports) == 300
    assert len({r["uid"] for r in reports}) == 300
    assert len({r["text"] for r in reports}) == 300

    truth_counts = Counter(r["truth"] for r in reports)
    assert truth_counts == {
        "adequate": 120,
        "awaiting_histology": 90,
        "inadequate": 90,
    }

    tag_counts = Counter(tag for report in reports for tag in report["tags"])
    assert tag_counts["fmt:arabic_bilingual"] == 33
    assert tag_counts["fmt:ocr_scan"] == 25
    assert tag_counts["prep:ungraded"] == 50
    assert tag_counts["prep:boston"] == 48
    assert tag_counts["hard:non_polyp_biopsy"] == 36
    assert tag_counts["tissue:biopsy"] == 70
    assert tag_counts["hard:negated_tissue"] == 28
    assert tag_counts["hard:addendum_correction"] == 16
    assert tag_counts["hard:prior_procedure"] == 24
    assert tag_counts["hard:missing_documentation"] == 87
    assert tag_counts["hidden:undocumented_failure"] == 12

    layout_tags = {"fmt:" + layout for layout in LAYOUTS}
    assert len(layout_tags.intersection(tag_counts)) == 8

    assert sum(r["key_fields"]["caecum"] == "not_stated" for r in reports) == 37
    assert sum(r["key_fields"]["prep"] == "not_stated" for r in reports) == 50

    expected_tag_counts = {
        "fmt:structured": 38,
        "fmt:narrative": 38,
        "fmt:impression_first": 38,
        "fmt:tabulated": 38,
        "fmt:discharge": 37,
        "fmt:dictated": 37,
        "fmt:compact": 37,
        "fmt:clinical_letter": 37,
        "spelling:uk": 150,
        "spelling:us": 150,
        "prep:verbal": 202,
        "tissue:none": 83,
        "tissue:polypectomy": 125,
        "tissue:biopsy": 70,
        "tissue:polyp_not_removed": 12,
        "tissue:planned_not_done": 10,
        "hard:bbps_total_trap": 12,
        "hard:prep_rescue": 12,
        "hard:prep_segment_overrides_overall": 10,
        "hard:extent_by_landmarks": 18,
        "hard:ti_implies_caecum": 12,
        "hard:withdrawal_phrase": 12,
        "hard:no_polyps_but_biopsied": 12,
        "hard:specimen_no_result": 8,
        "hard:histology_request_not_result": 14,
        "hard:polyp_seen_not_removed": 12,
        "hard:planned_not_done": 10,
        "hard:prior_histology_not_current": 8,
        "hard:prior_incomplete_not_current": 8,
        "hard:impression_only_failure": 12,
        "hard:ocr_noise": 25,
    }
    for tag, expected in expected_tag_counts.items():
        assert tag_counts[tag] == expected, (tag, tag_counts[tag], expected)

    for spec, report in zip(specs, reports):
        key = report["key_fields"]
        assert key["polyps"] == key["tissue_taken"]
        assert report["truth"] == truth_from_reality(spec["reality"])
        assert (report["key_state"], report["key_reason"]) == derive_key_state(key)
        assert report["sha"] == hashlib.sha256(report["text"].encode("utf-8")).hexdigest()
        assert any(tag.startswith("fmt:") for tag in report["tags"])
        assert any(tag.startswith("spelling:") for tag in report["tags"])
        assert any(tag.startswith("prep:") for tag in report["tags"])
        assert any(tag.startswith("tissue:") for tag in report["tags"])
        if key["histology"] == "yes":
            assert key["tissue_taken"] == "yes"
        if key["caecum"] != "not_stated":
            assert key["caecum"] == yes_no(spec["reality"]["caecum"])
        if key["prep"] != "not_stated":
            expected_prep = "adequate" if spec["reality"]["prep"] else "inadequate"
            assert key["prep"] == expected_prep
        assert key["tissue_taken"] == yes_no(spec["reality"]["tissue"])
        assert key["histology"] == yes_no(spec["reality"]["histology"])
        if spec["prep_mode"] == "boston":
            scores = spec["boston_scores"]
            expected_prep = "adequate" if min(scores) >= 2 else "inadequate"
            assert key["prep"] == expected_prep

    hard_report_count = sum(
        any(tag.startswith("hard:") for tag in report["tags"])
        for report in reports
    )
    assert hard_report_count >= 105


def main():
    rng = random.Random(SEED)
    specs = make_specs(rng)
    assign_features(specs)
    reports = build_reports(specs)
    validate(specs, reports)
    payload = {"source": SOURCE, "reports": reports}
    destination = os.path.join(
        os.path.dirname(os.path.abspath(__file__)), OUTPUT_NAME
    )
    with open(destination, "w", encoding="utf-8", newline="\n") as handle:
        json.dump(payload, handle, ensure_ascii=False, indent=2)
        handle.write("\n")


if __name__ == "__main__":
    main()
