package main

// Synthetic Abu Dhabi multi-provider FIT+ cohort. Persons = Wasl's held-out Wikidata eval split (Arabic + Latin names, QID truth).
// Emits FHIR-shaped JSON resources per provider (sorted by arrival), the ground truth, and the precomputed evidence-label file
// that the EvidenceJudge stub reads (stands in for the separately built student model).
import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Person struct {
	Qid string   `json:"qid"`
	Ar  string   `json:"ar"`
	En  string   `json:"en"`
	Als []string `json:"als"`
	Dob string   `json:"dob"`
	Sex string   `json:"sex"`
	Cc  string   `json:"cc"`
	Eid string   `json:"eid"`
	Spl string   `json:"split"`
	Origin string `json:"origin"`
	ArSrc  string `json:"ar_src"`
}

type ProvCfg struct {
	ID, Name, Kind string
	PAr            float64 // P(Arabic-script name at registration)
}

var Providers = []ProvCfg{
	{"ahs-mushrif", "Al Mushrif Healthcare Centre (primary care)", "clinic", 0.90},
	{"medlab", "Private reference laboratory (FIT screening)", "lab", 0.0},
	{"skmc-endo", "Endoscopy unit, tertiary hospital", "hospital", 0.15},
	{"noor-endo", "Endoscopy suite, private hospital", "hospital", 0.45},
	{"corniche-wh", "Women's health and colposcopy clinic, specialist hospital", "hospital", 0.30},
}

const TEnd = 120.0
const SLADays = 31.0

type GenEvent struct {
	Arrival float64                `json:"arrival"`
	Prov    string                 `json:"prov"`
	MsgID   string                 `json:"msgid"`
	Res     map[string]interface{} `json:"res"`
}

type LoopTruth struct {
	Loop       string  `json:"loop"`
	Qid        string  `json:"qid"`
	Cc         string  `json:"cc"`
	Origin     string  `json:"origin"`
	ArSrc      string  `json:"ar_src"`
	Prov       string  `json:"prov"`
	Hosp       string  `json:"hosp"`
	Pathway    string  `json:"pathway"`
	SRDay      float64 `json:"sr_day"`
	State      string  `json:"state"`
	ClosedDay  float64 `json:"closed_day,omitempty"`
	Overdue    bool    `json:"overdue"`
	Unowned    bool    `json:"unowned"`
	ScriptSR   string  `json:"script_sr"`
	ScriptHosp string  `json:"script_hosp"`
	TrueLang   string  `json:"true_lang"`  // patient's actual preferred language
	RecLang    string  `json:"rec_lang"`   // language recorded at the referring provider ("" = not recorded)
	ExpLang    string  `json:"exp_lang"`   // language the notification must use (recorded, else provider default)
	NotifDay   float64 `json:"notif_day"`  // first valid (approved, correct-language, delivered) notification; -1 none
	AckDay     float64 `json:"ack_day"`    // -1 none
	RefStatus  string  `json:"ref_status"` // latest receiving-unit response: accepted|rejected|none
	RefDay     float64 `json:"ref_day"`    // first response day; -1 none
	Dropoff    string  `json:"dropoff"`
	BadMsgs    []string `json:"bad_msgs,omitempty"` // injected non-compliant outbound messages (kind)
	Reports    []string `json:"reports,omitempty"`  // MTSamples uids used on this loop's colonoscopies
	// v6
	Prog       string   `json:"prog"`
	Cat        string   `json:"icat,omitempty"`
	Priority   string   `json:"prio,omitempty"`
	IndexDay   float64  `json:"iday"`
	Deadline   float64  `json:"deadline"`
	ProcDay    float64  `json:"proc_day"`             // first fulfilled procedure day (any adequacy); -1 none
	EcState    string   `json:"ec_state"`             // last e-consult state by TEnd
	EcTurns    int      `json:"ec_turns"`
	EcTrigger  string   `json:"ec_trigger,omitempty"` // rejected|unanswered|inadequate
	TBState    string   `json:"tb_state"`             // last teach-back state by TEnd
	TBLatent   string   `json:"tb_latent,omitempty"`  // first teach-back reply label (patient comprehension at first contact)
	Lung       *LungTruth `json:"lung,omitempty"`     // incidental lung-nodule loop (genlung.go)
	Handoff    *HandoffTruth `json:"handoff,omitempty"` // finally rejected referral: packet day and the referrer's action (handoff.go)
}

// HandoffTruth: the simulated hand-off of a finally rejected referral.
type HandoffTruth struct {
	Decided float64 `json:"decided"`
	ActBy   float64 `json:"act_by"`
	ActDay  float64 `json:"act_day"` // -1 = never (or after TEnd)
	Urgent  bool    `json:"urgent"`
}

type Truth struct {
	Loops   map[string]*LoopTruth `json:"loops"`
	PKQid   map[string]string     `json:"pk_qid"`
	PKCc    map[string]string     `json:"pk_cc"`
	Reports map[string]string     `json:"reports"` // report id -> adequate|inadequate|awaiting_histology|histology|no_histology
	ReportUID map[string]string   `json:"report_uid"` // report id -> MTSamples uid or pathology template id
	Distract map[string]bool      `json:"distract_reports"`
	Photos   map[string][]PhotoTruth `json:"report_photos,omitempty"` // -imgphotos: photo refs attached to each report, with true class
}

// ---------- noise (port of wasl/src/common.py) ----------
const keyb = "qwertyuiopasdfghjklzxcvbnm"

func typo(s string, r *rand.Rand) string {
	if len(s) < 3 {
		return s
	}
	i := r.Intn(len(s))
	op := r.Float64()
	switch {
	case op < .3:
		return s[:i] + string(keyb[r.Intn(26)]) + s[i+1:]
	case op < .55:
		return s[:i] + s[i+1:]
	case op < .8:
		return s[:i] + string("aeiouy"[r.Intn(6)]) + s[i:]
	case i < len(s)-1:
		return s[:i] + string(s[i+1]) + string(s[i]) + s[i+2:]
	}
	return s
}

var reAlDash = regexp.MustCompile(`\b[Aa]l-`)

func noisyLatin(name string, r *rand.Rand) string {
	toks := strings.Fields(name)
	if len(toks) >= 3 && r.Float64() < .2 {
		k := 1 + r.Intn(len(toks)-2)
		toks = append(toks[:k:k], toks[k+1:]...)
	}
	s := strings.Join(toks, " ")
	x := r.Float64()
	if x < .25 {
		s = reAlDash.ReplaceAllString(s, "Al ")
	} else if x < .4 {
		s = reAlDash.ReplaceAllString(s, "Al")
	} else if x < .5 {
		s = reAlDash.ReplaceAllString(s, "")
	}
	if r.Float64() < .25 {
		s = typo(s, r)
	}
	if r.Float64() < .1 {
		s = strings.ToUpper(s)
	}
	return s
}

func noisyArabic(name string, r *rand.Rand) string {
	toks := strings.Fields(name)
	if len(toks) >= 3 && r.Float64() < .2 {
		k := 1 + r.Intn(len(toks)-2)
		toks = append(toks[:k:k], toks[k+1:]...)
	}
	s := strings.Join(toks, " ")
	if r.Float64() < .3 {
		s = strings.NewReplacer("أ", "ا", "إ", "ا", "آ", "ا").Replace(s)
	}
	if r.Float64() < .2 {
		s = strings.ReplaceAll(s, "ة", "ه")
	}
	if r.Float64() < .2 {
		s = strings.ReplaceAll(s, "عبد ال", "عبدال")
	}
	if rs := []rune(s); r.Float64() < .08 && len(rs) > 4 {
		i := r.Intn(len(rs))
		s = string(append(rs[:i:i], rs[i+1:]...))
	}
	return s
}

