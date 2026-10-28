"""Evidence-reader prompt (shared by the 26B teacher and the local 4B hybrid reader)."""
import json,re
FIELDS="""Fields (use ONLY what the text explicitly states; never infer):
- caecum_reached: "yes" if the text documents the scope reached the caecum/cecum, ileocaecal valve or terminal ileum; "no" if it documents the exam stopped short of the caecum; "not_stated" otherwise (including no colonoscopy performed, or caecum surgically absent).
- prep: "adequate" if bowel preparation quality is explicitly graded acceptable (excellent/good/adequate/fair/well prepared, or a Boston Bowel Preparation Scale total >=6 with every segment >=2); "inadequate" if graded poor/inadequate, or a Boston total <6 or any segment <2; "not_stated" if not graded (naming or taking a preparation product is not a grade).
- tissue_taken: "yes" if any tissue was removed or sampled during this procedure: a colonic polyp removed (snare/forceps/ablation/fulguration) OR any biopsy taken (of a mass, ulcer or stricture, or random/mucosal biopsies); else "no".
- histology_present: "yes" only if the document contains a histopathology RESULT (e.g. tubular adenoma, adenocarcinoma, microscopic description); "no" if absent or pending.
For every field give a SHORT verbatim quote copied exactly from the text that supports the label ("" if not_stated/no with no supporting text)."""
ONE=f"""You extract colonoscopy screening-closure evidence from a clinical document.
{FIELDS}
Document:
<<<{{t}}>>>
Return ONLY JSON: {{{{"caecum_reached":"yes|no|not_stated","caecum_quote":"","prep":"adequate|inadequate|not_stated","prep_quote":"","tissue_taken":"yes|no","tissue_quote":"","histology_present":"yes|no","histology_quote":""}}}}"""
def pj(t,arr=False):
    m=re.search(r'\[.*\]' if arr else r'\{.*\}',t or '',re.S)
    try: return json.loads(m.group(0))
    except Exception: return [] if arr else {}
