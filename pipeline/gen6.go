package main

// v6 generator support: free-text pools (hand-labelled e-consult and teach-back sets), cervical colposcopy/pathology report
// templates (synthetic, authored here), and the parse cache that the prep stage uses (Gemma verdicts keyed by sha256(text)).
import (
	"encoding/json"
	"math/rand"
	"os"
)

type LabText struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Style string `json:"style"`
	Prog  string `json:"prog"`
	Text  string `json:"text"`
}

type Pools struct {
	EC map[string][]LabText // label -> items (any prog)
	TB map[string]map[string][]LabText
}

func loadPools(ecPath, tbPath string) *Pools {
	P := &Pools{EC: map[string][]LabText{}, TB: map[string]map[string][]LabText{"crc": {}, "cervical": {}}}
	var ec, tb []LabText
	if b, err := os.ReadFile(ecPath); err == nil {
		json.Unmarshal(b, &ec)
	} else {
		panic(err)
	}
	if b, err := os.ReadFile(tbPath); err == nil {
		json.Unmarshal(b, &tb)
	} else {
		panic(err)
	}
	for _, x := range ec {
		P.EC[x.Label] = append(P.EC[x.Label], x)
	}
	for _, x := range tb {
		P.TB[x.Prog][x.Label] = append(P.TB[x.Prog][x.Label], x)
	}
	return P
}

func (P *Pools) ec(label string, r *rand.Rand) LabText {
	xs := P.EC[label]
	return xs[r.Intn(len(xs))]
}

// tb draws a teach-back reply for the programme; falls back to the CRC pool when the cervical pool lacks the label.
func (P *Pools) tb(prog, label string, r *rand.Rand) LabText {
	xs := P.TB[prog][label]
	if len(xs) < 3 {
		xs = append(append([]LabText{}, xs...), P.TB["crc"][label]...)
	}
	return xs[r.Intn(len(xs))]
}

func drawLabel(r *rand.Rand, labels []string, probs []float64) string {
	x := r.Float64()
	for i, p := range probs {
		if x < p {
			return labels[i]
		}
		x -= p
	}
	return labels[len(labels)-1]
}

// Referrer-side e-consult questions (outbound, not parsed; structured trigger is in the extension).
var ecQuestions = map[string]string{
	"rejected":   "Referral returned by your unit. Patient has a positive screening result and remains unseen. Please advise: can you accept, what is needed, or where should we refer?",
	"unanswered": "No response to our referral after 7 days. Patient has a positive screening result awaiting follow-up. Can you accept, or advise an alternative?",
	"inadequate": "Procedure report indicates the examination was inadequate. Please advise on repeat procedure, conditions, or alternative pathway.",
	"followup":   "Condition / requested information now provided (see attached). Please confirm booking.",
}

// ---------------- cervical report templates (synthetic, authored for v6; disclosed) ----------------
// truth: adequate | awaiting_histology | inadequate (colposcopy) ; histology | no_histology (cervical pathology)
// coded: the structured conclusion code a colposcopy system records alongside the narrative (second reader).
type CervTpl struct {
	ID, Kind, Truth, Coded, Text string
}