func noisyDob(dob string, r *rand.Rand, p float64) string {
	if r.Float64() > p {
		return dob
	}
	y, m, d := dob[:4], dob[5:7], dob[8:10]
	x := r.Float64()
	switch {
	case x < .25:
		return ""
	case x < .5:
		return y + "-07-01"
	case x < .75 && atoi(d) <= 12 && d != m:
		return y + "-" + d + "-" + m
	}
	dy := 1
	if r.Intn(2) == 0 {
		dy = -1
	}
	return fmt.Sprintf("%04d-%s-%s", atoi(y)+dy, m, d)
}

func luhnOK(s string) bool {
	if len(s) != 15 {
		return false
	}
	sum := 0
	for i := 0; i < 15; i++ {
		d := int(s[14-i] - '0')
		if d < 0 || d > 9 {
			return false
		}
		if i%2 == 1 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return sum%10 == 0
}

type reg struct {
	pk, mrn, name, dob, sex, eid, script, lang string
}

type RepSrc struct {
	UID       string            `json:"uid"`
	Text      string            `json:"text"`
	Truth     string            `json:"truth"`
	KeyFields map[string]string `json:"key_fields"` // caecum/prep/polyps/histology answer key (used only for synthetic photos)
}

type PathTpl struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Histology bool   `json:"histology"`
}

var provDefaultLang = map[string]string{"ahs-mushrif": "ar", "medlab": "en", "skmc-endo": "en", "noor-endo": "en", "corniche-wh": "en"}

var disclosures = []struct{ lang, text string }{
	{"en", "Your FIT stool test came back positive. Please book a colonoscopy."},
	{"en", "Dear patient, blood was found in your stool sample. Call the clinic."},
	{"en", "Your screening result is abnormal and needs a colonoscopy."},
	{"ar", "نتيجة فحص البراز إيجابية، يرجى حجز موعد منظار."},
	{"ar", "تم العثور على دم في عينة البراز، يرجى مراجعة العيادة."},
}

var disclosuresCerv = []struct{ lang, text string }{
	{"en", "Your Pap smear came back abnormal. Please book a colposcopy."},
	{"en", "Your HPV test is positive, please call the clinic."},
	{"ar", "نتيجة مسحة عنق الرحم غير طبيعية، يرجى حجز موعد للتنظير."},
}

var rejectReasons = []string{"incomplete-referral", "out-of-network", "capacity", "duplicate-referral"}

// CRCFemaleShare: share of CRC (FIT+) loops for women; 69.6% of FIT+ patients were women in the SEHA cohort (PMC8343888).
const CRCFemaleShare = 0.696

func crcFemale(ps []Person, crc []int) float64 {
	n := 0
	for _, i := range crc {
		if ps[i].Sex == "F" {
			n++
		}
	}
	return float64(n) / float64(len(crc))
}

// GenSplit selects the person split a cohort is drawn from: eval (default) | cal (Learn-then-Test calibration cohorts).
var GenSplit = "eval"

