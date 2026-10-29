package main

// Shared message types. Everything that flows between processes is JSON on NATS JetStream.
import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash/fnv"
	"time"
)

var baseTime = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

func dayToTS(d float64) string {
	return baseTime.Add(time.Duration(d * 86400 * float64(time.Second))).Format(time.RFC3339)
}
func tsToDay(s string) float64 {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0
	}
	return t.Sub(baseTime).Seconds() / 86400
}

// Event is the normalised view of one FHIR resource (built by the normaliser/embed stage).
type Event struct {
	ID         string  `json:"id"`
	Provider   string  `json:"provider"`
	PatientKey string  `json:"pk"` // provider|local MRN
	RType      string  `json:"rtype"`
	Status     string  `json:"status,omitempty"`
	Day        float64 `json:"day"`
	Name       string  `json:"name"`
	Dob        string  `json:"dob,omitempty"`
	Sex        string  `json:"sex,omitempty"`
	Eid        string  `json:"eid,omitempty"`
	Focus      string  `json:"focus,omitempty"` // Task.focus -> ServiceRequest id
	Owner      string  `json:"owner,omitempty"`
	ReportID   string  `json:"report,omitempty"`
	Text       string  `json:"text,omitempty"`
	PrefLang   string  `json:"lang,omitempty"`  // Patient.communication preferred language at this provider
	Category   string  `json:"cat,omitempty"`   // Communication.category | DiagnosticReport kind (colonoscopy|pathology)
	Template   string  `json:"tpl,omitempty"`   // Communication template id (extension)
	CommLang   string  `json:"clang,omitempty"` // Communication.language
	Channel    string  `json:"chan,omitempty"`
	Reason     string  `json:"reason,omitempty"` // Task.statusReason (referral response)
	TextSha    string  `json:"tsha,omitempty"`
	Ev         *TwoVerdict `json:"evv,omitempty"` // two-reader evidence verdict, computed in the stateless prep stage
	// v6
	Prog     string  `json:"prog,omitempty"`     // crc|cervical (ServiceRequest code, Appointment serviceType, report kind)
	Priority string  `json:"prio,omitempty"`     // cervical App. 5 priority from the index cytology/HPV category
	Cat      string  `json:"icat,omitempty"`     // index test category (HSIL+|LSIL|ASC-H|ASC-US|HPV+NILM|FIT+)
	IndexDay float64 `json:"iday,omitempty"`     // specimen date (cervical 21-day notification clock)
	Thread   string  `json:"thread,omitempty"`   // e-consult thread = loop id (clinician-to-clinician, keyed by referral, no identity linking)
	Parse    string  `json:"parse,omitempty"`    // LLM parse of an inbound free-text reply (e-consult or teach-back)
	Coded    string  `json:"coded,omitempty"`    // coded conclusion on a colposcopy report (second reader)
	Images   []ImgRef `json:"imgs,omitempty"`    // DiagnosticReport.media photo references (sha256 + capture station)
	Lung     *LungSR  `json:"lung,omitempty"`    // lung-nodule tracking ServiceRequest: age, exclusion, risk, index report (lung.go)
}

// CommFact: one Communication on a loop (outbound result notification or inbound patient acknowledgement).
type CommFact struct {
	ID       string  `json:"id"`
	Day      float64 `json:"day"`
	Category string  `json:"cat"`
	Template string  `json:"tpl,omitempty"`
	Lang     string  `json:"lang,omitempty"`
	Channel  string  `json:"chan,omitempty"`
	Text     string  `json:"text,omitempty"`
	Status   string  `json:"status"`
	Parse    string  `json:"parse,omitempty"`
	Sha      string  `json:"tsha,omitempty"`
}

// Signoff is a human reviewer decision on a unified review item (published as a FHIR Provenance).
type Signoff struct {
	ItemID     string    `json:"item"`
	LoopID     string    `json:"loop"`
	Kinds      []string  `json:"kinds"`
	Pair       [2]string `json:"pair,omitempty"`
	ReportID   string    `json:"report,omitempty"`
	Link       string    `json:"link,omitempty"`     // same|different
	Evidence   string    `json:"evidence,omitempty"` // adequate|inadequate
	Reviewer   string    `json:"reviewer"`
	SignedHead string    `json:"signed_head"`
	Run        string    `json:"run"`
	// v6 governance fields (omitempty keeps v5 sign-offs byte-identical)
	Role      string   `json:"role,omitempty"`      // RBAC role of the signing actor
	Reads     []string `json:"reads,omitempty"`     // distinct reviewer ids that concurred (double review)
	DurationS float64  `json:"dur_s,omitempty"`     // simulated reviewer seconds spent on this item (all reads)
	Purpose   string   `json:"purpose,omitempty"`   // v7: second_identifier = messaging confirmation only (never cuts a cluster)

	audit *auditRec // H7 senior audit (governance.go); unexported, so never serialised
}