var CervTpls = []CervTpl{
	{"colpo-sat-nobx", "colposcopy", "adequate", "satisfactory-no-biopsy", "COLPOSCOPY REPORT. Indication: HPV positive, cytology NILM. Cervix visualised fully, transformation zone type 1, squamocolumnar junction seen in entirety. Acetic acid: no acetowhite change. Lugol's iodine uptake uniform. Impression: normal colposcopy, satisfactory examination. Plan: return to routine recall with co-test in 12 months."},
	{"colpo-sat-nobx2", "colposcopy", "adequate", "satisfactory-no-biopsy", "Colposcopy. Referral: ASC-US, HPV+. TZ type 2, fully visualised. Minor faint acetowhite area at 3 o'clock consistent with immature metaplasia. Examination adequate. No lesion requiring sampling. Discharged to primary care for repeat co-test in 1 year."},
	{"colpo-sat-nobx-trap", "colposcopy", "adequate", "satisfactory-no-biopsy", "COLPOSCOPY. Indication LSIL. Satisfactory colposcopy, TZ type 1. Squamous epithelium normal after acetic acid and iodine; no biopsy was necessary. Plan: cytology in 12 months."},
	{"colpo-bx-lsil", "colposcopy", "awaiting_histology", "satisfactory-biopsy", "COLPOSCOPY REPORT. Indication: LSIL. TZ type 1, satisfactory. Thin acetowhite epithelium with fine mosaic at 6-7 o'clock (Swede score 4). Two punch biopsies taken from 6 and 7 o'clock. Plan: review with histology."},
	{"colpo-bx-hsil", "colposcopy", "awaiting_histology", "satisfactory-biopsy", "Colposcopy. Referral HSIL. Transformation zone type 2, adequate examination. Dense acetowhite epithelium with coarse punctation, anterior lip (Swede score 7). Directed biopsy x2 obtained. Patient counselled regarding possible LLETZ pending pathology."},
	{"colpo-bx-ascus", "colposcopy", "awaiting_histology", "satisfactory-biopsy", "COLPOSCOPY. ASC-H referral. Squamocolumnar junction fully visible (TZ1). Acetowhite area at 12 o'clock with irregular margins; sampled by punch biopsy. Await results before management decision."},
	{"colpo-unsat-tz3", "colposcopy", "inadequate", "unsatisfactory", "COLPOSCOPY REPORT. Indication: HSIL. Transformation zone type 3; upper limit not visualised despite endocervical speculum. Examination unsatisfactory. Plan: discuss diagnostic excision versus repeat colposcopy with endocervical sampling at MDT."},
	{"colpo-unsat-trap", "colposcopy", "inadequate", "unsatisfactory", "Colposcopy. Referral ASC-US/HPV+. Postmenopausal, atrophic changes. The squamocolumnar junction could not be seen in its entirety and the endocervical canal could not be assessed. Recommend vaginal oestrogen for 4 weeks and repeat the examination."},
	{"cpath-cin2", "cervpath", "histology", "histology-present", "PATHOLOGY. Specimen: cervical punch biopsies x2. Microscopic: squamous epithelium with full-thickness loss of maturation in the lower two thirds, mitoses above the basal layer. DIAGNOSIS: HSIL (CIN 2). p16 block positive."},
	{"cpath-cin1", "cervpath", "histology", "histology-present", "Histopathology, cervix biopsy (6 o'clock). Koilocytic atypia confined to the lower third of the epithelium. DIAGNOSIS: LSIL (CIN 1). No high-grade dysplasia."},
	{"cpath-benign", "cervpath", "histology", "histology-present", "PATHOLOGY. Cervical biopsy. Microscopic examination: squamous metaplasia and chronic cervicitis. No dysplasia or malignancy identified."},
	{"cpath-pend", "cervpath", "no_histology", "histology-absent", "Pathology: specimen (cervical biopsy x2) received in formalin. Processing. Final report to follow."},
}

func cervPool(kind, truth string) []CervTpl {
	out := []CervTpl{}
	for _, t := range CervTpls {
		if t.Kind == kind && (truth == "" || t.Truth == truth) {
			out = append(out, t)
		}
	}
	return out
}

// ---------------- parse cache (prep stage) ----------------
// sha256(text) -> label produced by Gemma 4 26B on the frozen test sets (econsult/pred_gemma.json, teachback/pred_gemma.json).
type Parser struct{ m map[string]string }

var Parse *Parser

func LoadParser(path string) *Parser {
	p := &Parser{m: map[string]string{}}
	b, err := os.ReadFile(path)
	if err != nil {
		return p
	}
	json.Unmarshal(b, &p.m)
	return p
}

func (p *Parser) Get(text string) string {
	if p == nil {
		return "unparsed"
	}
	if v, ok := p.m[textSha(text)]; ok {
		return v
	}
	return "unparsed"
}