func runGen(dataDir, outDir, reportsPath, pathPath string, seed int64, nLoops, nDistract, nPersons, nCerv int, pools *Pools, nLung int, lungDir string) {
	b, err := os.ReadFile(filepath.Join(dataDir, "persons3.json"))
	if err != nil {
		panic(err)
	}
	var all []Person
	json.Unmarshal(b, &all)
	ps := []Person{}
	for _, p := range all {
		if p.Spl == GenSplit { // default "eval": held out from encoder, scorer and calibrator training; "cal" for Learn-then-Test calibration
			ps = append(ps, p)
		}
	}
	var repFile struct {
		Reports []RepSrc `json:"reports"`
	}
	b, err = os.ReadFile(reportsPath)
	if err != nil {
		panic(err)
	}
	json.Unmarshal(b, &repFile)
	var paths []PathTpl
	b, err = os.ReadFile(pathPath)
	if err != nil {
		panic(err)
	}
	json.Unmarshal(b, &paths)
	var poolOK, poolBad []RepSrc
	for _, x := range repFile.Reports {
		if x.Truth == "inadequate" {
			poolBad = append(poolBad, x)
		} else {
			poolOK = append(poolOK, x)
		}
	}
	var pathFinal, pathPend []PathTpl
	for _, x := range paths {
		if x.Histology {
			pathFinal = append(pathFinal, x)
		} else {
			pathPend = append(pathPend, x)
		}
	}
	r := rand.New(rand.NewSource(seed))
	var photos *photoPool
	if ImgPhotos {
		photos = loadPhotoPool(ImgCachePath)
	}
	r.Shuffle(len(ps), func(i, j int) { ps[i], ps[j] = ps[j], ps[i] })
	if nPersons > 0 && nPersons < len(ps) {
		// keep every woman: the Wikidata splits are ~22% female, and cervical loops plus 69.6% of CRC loops need women
		sort.SliceStable(ps, func(i, j int) bool { return ps[i].Sex == "F" && ps[j].Sex != "F" })
		ps = ps[:nPersons]
		r.Shuffle(len(ps), func(i, j int) { ps[i], ps[j] = ps[j], ps[i] })
	}
	if nLoops+nDistract+nCerv > len(ps) {
		panic("not enough persons")
	}
	tr := &Truth{Photos: map[string][]PhotoTruth{}, Loops: map[string]*LoopTruth{}, PKQid: map[string]string{}, PKCc: map[string]string{}, Reports: map[string]string{},
		ReportUID: map[string]string{}, Distract: map[string]bool{}}
	evs := []GenEvent{}
	regs := map[string]*reg{}
	mrnN := map[string]int{}
	ctr := map[string]int{}
	textUse := map[string]int{}
	nextID := func(prov, typ string) string {
		ctr[prov+typ]++
		return fmt.Sprintf("%s-%s-%d", strings.ToUpper(prov[:3]), typ, ctr[prov+typ])
	}
	trueLang := map[string]string{}
	langOf := func(p *Person) string {
		if l, ok := trueLang[p.Qid]; ok {
			return l
		}
		l := "en"
		if p.Origin == "arab" && r.Float64() < .85 {
			l = "ar"
		}
		trueLang[p.Qid] = l
		return l
	}
	getReg := func(p *Person, prov ProvCfg) *reg {
		k := p.Qid + "|" + prov.ID
		if g, ok := regs[k]; ok {
			return g
		}
		mrnN[prov.ID]++
		g := &reg{mrn: fmt.Sprintf("%s-MRN%06d", strings.ToUpper(prov.ID[:3]), mrnN[prov.ID])}
		g.pk = prov.ID + "|" + g.mrn
		if r.Float64() < prov.PAr {
			if NoiseModel == "v2" {
				g.name, g.script = noisyArabic2(p.Ar, r), "ar"
			} else {
				g.name, g.script = noisyArabic(p.Ar, r), "ar"
			}
		} else {
			opts := append([]string{p.En}, p.Als...)
			src := p.En
			if r.Float64() < .5 {
				src = opts[r.Intn(len(opts))]
			}
			if NoiseModel == "v2" {
				g.name, g.script = noisyLatin2(src, r), "lat"
			} else {
				g.name, g.script = noisyLatin(src, r), "lat"
			}
		}
		if NoiseModel == "v2" {
			g.dob = noisyDob2(p.Dob, r, .3)
		} else {
			g.dob = noisyDob(p.Dob, r, .3)
		}
		g.sex = p.Sex
		if r.Float64() < .02 {
			g.sex = "U"
		}
		x := r.Float64() // Emirates ID capture: 62% valid, 26% missing, 12% mistyped (one digit)
		switch {
		case x < .62:
			g.eid = p.Eid
		case x < .88:
			g.eid = ""
		default:
			bs := []byte(p.Eid)
			i := 3 + r.Intn(12)
			bs[i] = byte('0' + (int(bs[i]-'0')+1+r.Intn(9))%10)
			g.eid = string(bs)
		}
		if r.Float64() < .82 { // preferred language recorded at this provider
			g.lang = langOf(p)
		}
		regs[k] = g
		tr.PKQid[g.pk] = p.Qid
		tr.PKCc[g.pk] = p.Cc
		return g
	}
	patient := func(g *reg, prov string) map[string]interface{} {
		ids := []interface{}{map[string]interface{}{"system": "urn:ae:" + prov + ":mrn", "value": g.mrn}}
		if g.eid != "" {
			ids = append(ids, map[string]interface{}{"system": "http://doh.gov.ae/fhir/sid/emirates-id", "value": g.eid})
		}
		pt := map[string]interface{}{"resourceType": "Patient", "id": "pt", "identifier": ids,
			"name": []interface{}{map[string]interface{}{"text": g.name}}, "gender": map[string]string{"M": "male", "F": "female"}[g.sex]}
		if g.dob != "" {
			pt["birthDate"] = g.dob
		}
		if g.lang != "" {
			pt["communication"] = []interface{}{map[string]interface{}{"language": map[string]interface{}{"coding": []interface{}{map[string]string{"system": "urn:ietf:bcp:47", "code": g.lang}}}, "preferred": true}}
		}
		return pt
	}
	emit := func(prov string, arrival float64, res map[string]interface{}, g *reg) {
		if arrival > TEnd {
			return
		}
		id := res["id"].(string)
		ver := 1
		if v, ok := res["_v"]; ok {
			ver = v.(int)
			delete(res, "_v")
		}
		res["meta"] = map[string]interface{}{"versionId": fmt.Sprint(ver), "lastUpdated": dayToTS(arrival), "source": "urn:ae:provider:" + prov}
		if g != nil {
			res["contained"] = []interface{}{patient(g, prov)}
			res["subject"] = map[string]string{"reference": "#pt"}
		}
		evs = append(evs, GenEvent{Arrival: arrival, Prov: prov, MsgID: fmt.Sprintf("%s/%s/%s/v%d", prov, res["resourceType"], id, ver), Res: res})
	}
	lognorm := func(median, sigma float64) float64 { return median * math.Exp(sigma*r.NormFloat64()) }
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	type colo struct {
		apptID, hosp        string
		g                   *reg
		book, day, bookArr  float64
		status              string
		truth               string // adequate|awaiting_histology|inadequate (from the real text's resolved key)
		reportID            string
		reportArr, fulArr   float64
		histArr             float64 // arrival of a final pathology report with histology; Inf = none by TEnd
		prepUngraded        bool    // the report text never grades bowel prep (answer key prep = not_stated)
	}
	intake := func(p *Person, hosp ProvCfg, day float64, status, reason, ref, prog string) {
		g := getReg(p, hosp)
		m := map[string]interface{}{"resourceType": "ServiceRequest", "id": nextID(hosp.ID, "intake"), "status": status, "intent": "order",
			"category": []interface{}{map[string]string{"text": "referral-intake"}}, "code": map[string]string{"text": Rules[prog].Proc + " (external referral)"},
			"authoredOn": dayToTS(day)}
		if ref != "" {
			m["basedOn"] = []interface{}{map[string]string{"display": "external referral " + ref}}
		}
		if reason != "" {
			m["extension"] = []interface{}{map[string]string{"url": "urn:wsi:referral-response-reason", "valueString": reason}}
		}
		emit(hosp.ID, day+.02, m, g)
	}
	doColo := func(p *Person, hosp ProvCfg, book, day float64, status string, wantOK bool, srRef string, pool []RepSrc) *colo {
		g := getReg(p, hosp)
		c := &colo{apptID: nextID(hosp.ID, "appt"), hosp: hosp.ID, g: g, book: book, day: day, status: status, histArr: math.Inf(1)}
		appt := func(st string, v int) map[string]interface{} {
			m := map[string]interface{}{"resourceType": "Appointment", "id": c.apptID, "_v": v, "status": st, "start": dayToTS(day),
				"serviceType": []interface{}{map[string]interface{}{"text": "Colonoscopy"}}, "created": dayToTS(book)}
			if srRef != "" {
				m["basedOn"] = []interface{}{map[string]string{"display": "external referral " + srRef}}
			}
			return m
		}
		c.bookArr = book + .05*r.Float64()
		emit(hosp.ID, c.bookArr, appt("booked", 1), g)
		c.fulArr = day + .2 + .5*r.Float64()
		emit(hosp.ID, c.fulArr, appt(status, 2), g)
		if status != "fulfilled" {
			return c
		}
		var src RepSrc
		switch {
		case pool != nil:
			src = pool[r.Intn(len(pool))]
		case wantOK:
			src = poolOK[r.Intn(len(poolOK))]
		default:
			src = poolBad[r.Intn(len(poolBad))]
		}
		c.truth = src.Truth
		c.prepUngraded = src.KeyFields["prep"] == "not_stated"
		c.reportID = nextID(hosp.ID, "dr")
		arr := day + lognorm(1, 0.7)
		if r.Float64() < .10 {
			arr += 5 + 15*r.Float64()
		}
		c.reportArr = arr
		dr := map[string]interface{}{"resourceType": "DiagnosticReport", "id": c.reportID, "status": "final",
			"code": map[string]interface{}{"coding": []interface{}{map[string]string{"system": "http://loinc.org", "code": "18746-8", "display": "Colonoscopy study"}}},
			"effectiveDateTime": dayToTS(day),
			"presentedForm": []interface{}{map[string]string{"contentType": "text/plain; charset=utf-8", "data": b64(src.Text), "title": "MTSamples " + src.UID}}}
		if photos != nil { // separate seeded stream: the rest of the cohort is byte-identical with or without photos
			media, pt := photos.attach(seed, c.reportID, src)
			dr["media"] = media
			if arr <= TEnd {
				tr.Photos[c.reportID] = pt
			}
		}
		emit(hosp.ID, arr, dr, g)
		if arr <= TEnd {
			tr.Reports[c.reportID] = src.Truth
			tr.ReportUID[c.reportID] = src.UID
			textUse[src.UID]++
		}
		if src.Truth == "awaiting_histology" && r.Float64() < .88 { // pathology addendum (synthetic template text)
			t := arr + lognorm(6, .4)
			emitPath := func(tp PathTpl, at float64) {
				pid := nextID(hosp.ID, "path")
				emit(hosp.ID, at, map[string]interface{}{"resourceType": "DiagnosticReport", "id": pid, "status": "final",
					"code": map[string]interface{}{"coding": []interface{}{map[string]string{"system": "http://loinc.org", "code": "11526-1", "display": "Pathology study"}}},
					"effectiveDateTime": dayToTS(at - 1),
					"extension":         []interface{}{map[string]string{"url": "urn:wsi:addendum-to", "valueString": "DiagnosticReport/" + c.reportID}},
					"presentedForm":     []interface{}{map[string]string{"contentType": "text/plain; charset=utf-8", "data": b64(tp.Text), "title": "pathology " + tp.ID}}}, g)
				if at <= TEnd {
					tr.Reports[pid] = map[bool]string{true: "histology", false: "no_histology"}[tp.Histology]
					tr.ReportUID[pid] = tp.ID
					textUse[tp.ID]++
					if tp.Histology && at < c.histArr {
						c.histArr = at
					}
				}
			}
			if r.Float64() < .15 {
				emitPath(pathPend[r.Intn(len(pathPend))], t)
				t += lognorm(7, .3)
			}
			emitPath(pathFinal[r.Intn(len(pathFinal))], t)
		}
		return c
	}
	encounter := func(p *Person, prov ProvCfg, day float64) {
		g := getReg(p, prov)
		emit(prov.ID, day+.01, map[string]interface{}{"resourceType": "Encounter", "id": nextID(prov.ID, "enc"), "status": "finished",
			"class": map[string]string{"code": "AMB"}, "period": map[string]string{"start": dayToTS(day)}}, g)
	}
	// doColpo: colposcopy appointment + synthetic colposcopy report (+ cervical pathology when biopsied). Same colo record shape.
	doColpo := func(p *Person, hosp ProvCfg, book, day float64, status string, wantOK bool, srRef string) *colo {
		g := getReg(p, hosp)
		c := &colo{apptID: nextID(hosp.ID, "appt"), hosp: hosp.ID, g: g, book: book, day: day, status: status, histArr: math.Inf(1)}
		appt := func(st string, v int) map[string]interface{} {
			return map[string]interface{}{"resourceType": "Appointment", "id": c.apptID, "_v": v, "status": st, "start": dayToTS(day),
				"serviceType": []interface{}{map[string]interface{}{"text": "Colposcopy"}}, "created": dayToTS(book),
				"basedOn": []interface{}{map[string]string{"display": "external referral " + srRef}}}
		}
		c.bookArr = book + .05*r.Float64()
		emit(hosp.ID, c.bookArr, appt("booked", 1), g)
		c.fulArr = day + .2 + .5*r.Float64()
		emit(hosp.ID, c.fulArr, appt(status, 2), g)
		if status != "fulfilled" {
			return c
		}
		var tp CervTpl
		if wantOK {
			pool := append(cervPool("colposcopy", "adequate"), cervPool("colposcopy", "awaiting_histology")...)
			tp = pool[r.Intn(len(pool))]
		} else {
			pool := cervPool("colposcopy", "inadequate")
			tp = pool[r.Intn(len(pool))]
		}
		c.truth = tp.Truth
		c.reportID = nextID(hosp.ID, "dr")
		arr := day + lognorm(1, 0.7)
		if r.Float64() < .10 {
			arr += 5 + 15*r.Float64()
		}
		c.reportArr = arr
		emit(hosp.ID, arr, map[string]interface{}{"resourceType": "DiagnosticReport", "id": c.reportID, "status": "final",
			"code":              map[string]interface{}{"coding": []interface{}{map[string]string{"system": "urn:wsi:proc", "code": "colposcopy", "display": "Colposcopy report"}}},
			"conclusionCode":    []interface{}{map[string]interface{}{"text": tp.Coded}},
			"effectiveDateTime": dayToTS(day),
			"presentedForm":     []interface{}{map[string]string{"contentType": "text/plain; charset=utf-8", "data": b64(tp.Text), "title": "colposcopy " + tp.ID}}}, g)
		if arr <= TEnd {
			tr.Reports[c.reportID] = tp.Truth
			tr.ReportUID[c.reportID] = tp.ID
			textUse[tp.ID]++
		}
		if tp.Truth == "awaiting_histology" && r.Float64() < .9 {
			t := arr + lognorm(7, .4)
			emitP := func(pt CervTpl, at float64) {
				pid := nextID(hosp.ID, "path")
				emit(hosp.ID, at, map[string]interface{}{"resourceType": "DiagnosticReport", "id": pid, "status": "final",
					"code":              map[string]interface{}{"coding": []interface{}{map[string]string{"system": "http://loinc.org", "code": "11526-1", "display": "Pathology study"}}},
					"conclusionCode":    []interface{}{map[string]interface{}{"text": pt.Coded}},
					"effectiveDateTime": dayToTS(at - 1),
					"extension":         []interface{}{map[string]string{"url": "urn:wsi:addendum-to", "valueString": "DiagnosticReport/" + c.reportID}},
					"presentedForm":     []interface{}{map[string]string{"contentType": "text/plain; charset=utf-8", "data": b64(pt.Text), "title": "pathology " + pt.ID}}}, g)
				if at <= TEnd {
					tr.Reports[pid] = pt.Truth
					tr.ReportUID[pid] = pt.ID
					textUse[pt.ID]++
					if pt.Truth == "histology" && at < c.histArr {
						c.histArr = at
					}
				}
			}
			if r.Float64() < .15 {
				emitP(cervPool("cervpath", "no_histology")[0], t)
				t += lognorm(7, .3)
			}
			fin := cervPool("cervpath", "histology")
			emitP(fin[r.Intn(len(fin))], t)
		}
		return c
	}
	doProc := func(p *Person, prog string, hosp ProvCfg, book, day float64, status string, wantOK bool, srRef string) *colo {
		if prog == "cervical" {
			return doColpo(p, hosp, book, day, status, wantOK, srRef)
		}
		return doColo(p, hosp, book, day, status, wantOK, srRef, nil)
	}
	other := func(h ProvCfg) ProvCfg {
		if h.ID == Providers[2].ID {
			return Providers[3]
		}
		return Providers[2]
	}
	inf := math.Inf(1)

	// ---------- screening loops (v6: one generator for both programmes; CRC FIT+ -> colonoscopy, cervical -> colposcopy) ----------
	ECL := []string{"accept", "conditional", "needs_info", "redirect", "reject"}
	TBL := []string{"understood", "partial", "misunderstood", "needs_human"}
	arStyles := map[string]bool{"msa": true, "gulf": true, "arabizi": true, "mix": true}
	pickTB := func(prog, label, lang string) LabText {
		for k := 0; k < 12; k++ {
			x := pools.tb(prog, label, r)
			if (lang == "ar") == arStyles[x.Style] || x.Style == "mix" {
				return x
			}
		}
		return pools.tb(prog, label, r)
	}
	pickEC := func(prog, label string) LabText {
		for k := 0; k < 12; k++ {
			x := pools.ec(label, r)
			if x.Prog == prog {
				return x
			}
		}
		return pools.ec(label, r)
	}
	otherFor := func(prog string, h ProvCfg) ProvCfg {
		if prog == "cervical" {
			if h.ID == Providers[4].ID {
				return Providers[3]
			}
			return Providers[4]
		}
		return other(h)
	}
	var handoffJobs []func() // deferred hand-off simulations (run last: see genLoop)
	genLoop := func(p *Person, prog string) {
		rule := Rules[prog]
		src := Providers[0]
		hosp := Providers[2]
		cat, prio := "FIT+", ""
		if prog == "crc" {
			if r.Float64() < .35 {
				src = Providers[1]
			}
			if r.Float64() < .4 {
				hosp = Providers[3]
			}
		} else {
			hosp = Providers[4]
			if r.Float64() < .3 {
				hosp = Providers[3]
			}
			x := r.Float64()
			switch {
			case x < .10:
				cat = "HSIL+"
			case x < .18:
				cat = "ASC-H"
			case x < .50:
				cat = "LSIL"
			case x < .75:
				cat = "ASC-US"
			default:
				cat = "HPV+NILM"
			}
			prio = CervPriority(cat)
		}
		deadline := DeadlineDays(prog, prio)
		g := getReg(p, src)
		sr := r.Float64() * 100
		index := sr
		if prog == "cervical" { // specimen date; DoH: lab report to the screening centre within 8 working days
			index = math.Max(0, sr-lognorm(8, .3))
		}
		if r.Float64() < .5 {
			encounter(p, src, math.Max(0, index-r.Float64()*30))
		}
		if prog == "crc" && r.Float64() < .2 {
			o := Providers[1]
			if src.ID == o.ID {
				o = Providers[0]
			}
			encounter(p, o, r.Float64()*TEnd)
		}
		srID := nextID(src.ID, "sr")
		loop := src.ID + "/ServiceRequest/" + srID
		srRes := map[string]interface{}{"resourceType": "ServiceRequest", "id": srID, "status": "active", "intent": "order", "authoredOn": dayToTS(sr)}
		if prog == "crc" {
			srRes["code"] = map[string]interface{}{"text": "Colonoscopy referral after positive FIT"}
			srRes["reasonCode"] = []interface{}{map[string]interface{}{"coding": []interface{}{map[string]string{"system": "http://loinc.org", "code": "29771-3", "display": "Hemoglobin.gastrointestinal.lower [Presence] in Stool by Immunoassay"}}, "text": "FIT positive"}}
		} else {
			srRes["code"] = map[string]interface{}{"text": "Colposcopy referral after abnormal cervical screening"}
			srRes["reasonCode"] = []interface{}{map[string]interface{}{"text": cat}}
			srRes["priority"] = map[string]string{"urgent": "urgent", "soon": "asap", "routine": "routine"}[prio]
			srRes["extension"] = []interface{}{map[string]string{"url": "urn:wsi:index-category", "valueString": cat},
				map[string]string{"url": "urn:wsi:specimen-date", "valueString": dayToTS(index)}}
		}
		emit(src.ID, sr+.02*r.Float64(), srRes, g)
		tl := langOf(p)
		exp := g.lang
		if exp == "" {
			exp = provDefaultLang[src.ID]
		}
		lt := &LoopTruth{Loop: loop, Qid: p.Qid, Cc: p.Cc, Origin: p.Origin, ArSrc: p.ArSrc, Prov: src.ID, Hosp: hosp.ID, SRDay: sr, ScriptSR: g.script,
			TrueLang: tl, RecLang: g.lang, ExpLang: exp, NotifDay: -1, AckDay: -1, RefDay: -1, RefStatus: "none",
			Prog: prog, Cat: cat, Priority: prio, IndexDay: index, Deadline: deadline, ProcDay: -1, EcState: "none", TBState: "none"}
		tr.Loops[loop] = lt
		// --- care-coordination Task with owner
		taskID := nextID(src.ID, "task")
		tv := 0
		task := func(day float64, status, owner string) {
			tv++
			m := map[string]interface{}{"resourceType": "Task", "id": taskID, "_v": tv, "status": status, "intent": "order",
				"focus": map[string]string{"reference": "ServiceRequest/" + srID}, "authoredOn": dayToTS(sr), "lastModified": dayToTS(day),
				"code": map[string]string{"text": fmt.Sprintf("Close %s loop: %s within %.0f days", prog, rule.Proc, deadline)}}
			if owner != "" {
				m["owner"] = map[string]string{"reference": owner}
			}
			emit(src.ID, day, m, nil)
		}
		owner := ""
		ownerDay := inf
		ox := r.Float64()
		nurse := fmt.Sprintf("Practitioner/%s-nurse-%d", src.ID, 1+r.Intn(6))
		if ox < .75 {
			owner, ownerDay = nurse, sr
		}
		task(sr+.03, "requested", owner)
		if ox >= .75 && ox < .85 {
			ownerDay = sr + 2 + 8*r.Float64()
			owner = nurse
			task(ownerDay, "accepted", owner)
		}
		lt.Unowned = !(ownerDay <= TEnd) && TEnd-sr > 3
		// --- communications (patient side + clinician e-consult thread), all on the referrer's stream keyed by the referral
		commN := 0
		comm := func(day float64, cat, tpl, lang, text, status, inResp string, ext map[string]string) string {
			commN++
			id := fmt.Sprintf("%s-comm-%d", srID, commN)
			m := map[string]interface{}{"resourceType": "Communication", "id": id, "status": status,
				"category": []interface{}{map[string]interface{}{"text": cat}}, "basedOn": []interface{}{map[string]string{"reference": "ServiceRequest/" + srID}},
				"payload": []interface{}{map[string]string{"contentString": text}}}
			if lang != "" {
				m["language"] = lang
			}
			inbound := cat == "acknowledgement" || cat == "teachback-reply" || cat == "econsult-reply" || cat == "prep-reply" || cat == "handoff-reply"
			if inbound {
				m["received"] = dayToTS(day)
				if inResp != "" {
					m["inResponseTo"] = []interface{}{map[string]string{"reference": "Communication/" + inResp}}
				}
				m["medium"] = []interface{}{map[string]string{"text": map[bool]string{true: "econsult", false: "sms-reply"}[cat == "econsult-reply" || cat == "prep-reply" || cat == "handoff-reply"]}}
			} else {
				m["sent"] = dayToTS(day)
				med := []string{"sms", "app"}[r.Intn(2)]
				if cat == "econsult-question" || cat == "prep-query" || cat == "prep-escalation" || strings.HasPrefix(cat, "handoff-") {
					med = "econsult"
				} else if cat == "callback" {
					med = "phone"
				}
				m["medium"] = []interface{}{map[string]string{"text": med}}
				if tpl != "" {
					m["extension"] = []interface{}{map[string]string{"url": "urn:wsi:template", "valueString": tpl}}
				}
				if status == "not-done" {
					m["statusReason"] = map[string]string{"text": "undeliverable"}
				}
			}
			ks := make([]string, 0, len(ext)) // sorted: map order must not leak into the cohort bytes
			for k := range ext {
				ks = append(ks, k)
			}
			sort.Strings(ks)
			for _, k := range ks {
				xs, _ := m["extension"].([]interface{})
				m["extension"] = append(xs, map[string]string{"url": k, "valueString": ext[k]})
			}
			emit(src.ID, day+.01, m, nil)
			return id
		}
		flip := map[string]string{"ar": "en", "en": "ar"}
		ackDay := inf
		var ackID string
		tryAck := func(id string, day float64, lang string, p0 float64) {
			if lang != tl {
				p0 /= 2
			}
			if r.Float64() < p0 {
				a := day + lognorm(1.2, .9)
				comm(a, "acknowledgement", "", "", "1", "completed", id, nil)
				if a <= TEnd && a < ackDay {
					ackDay = a
					ackID = id
				}
			}
		}
		notif := inf
		sendTpl := func(day float64, kind, lang string) (string, bool) {
			tid := rule.Tpl + "-" + kind + "-v1/" + lang
			txt, why := Render(tid, src.ID)
			if why != "" {
				panic(why)
			}
			if r.Float64() < .05 {
				return comm(day, "result-notification", tid, lang, txt, "not-done", "", nil), false
			}
			return comm(day, "result-notification", tid, lang, txt, "completed", "", nil), true
		}
		nx := r.Float64()
		switch {
		case nx < .07: // never notified
		case nx < .09: // staff free-text message that discloses the result (non-compliant)
			ds := disclosures
			if prog == "cervical" {
				ds = disclosuresCerv
			}
			d := ds[r.Intn(len(ds))]
			t := sr + lognorm(3, .8)
			comm(t, "result-notification", "", d.lang, d.text, "completed", "", nil)
			if t+.01 <= TEnd {
				lt.BadMsgs = append(lt.BadMsgs, "disclosure")
			}
			if r.Float64() < .5 {
				t2 := t + lognorm(8, .5)
				if id, ok := sendTpl(t2, "notify", exp); ok {
					notif = t2
					tryAck(id, t2, exp, .6)
				}
			}
		default:
			t := sr + lognorm(3, .9)
			lang := exp
			if r.Float64() < .025 {
				lang = flip[exp]
			}
			id, ok := sendTpl(t, "notify", lang)
			if !ok && r.Float64() < .6 {
				t += lognorm(5, .5)
				id, ok = sendTpl(t, "notify", lang)
			}
			if ok && lang != exp && t+.01 <= TEnd {
				lt.BadMsgs = append(lt.BadMsgs, "wrong_language")
			}
			if ok {
				tryAck(id, t, lang, .6)
				if lang == exp {
					notif = t
				} else if r.Float64() < .4 {
					t += lognorm(7, .5)
					if id2, ok2 := sendTpl(t, "notify", exp); ok2 {
						notif = t
						tryAck(id2, t, exp, .6)
					}
				}
			}
		}
		if notif < inf && ackDay > notif+7 && r.Float64() < .7 {
			t := notif + 7 + 3*r.Float64()
			if id, ok := sendTpl(t, "remind", exp); ok {
				tryAck(id, t, exp, .4)
			}
		}
		if notif <= TEnd {
			lt.NotifDay = notif
		}
		if ackDay <= TEnd {
			lt.AckDay = ackDay
		}
		// --- v6 teach-back: after the acknowledgement the patient is asked to restate the next step in their own words
		tbFinal := "none"     // comprehension after the teach-back exchange (drives booking)
		callback := false
		setTB := func(st string, d float64) {
			if d <= TEnd {
				lt.TBState = st
			}
		}
		if ackDay < inf && ackID != "" && r.Float64() < .92 {
			t := ackDay + .05 + .2*r.Float64()
			ptxt, _ := Render("tb-prompt-v1/"+exp, src.ID)
			pid := comm(t, "teachback-prompt", "tb-prompt-v1/"+exp, exp, ptxt, "completed", "", nil)
			setTB("prompted", t+.01)
			if r.Float64() < .82 {
				lab := drawLabel(r, TBL, []float64{.50, .20, .15, .15})
				x := pickTB(prog, lab, tl)
				t2 := t + lognorm(.4, .8)
				comm(t2, "teachback-reply", "", "", x.Text, "completed", pid, nil)
				setTB(lab, t2+.01)
				if lt.TBLatent == "" {
					lt.TBLatent = lab
				}
				tbFinal = lab
				switch lab {
				case "partial": // re-explain with the approved clarification template, then re-ask
					t3 := t2 + lognorm(1, .5)
					ctxt, _ := Render("tb-clarify-v1/"+exp, src.ID)
					cid := comm(t3, "teachback-prompt", "tb-clarify-v1/"+exp, exp, ctxt, "completed", "", nil)
					setTB("prompted", t3+.01)
					if r.Float64() < .75 {
						lab2 := drawLabel(r, TBL, []float64{.65, .20, 0, .15})
						x2 := pickTB(prog, lab2, tl)
						t4 := t3 + lognorm(.4, .8)
						comm(t4, "teachback-reply", "", "", x2.Text, "completed", cid, nil)
						setTB(lab2, t4+.01)
						tbFinal = lab2
					}
				}
				if tbFinal == "misunderstood" || tbFinal == "needs_human" { // nurse / navigator phone call
					if r.Float64() < .85 {
						t5 := t2 + lognorm(2, .5)
						comm(t5, "callback", "", exp, "", "completed", "", map[string]string{"urn:wsi:callback-reason": "teachback-" + tbFinal})
						callback = true
					}
				}
			}
		}
		// --- v6 clinician-to-clinician e-consult thread (rejected / unanswered referral, inadequate procedure)
		ecQ := func(day float64, kind string) {
			comm(day, "econsult-question", "", "en", ecQuestions[kind], "completed", "", map[string]string{"urn:wsi:econsult-trigger": kind, "urn:wsi:recipient": hosp.ID})
			if day+.01 <= TEnd {
				lt.EcState = "awaiting_reply"
			}
		}
		ecR := func(day float64, label string) {
			x := pickEC(prog, label)
			comm(day, "econsult-reply", "", "", x.Text, "completed", "", map[string]string{"urn:wsi:sender": hosp.ID})
			if day+.01 <= TEnd {
				lt.EcState = label
				lt.EcTurns++
			}
		}
		runEC := func(trig string, t0 float64) (string, float64) {
			if lt.EcTrigger == "" {
				lt.EcTrigger = trig
			}
			t := t0 + lognorm(1.5, .5)
			ecQ(t, trig)
			for turn := 0; turn < 3; turn++ {
				if r.Float64() < .08 {
					return "none", t // t: the last unanswered question
				}
				t += lognorm(2, .6)
				var lab string
				if turn == 0 {
					lab = drawLabel(r, ECL, []float64{.35, .20, .20, .15, .10})
				} else {
					lab = drawLabel(r, ECL, []float64{.75, 0, 0, .10, .15})
				}
				ecR(t, lab)
				if lab == "accept" || lab == "redirect" || lab == "reject" {
					return lab, t
				}
				t += lognorm(3, .5) // referrer meets the condition / sends the info
				ecQ(t, "followup")
			}
			return "none", t
		}
		// --- receiving unit response to the referral (hospital intake ServiceRequest, linked by identity only)
		respDay := inf
		rejectedFinal, unanswered := false, false
		rejDay := inf // day the referral was finally rejected (hand-off packet)
		lastResp, lastRespDay := "none", -1.0
		setResp := func(st string, d float64) {
			if d <= TEnd {
				if lt.RefDay < 0 {
					lt.RefDay = d
				}
				if d >= lastRespDay {
					lastResp, lastRespDay = st, d
				}
			}
		}
		afterEC := func(o string, t float64, failState *bool) {
			switch o {
			case "accept":
				respDay = t + lognorm(.5, .5)
				intake(p, hosp, respDay, "active", "", loop, prog)
				setResp("accepted", respDay+.02)
			case "redirect":
				hosp = otherFor(prog, hosp)
				lt.Hosp = hosp.ID
				respDay = t + lognorm(3, .5)
				if r.Float64() < .9 {
					intake(p, hosp, respDay, "active", "", loop, prog)
					setResp("accepted", respDay+.02)
				} else {
					intake(p, hosp, respDay, "revoked", rejectReasons[r.Intn(len(rejectReasons))], loop, prog)
					setResp("rejected", respDay+.02)
					*failState = true
					rejDay = respDay
				}
			default:
				*failState = true
				rejDay = t
				if o == "none" { // rejected referral, e-consult never answered: the packet goes after 5 business days
					rejDay = addBD(t, HandoffBDRoutine)
				}
			}
		}
		rx := r.Float64()
		switch {
		case rx < .05:
			o, t := runEC("unanswered", sr+RefRespSLA)
			afterEC(o, t, &unanswered)
		case rx < .13:
			respDay = sr + lognorm(2.5, .6)
			intake(p, hosp, respDay, "revoked", rejectReasons[r.Intn(len(rejectReasons))], loop, prog)
			setResp("rejected", respDay+.02)
			o, t := runEC("rejected", respDay)
			afterEC(o, t, &rejectedFinal)
		default:
			respDay = sr + lognorm(2.5, .6)
			intake(p, hosp, respDay, "active", "", loop, prog)
			setResp("accepted", respDay+.02)
		}
		if unanswered && lastResp == "none" {
			lt.RefStatus = "none"
		}
		lt.RefStatus = lastResp
		// --- booking depends on the communication chain (v6: on the teach-back outcome) [simulated parameters]
		pBook := .9
		switch {
		case rejectedFinal || unanswered:
			pBook = 0
		case notif == inf:
			pBook = .15
		case ackDay == inf:
			pBook = .4
		case tbFinal == "understood":
			pBook = .92
		case tbFinal == "partial":
			pBook = .6
		case tbFinal == "misunderstood" && callback:
			pBook = .55
		case tbFinal == "misunderstood":
			pBook = .2
		case tbFinal == "needs_human" && callback:
			pBook = .6
		case tbFinal == "needs_human":
			pBook = .25
		}
		procGap := func() float64 {
			if prog == "cervical" {
				switch prio {
				case "urgent":
					return lognorm(7, .4)
				case "soon":
					return lognorm(14, .4)
				}
				return lognorm(25, .5)
			}
			return lognorm(14, .55)
		}
		var cs []*colo
		pw := ""
		var finalize func(cs []*colo, pw string) // defined below; a deferred hand-off calls it again
		if r.Float64() < pBook {
			start := respDay
			if notif < inf && notif > start {
				start = notif
			}
			if ackDay < inf && ackDay > start {
				start = ackDay
			}
			book := start + lognorm(4, .6)
			day := book + procGap()
			if r.Float64() < .14 {
				cs = append(cs, doProc(p, prog, hosp, book, day, "noshow", false, loop))
				pw = "no_show"
				if r.Float64() < .3 {
					b2 := day + 3 + 10*r.Float64()
					cs = append(cs, doProc(p, prog, hosp, b2, b2+procGap(), "fulfilled", r.Float64() < .85, loop))
					pw = "no_show_rebooked"
				}
			} else {
				c := doProc(p, prog, hosp, book, day, "fulfilled", r.Float64() < .82, loop)
				cs = append(cs, c)
				pw = "completed_" + c.truth
				if c.truth == "inadequate" && c.reportArr < inf {
					o, t := runEC("inadequate", c.reportArr)
					noRepeat := false
					switch o {
					case "accept":
					case "redirect":
						hosp = otherFor(prog, hosp)
						lt.Hosp = hosp.ID
					default:
						noRepeat = true
					}
					if !noRepeat && r.Float64() < .9 {
						b2 := t + 2 + 8*r.Float64()
						cs = append(cs, doProc(p, prog, hosp, b2, b2+procGap(), "fulfilled", true, loop))
						pw = "inadequate_repeated"
					}
				}
			}
		} else if Handoff && rejectedFinal && rejDay < TEnd {
			// hand-off packet to the referrer (handoff.go). Simulated after every other loop and on its own random
			// stream, so the rest of the cohort is byte-identical with or without it.
			pw = "referral_rejected_handoff_no_action"
			urgent := handoffUrgent(prog, prio)
			actBy, medDir, act := handoffPlan(seed, loop, rejDay, urgent)
			lt.Handoff = &HandoffTruth{Decided: rejDay, ActBy: actBy, ActDay: -1, Urgent: urgent}
			handoffJobs = append(handoffJobs, func() {
				saved := r
				r = rand.New(rand.NewSource(seed ^ int64(fnv32(loop+"|handoff"))))
				var cs []*colo
				pw := "referral_rejected_handoff_no_action"
				to := map[string]string{"urn:wsi:recipient": src.ID}
				comm(rejDay+.05, "handoff-packet", "", "en", "Referral "+srID+" was declined by "+hosp.ID+". You are responsible for the next step. Act by "+dayToTS(actBy)+".", "completed", "", to)
				if act < 0 || act > actBy {
					comm(actBy, "handoff-coordinator", "", "en", "Care coordinator: declined referral "+srID+" has no action from the referrer.", "completed", "", to)
				}
				if act < 0 || act > medDir {
					comm(medDir, "handoff-medical-director", "", "en", "Medical director: declined referral "+srID+" is still without action.", "completed", "", to)
				}
				if act >= 0 && act <= TEnd {
					lt.Handoff.ActDay = act
					nh := otherFor(prog, hosp)
					comm(act, "handoff-reply", "", "en", "Re-referred to "+nh.ID+".", "completed", "", map[string]string{"urn:wsi:handoff-action": "re_referred", "urn:wsi:sender": src.ID})
					hosp = nh
					lt.Hosp = hosp.ID
					rd := act + lognorm(2.5, .6)
					if r.Float64() < .9 {
						intake(p, hosp, rd, "active", "", loop, prog)
						setResp("accepted", rd+.02)
						pw = "handoff_rereferred_not_booked"
						if r.Float64() < .85 {
							book := rd + lognorm(4, .6)
							c := doProc(p, prog, hosp, book, book+procGap(), "fulfilled", r.Float64() < .82, loop)
							cs = append(cs, c)
							pw = "handoff_rereferred_" + c.truth
						}
					} else {
						intake(p, hosp, rd, "revoked", rejectReasons[r.Intn(len(rejectReasons))], loop, prog)
						setResp("rejected", rd+.02)
						pw = "handoff_rereferred_rejected"
					}
					lt.RefStatus = lastResp
				}
				r = saved
				finalize(cs, pw)
			})
		} else {
			switch {
			case rejectedFinal:
				pw = "referral_rejected"
			case unanswered:
				pw = "referral_unanswered"
			case notif == inf:
				pw = "never_notified_not_booked"
			case ackDay == inf:
				pw = "no_ack_not_booked"
			default:
				pw = "ack_not_booked"
			}
		}
		// finalize: the loop's truth from its procedures (a deferred hand-off re-runs it with the re-referral's procedures)
		finalize = func(cs []*colo, pw string) {
	lt.Pathway = pw
			if len(cs) > 0 {
				lt.ScriptHosp = cs[0].g.script
			}
			for _, c := range cs {
				if c.reportID != "" && c.reportArr <= TEnd {
					lt.Reports = append(lt.Reports, tr.ReportUID[c.reportID])
					if PrepQuery && c.prepUngraded { // ungraded prep: unit-lead query thread (prepquery.go), own hashed stream
						for _, pm := range prepThread(seed, c.reportID, c.reportArr+.1, c.truth != "inadequate") {
							if pm.Day > TEnd {
								continue
							}
							text := map[string]string{"prep-query": "Please grade the bowel preparation (Boston or Aronchick) for colonoscopy " + c.reportID + ".",
								"prep-escalation": "Escalation: bowel-preparation grade for colonoscopy " + c.reportID + " not received within 5 working days.",
								"prep-reply": "Bowel preparation for " + c.reportID + ": " + strings.ReplaceAll(pm.Grade, "_", " ") + "."}[pm.Cat]
							ext := map[string]string{"urn:wsi:report": c.reportID}
							if pm.Cat == "prep-reply" {
								ext["urn:wsi:prep-grade"] = pm.Grade
								ext["urn:wsi:sender"] = c.hosp
							} else {
								ext["urn:wsi:recipient"] = c.hosp
							}
							comm(pm.Day, pm.Cat, "", "en", text, "completed", "", ext)
						}
					}
				}
				if c.status == "fulfilled" && c.fulArr <= TEnd && (lt.ProcDay < 0 || c.day < lt.ProcDay) {
					lt.ProcDay = c.day
				}
			}
			closed := inf
			awaiting, inadequate := false, false
			lastAppt, lastApptDay := "", -1.0
			for _, c := range cs {
				if c.reportID != "" && c.reportArr <= TEnd {
					switch c.truth {
					case "adequate":
						closed = math.Min(closed, c.day)
					case "awaiting_histology":
						if c.histArr <= TEnd {
							closed = math.Min(closed, c.day)
						} else {
							awaiting = true
						}
					case "inadequate":
						inadequate = true
					}
				}
				st := ""
				if c.fulArr <= TEnd {
					st = c.status
				} else if c.bookArr <= TEnd {
					st = "booked"
				}
				if st != "" && c.day >= lastApptDay {
					lastAppt, lastApptDay = st, c.day
				}
			}
			state := "open"
			switch {
			case closed < inf:
				state = "closed"
				lt.ClosedDay = closed
			case awaiting:
				state = "awaiting_histology"
			case inadequate:
				state = "inadequate"
			case lastAppt == "fulfilled":
				state = "procedure_done"
			case lastAppt == "noshow":
				state = "no_show"
			case lastAppt == "booked":
				state = "booked"
			case lastResp == "rejected":
				state = "referral_rejected"
			}
			lt.State = state
			lt.Overdue = state != "closed" && TEnd-sr > deadline
			lt.Dropoff = dropoffCode(state, false, lastResp, lt.NotifDay >= 0, lt.AckDay >= 0, lt.RefDay >= 0)
		}
		finalize(cs, pw)
	}
	// persons: cervical loops use women; CRC loops, distractors and background use the rest
	used := map[int]bool{}
	nc := 0
	for i := range ps {
		if nc >= nCerv {
			break
		}
		if ps[i].Sex == "F" {
			used[i] = true
			genLoop(&ps[i], "cervical")
			nc++
		}
	}
	// CRC loops: CRCFemaleShare women (SEHA FIT+ cohort). Women are drawn from every woman in the sample, so some also
	// hold a cervical loop (same registrations, other programme). If the split has too few women, the rest are men.
	women, others := []int{}, []int{}
	for i := range ps {
		if ps[i].Sex == "F" {
			women = append(women, i)
		} else if !used[i] {
			others = append(others, i)
		}
	}
	r.Shuffle(len(women), func(i, j int) { women[i], women[j] = women[j], women[i] }) // cervical took the first women
	nF := int(math.Round(CRCFemaleShare * float64(nLoops)))
	if nF > len(women) {
		nF = len(women)
	}
	for _, i := range women[nF:] { // small samples: top up with the remaining unused women if men run out
		if !used[i] && len(others) < nLoops-nF {
			others = append(others, i)
		}
	}
	if nLoops-nF > len(others) {
		panic("not enough persons for CRC loops")
	}
	crc := append(append([]int{}, women[:nF]...), others[:nLoops-nF]...)
	r.Shuffle(len(crc), func(i, j int) { crc[i], crc[j] = crc[j], crc[i] })
	crcBoth := 0
	for _, i := range crc {
		if used[i] {
			crcBoth++
		}
		genLoop(&ps[i], "crc")
	}
	for _, i := range crc {
		used[i] = true
	}
	rest := []int{}
	for i := range ps {
		if !used[i] {
			rest = append(rest, i)
		}
	}
	// ---------- distractors: diagnostic colonoscopies for other people at the same hospitals ----------
	allRep := append(append([]RepSrc{}, poolOK...), poolBad...)
	for k := 0; k < nDistract && k < len(rest); k++ {
		p := &ps[rest[k]]
		hosp := Providers[2+r.Intn(2)]
		bk := r.Float64() * 110
		intake(p, hosp, math.Max(0, bk-2-3*r.Float64()), "active", "", "", "crc")
		st := "fulfilled"
		if r.Float64() < .1 {
			st = "noshow"
		}
		c := doColo(p, hosp, bk, bk+lognorm(10, .5), st, true, "", allRep)
		if c.reportID != "" {
			tr.Distract[c.reportID] = true
		}
		if r.Float64() < .3 {
			encounter(p, Providers[r.Intn(2)], r.Float64()*TEnd)
		}
	}
	// ---------- background primary-care / lab encounters ----------
	for k := nDistract; k < len(rest); k++ {
		p := &ps[rest[k]]
		prov := Providers[r.Intn(2)]
		for k := 0; k < 1+r.Intn(2); k++ {
			encounter(p, prov, r.Float64()*TEnd)
		}
	}
	if nLung > 0 { // incidental lung-nodule loops (genlung.go): own RNG stream, background persons only
		bg := []*Person{}
		for k := nDistract; k < len(rest); k++ {
			bg = append(bg, &ps[rest[k]])
		}
		genLung(lungDirFor(lungDir, reportsPath), seed, nLung, bg, tr, &evs, regs, mrnN, ctr, textUse)
	}
	for _, job := range handoffJobs {
		job()
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].Arrival < evs[j].Arrival })
	os.MkdirAll(outDir, 0755)
	byProv := map[string]*os.File{}
	cnt := map[string]int{}
	for _, pc := range Providers {
		f, _ := os.Create(filepath.Join(outDir, "fhir_"+pc.ID+".ndjson"))
		byProv[pc.ID] = f
	}
	for _, e := range evs {
		bb, _ := json.Marshal(e)
		byProv[e.Prov].Write(append(bb, '\n'))
		cnt[e.Prov+"/"+e.Res["resourceType"].(string)]++
	}
	for _, f := range byProv {
		f.Close()
	}
	wj := func(name string, v interface{}) {
		bb, _ := json.MarshalIndent(v, "", " ")
		os.WriteFile(filepath.Join(outDir, name), bb, 0644)
	}
	wj("truth.json", tr)
	states, paths2, drops, origins, langs := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	for _, l := range tr.Loops {
		states[l.State]++
		paths2[l.Pathway]++
		drops[l.Dropoff]++
		origins[l.Origin]++
		langs[l.TrueLang+"/exp:"+l.ExpLang]++
	}
	uniq := 0
	for k := range textUse {
		if strings.HasPrefix(k, "u") {
			uniq++
		}
	}
	summary := map[string]interface{}{"events": len(evs), "by_provider_type": cnt, "loops": len(tr.Loops), "truth_states": states,
		"pathways": paths2, "dropoff_truth": drops, "loop_origins": origins, "lang_true_vs_expected": langs, "registrations": len(regs),
		"reports_in_window": len(tr.Reports), "text_use": textUse, "unique_mtsamples_texts_used": uniq, "persons_used": len(ps),
		"crc_female_share": crcFemale(ps, crc), "crc_women_with_cervical_loop": crcBoth}
	wj("cohort_summary.json", summary)
	bb, _ := json.Marshal(summary)
	fmt.Println(string(bb))
}

// dropoffCode: the step at which a not-yet-closed loop is stuck (same function for truth and pipeline).
func dropoffCode(state string, unlinked bool, ref string, notified, acked, responded bool) string {
	switch state {
	case "closed":
		return ""
	case "awaiting_histology":
		return "awaiting_histology"
	case "inadequate":
		return "inadequate"
	case "no_show":
		return "no_show"
	case "booked", "procedure_done":
		return "in_progress"
	}
	if unlinked {
		return "unlinked"
	}
	switch {
	case ref == "rejected":
		return "referral_rejected"
	case !notified:
		return "never_notified"
	case !acked:
		return "no_acknowledgement"
	case !responded:
		return "referral_unanswered"
	}
	return "not_booked"
}