// Prep is the output of the stateless normalise+embed stage; keyed by FHIR stream sequence.
type Prep struct {
	Seq      uint64    `json:"seq"`
	Skip     bool      `json:"skip,omitempty"`
	Kind     string    `json:"kind"` // event|tick|signoff
	Ev       *Event    `json:"ev,omitempty"`
	Tick     float64   `json:"tick,omitempty"`
	So       *Signoff  `json:"so,omitempty"`
	Emb      []float32 `json:"emb,omitempty"`
	OriginTs int64     `json:"ots"`
}

type Edge struct {
	A     string  `json:"a"`
	B     string  `json:"b"`
	P     float64 `json:"p"`
	Basis string  `json:"basis"` // wasl|eid|human
}


type TaskFact struct {
	Day    float64 `json:"day"`
	Status string  `json:"status"`
	Owner  string  `json:"owner,omitempty"`
}

type DownFact struct {
	EventID  string  `json:"ev"`
	Provider string  `json:"prov"`
	RType    string  `json:"rtype"`
	Status   string  `json:"status,omitempty"`
	Day      float64 `json:"day"`
	ReportID string  `json:"report,omitempty"`
	Reason   string  `json:"reason,omitempty"`
	Parent   string  `json:"parent,omitempty"` // pathology addendum -> colonoscopy report id
	Prog     string  `json:"prog,omitempty"`
	Ev       *TwoVerdict `json:"evv,omitempty"`
	Strength float64 `json:"strength"`
	Weak     Edge    `json:"weak"`
	Hops     int     `json:"hops"`
	NearMiss bool    `json:"near_miss,omitempty"` // linked only by a sub-threshold score (possible missed link)
	// v7 (set only when V7Cal / V7Guard are on; omitempty keeps v6 facts byte-identical)
	PCal     float64 `json:"pcal7,omitempty"`    // min calibrated edge probability along the widest path (eid/human edges = 1)
	AllHuman bool    `json:"all_human,omitempty"` // every edge on the path is a human-confirmed link
	RecPk    string  `json:"rec_pk,omitempty"`    // downstream registration (endpoint of the path)
	RecDob   string  `json:"rec_dob,omitempty"`
}

// LoopFacts: everything the linker knows about one FIT+ loop, recomputed whenever its patient cluster changes.
type LoopFacts struct {
	LoopID   string              `json:"loop"`
	InSeq    uint64              `json:"in_seq"`
	Provider string              `json:"prov"`
	SRDay    float64             `json:"sr_day"`
	SRRec    string              `json:"sr_rec"`
	SRLang   string              `json:"sr_lang,omitempty"`
	Prog     string              `json:"prog,omitempty"`
	Priority string              `json:"prio,omitempty"`
	Cat      string              `json:"icat,omitempty"`
	IndexDay float64             `json:"iday,omitempty"`
	Comms    []CommFact          `json:"comms,omitempty"`
	Tasks    []TaskFact          `json:"tasks"`
	Down     []DownFact          `json:"down"`
	Signoffs []Signoff           `json:"signoffs,omitempty"`
	EvOver   map[string]string   `json:"ev_over,omitempty"` // reportID -> human evidence label
	MsgOptOut bool               `json:"msg_opt_out,omitempty"` // v6: referral patient withheld consent to automated outreach
	Contact   string             `json:"contact,omitempty"`     // stage 3: contact permission at fact time: unknown|granted|denied|opted_out
	SRDob     string             `json:"sr_dob,omitempty"`      // v7: referral registration DOB (second-identifier guard)
	Lung      *LungSR            `json:"lung,omitempty"`        // lung-nodule loop: scope and risk from the tracking ServiceRequest
}

// FactsMsg goes linker -> closure partition. Either facts for one loop or a clock tick.
type FactsMsg struct {
	ID       string     `json:"id"`
	InSeq    uint64     `json:"in_seq"`
	Facts    *LoopFacts `json:"facts,omitempty"`
	TickDay  float64    `json:"tick,omitempty"`
	IsTick   bool       `json:"is_tick,omitempty"`
	Part     int        `json:"part"`
	PSeq     uint64     `json:"pseq"` // deterministic per-partition sequence assigned by the linker core
	OriginTs int64      `json:"ots"`
}

type Block struct {
	Type string                 `json:"type"`
	Body map[string]interface{} `json:"body"`
	Prev string                 `json:"prev"`
	Hash string                 `json:"hash"`
}

type ReviewItem struct {
	ItemID   string    `json:"item"`
	LoopID   string    `json:"loop"`
	Kinds    []string  `json:"kinds"`
	Pair     [2]string `json:"pair"`
	P        float64   `json:"p"`
	ReportID string    `json:"report"`
	Verdict  string    `json:"verdict"`
	Conf     float64   `json:"conf"`
	Quote    string    `json:"quote"`
	Chain    []Block   `json:"chain"`
	Head     string    `json:"head"`
	InSeq    uint64    `json:"in_seq"`
	OverdueAtRaise bool `json:"overdue"`
	Reason   string    `json:"reason,omitempty"` // v7: second_identifier (outreach guard confirmation)
}

type Decision struct {
	ID             string   `json:"id"`
	LoopID         string   `json:"loop"`
	InSeq          uint64   `json:"in_seq"`
	Provider       string   `json:"prov"`
	SRDay          float64  `json:"sr_day"`
	Now            float64  `json:"now"`
	State          string   `json:"state"`
	Overdue        bool     `json:"overdue"`
	Unowned        bool     `json:"unowned"`
	Owner          string   `json:"owner,omitempty"`
	Escalations    []string `json:"esc,omitempty"`
	ReportedClosed bool     `json:"reported_closed"`
	ClosedDay      float64  `json:"closed_day,omitempty"`
	ClosingReport  string   `json:"closing_report,omitempty"`
	Pending        []string `json:"pending,omitempty"`
	NotifDay       float64  `json:"notif_day"` // -1 = none
	AckDay         float64  `json:"ack_day"`
	RefStatus      string   `json:"ref_status"`
	RefDay         float64  `json:"ref_day"`
	CommFlags      []string `json:"comm_flags,omitempty"`
	Dropoff        string   `json:"dropoff"`
	HumanAssisted  bool     `json:"human_assisted,omitempty"`
	Prog           string   `json:"prog,omitempty"`
	Priority       string   `json:"prio,omitempty"`
	Deadline       float64  `json:"deadline,omitempty"`
	ProcDay        float64  `json:"proc_day"` // first trusted fulfilled procedure for this programme; -1 none
	EcState        string   `json:"ec_state,omitempty"`  // none|awaiting_reply|accept|conditional|needs_info|redirect|reject|unparsed
	EcTurns        int      `json:"ec_turns,omitempty"`
	TBState        string   `json:"tb_state,omitempty"`  // none|prompted|understood|partial|misunderstood|needs_human|unparsed
	Chain          []Block  `json:"chain,omitempty"`
	Policy         string   `json:"policy,omitempty"` // gate policy that produced this decision (v6 | v7strict+...)
	ConsentLog     []ConsentTransition `json:"consent_log,omitempty"` // stage 3: hold/release/cancel transitions at this decision
	Contact        string   `json:"contact,omitempty"` // contact permission of the referral patient at this decision
	Sig            string   `json:"sig"`
	Outreach       []Outreach `json:"outreach,omitempty"` // v6: patient-facing outreach intents (sent or suppressed)
	Lung           *LungDecision `json:"lung,omitempty"` // lung-nodule loop: Fleischner recommendation, window, outcome (lung.go)
}

// Outreach is one patient-facing message the closure layer would trigger from a downstream (cross-provider) fact.
// It is only released if the link that carried the fact is trusted (p_cal>=tau | EID | human-confirmed), the patient has
// not opted out, and a coordinator dispatches it (RBAC).
type Outreach struct {
	Kind     string  `json:"kind"`  // appointment_reminder|rebook_after_noshow|referral_rejected|repeat_colonoscopy
	Event    string  `json:"ev"`    // downstream event id that triggered it
	Basis    string  `json:"basis"` // trusted link basis: p_cal|eid|human|below_tau
	PCal     float64 `json:"p_cal"`
	Released bool    `json:"released"`
	Suppress string  `json:"suppress,omitempty"` // link_below_tau_unconfirmed|consent_withheld|second_identifier_unconfirmed
	Guard    string  `json:"guard,omitempty"`    // v7: dob_match|human_path|held
	Pair     [2]string `json:"pair,omitempty"`   // v7: referral registration, downstream registration
}

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func canon(v interface{}) []byte { b, _ := json.Marshal(v); return b } // encoding/json sorts map keys

func partOf(loop string, P int) int {
	h := fnv.New32a()
	h.Write([]byte(loop))
	return int(h.Sum32() % uint32(P))
}

// appendBlock hash-links a provenance block to the chain.
func appendBlock(chain []Block, typ string, body map[string]interface{}) []Block {
	prev := "genesis"
	if len(chain) > 0 {
		prev = chain[len(chain)-1].Hash
	}
	b := Block{Type: typ, Body: body, Prev: prev}
	b.Hash = sha(append([]byte(prev+"|"+typ+"|"), canon(body)...))
	return append(append([]Block{}, chain...), b)
}

func verifyChain(chain []Block) bool {
	prev := "genesis"
	for _, b := range chain {
		if b.Prev != prev || b.Hash != sha(append([]byte(prev+"|"+b.Type+"|"), canon(b.Body)...)) {
			return false
		}
		prev = b.Hash
	}
	return true
}

// ConsentTransition records an outreach intent entering or leaving the consent hold (stage 3 audit trail).
type ConsentTransition struct {
	Event   string `json:"ev"`
	Kind    string `json:"kind"`
	From    string `json:"from,omitempty"` // "" = first seen
	To      string `json:"to"`             // held | released | cancelled | other
	Contact string `json:"contact"`        // contact permission at decision time
}
