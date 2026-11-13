package main

// Shared message types. Everything that flows between processes is JSON on NATS JetStream.
import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"
	"time"
)

var baseTime = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

func dayToTS(d float64) string {
	return baseTime.Add(time.Duration(d * 86400 * float64(time.Second))).Format(time.RFC3339)
}

// tsToDay parses the Wasl clinical timestamp profile: RFC3339 dateTime or a
// partial FHIR date/dateTime (YYYY, YYYY-MM, YYYY-MM-DD). Partial values are
// anchored at midnight UTC. A real day 0 is valid; a parse failure is an error.
func parseClinicalTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("timestamp is required")
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02", "2006-01", "2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid FHIR date/dateTime %q", s)
}

func tsToDay(s string) (float64, error) {
	t, err := parseClinicalTime(s)
	if err != nil {
		return 0, err
	}
	return t.Sub(baseTime).Seconds() / 86400, nil
}

// NormalizationError is persisted with a skipped PREP record so live ingest
// and materialized replay retain the same structured quarantine decision.
type NormalizationError struct {
	Code         string `json:"code"`
	ResourceType string `json:"resource_type,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	Field        string `json:"field,omitempty"`
	Detail       string `json:"detail,omitempty"`
}

func (e *NormalizationError) Error() string {
	parts := []string{e.Code}
	if e.ResourceType != "" {
		parts = append(parts, e.ResourceType+"/"+e.ResourceID)
	}
	if e.Field != "" {
		parts = append(parts, e.Field)
	}
	if e.Detail != "" {
		parts = append(parts, e.Detail)
	}
	return strings.Join(parts, ": ")
}

func normalizationError(rt, id, code, field, detail string) *NormalizationError {
	return &NormalizationError{Code: code, ResourceType: rt, ResourceID: id, Field: field, Detail: detail}
}

// Event is the normalised view of one FHIR resource (built by the normaliser/embed stage).
type Event struct {
	ID            string      `json:"id"`
	Provider      string      `json:"provider"`
	PatientKey    string      `json:"pk"` // provider|local MRN
	RType         string      `json:"rtype"`
	Status        string      `json:"status,omitempty"`
	ConsentStatus string      `json:"consent_status,omitempty"` // Consent lifecycle; Status remains provision.type
	Version       int         `json:"ver,omitempty"`            // FHIR meta.versionId; DiagnosticReport, Communication and Consent revision/withdrawal ordering
	Day           float64     `json:"day"`
	Name          string      `json:"name"`
	Dob           string      `json:"dob,omitempty"`
	Sex           string      `json:"sex,omitempty"`
	Eid           string      `json:"eid,omitempty"`
	Focus         string      `json:"focus,omitempty"` // Task.focus -> ServiceRequest id
	Owner         string      `json:"owner,omitempty"`
	ReportID      string      `json:"report,omitempty"`
	ReportVersion int         `json:"report_version,omitempty"` // Communication prep thread -> DiagnosticReport version; zero/one share the legacy key
	Text          string      `json:"text,omitempty"`
	PrefLang      string      `json:"lang,omitempty"`      // Patient.communication preferred language at this provider
	Category      string      `json:"cat,omitempty"`       // Communication.category | DiagnosticReport kind (colonoscopy|pathology)
	FollowUp      string      `json:"follow_up,omitempty"` // typed CRC downstream care, independent of referral-intake classification
	Template      string      `json:"tpl,omitempty"`       // Communication template id (extension)
	CommLang      string      `json:"clang,omitempty"`     // Communication.language
	Channel       string      `json:"chan,omitempty"`
	Reason        string      `json:"reason,omitempty"` // Task.statusReason (referral response)
	Recipient     string      `json:"rcpt,omitempty"`   // Communication recipient unit (e-consult question: the unit asked)
	TextSha       string      `json:"tsha,omitempty"`
	Ev            *TwoVerdict `json:"evv,omitempty"` // two-reader evidence verdict, computed in the stateless prep stage
	// v6
	Prog     string    `json:"prog,omitempty"`   // crc|cervical (ServiceRequest code, Appointment serviceType, report kind)
	Priority string    `json:"prio,omitempty"`   // cervical App. 5 priority from the index cytology/HPV category
	Cat      string    `json:"icat,omitempty"`   // index test category (HSIL+|LSIL|ASC-H|ASC-US|HPV+NILM|FIT+)
	IndexDay float64   `json:"iday,omitempty"`   // specimen date (cervical 21-day notification clock)
	Thread   string    `json:"thread,omitempty"` // e-consult thread = loop id (clinician-to-clinician, keyed by referral, no identity linking)
	Parse    string    `json:"parse,omitempty"`  // LLM parse of an inbound free-text reply (e-consult or teach-back)
	Coded    string    `json:"coded,omitempty"`  // coded conclusion on a colposcopy report (second reader)
	Images   []ImgRef  `json:"imgs,omitempty"`   // DiagnosticReport.media photo references (sha256 + capture station)
	Lung     *LungSR   `json:"lung,omitempty"`   // lung-nodule tracking ServiceRequest: age, exclusion, risk, index report (lung.go)
	DR       *DRSR     `json:"dr,omitempty"`     // diabetic retinopathy tracking ServiceRequest: first screening report (dr.go)
	XRef     string    `json:"xref,omitempty"`   // referral intake: the referral it answers (basedOn "external referral <loop>"; handoffref.go)
	Breast   *BreastSR `json:"breast,omitempty"` // breast recall tracking ServiceRequest: first screening mammogram report (breast.go)
	LDCT     *LDCTSR   `json:"ldct,omitempty"`   // lung cancer screening tracking ServiceRequest: the LDCT report (ldct.go)
}

// CommFact: one Communication on a loop (outbound result notification or inbound patient acknowledgement).
type CommFact struct {
	ID            string  `json:"id"`
	Version       int     `json:"ver,omitempty"` // current Communication.meta.versionId; zero/one are legacy-compatible
	Day           float64 `json:"day"`
	Category      string  `json:"cat"`
	Template      string  `json:"tpl,omitempty"`
	Lang          string  `json:"lang,omitempty"`
	Channel       string  `json:"chan,omitempty"`
	Text          string  `json:"text,omitempty"`
	Status        string  `json:"status"`
	Parse         string  `json:"parse,omitempty"`
	Sha           string  `json:"tsha,omitempty"`
	RevisionSha   string  `json:"revision_sha,omitempty"` // normalized same-version conflict detection, including unprojected payload
	Report        string  `json:"rep,omitempty"`          // prep-query thread: the colonoscopy report the query is about
	ReportVersion int     `json:"report_version,omitempty"`
	Grade         string  `json:"grade,omitempty"`  // prep-reply: adequate|inadequate|cannot_determine (structured, not free text)
	Action        string  `json:"action,omitempty"` // handoff-reply: what the referrer did (re_referred)
	Trigger       string  `json:"trig,omitempty"`   // econsult-question: why it was asked (rejected | unanswered | inadequate)
	Unit          string  `json:"unit,omitempty"`   // econsult-question: the receiving unit asked
}

// Signoff is a human reviewer decision on a unified review item (published as a FHIR Provenance).
type Signoff struct {
	ItemID        string    `json:"item"`
	LoopID        string    `json:"loop"`
	Kinds         []string  `json:"kinds"`
	Pair          [2]string `json:"pair,omitempty"`
	ReportID      string    `json:"report,omitempty"`
	ReportVersion int       `json:"report_version,omitempty"` // DiagnosticReport version reviewed; zero is the legacy unversioned replay identity
	Link          string    `json:"link,omitempty"`           // same|different
	Evidence      string    `json:"evidence,omitempty"`       // adequate|inadequate
	Reviewer      string    `json:"reviewer"`
	SignedHead    string    `json:"signed_head"`
	ClinicalDay   *float64  `json:"clinical_day,omitempty"`  // CRC confirmation's event clock, persisted for restart-safe follow-up gating
	ObligationID  string    `json:"obligation_id,omitempty"` // persistent CRC need identity, including after source withdrawal
	Run           string    `json:"run"`
	// v6 governance fields (omitempty keeps v5 sign-offs byte-identical)
	Role      string   `json:"role,omitempty"`    // RBAC role of the signing actor
	Reads     []string `json:"reads,omitempty"`   // distinct reviewer ids that concurred (double review)
	DurationS float64  `json:"dur_s,omitempty"`   // simulated reviewer seconds spent on this item (all reads)
	Purpose   string   `json:"purpose,omitempty"` // v7: second_identifier = messaging confirmation only (never cuts a cluster)
	Hold      string   `json:"hold,omitempty"`    // linker: a "same" held for a confirming review (sex|dob contradiction)
	// Agent read (agentrecs.go): the agent's label and its seconds. DurationS stays human-only.
	AgentRead string  `json:"agent_read,omitempty"`
	AgentS    float64 `json:"agent_s,omitempty"`

	audit *auditRec // H7 senior audit (governance.go); unexported, so never serialised
}

// Prep is the output of the stateless normalise+embed stage; keyed by FHIR stream sequence.
type Prep struct {
	Seq      uint64              `json:"seq"`
	Skip     bool                `json:"skip,omitempty"`
	Reject   *NormalizationError `json:"reject,omitempty"`
	Kind     string              `json:"kind"` // event|tick|signoff
	Ev       *Event              `json:"ev,omitempty"`
	Tick     float64             `json:"tick,omitempty"`
	So       *Signoff            `json:"so,omitempty"`
	Emb      []float32           `json:"emb,omitempty"`
	OriginTs int64               `json:"ots"`
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
	EventID  string      `json:"ev"`
	Provider string      `json:"prov"`
	RType    string      `json:"rtype"`
	Status   string      `json:"status,omitempty"`
	Day      float64     `json:"day"`
	ReportID string      `json:"report,omitempty"`
	Version  int         `json:"ver,omitempty"` // current DiagnosticReport meta.versionId; zero preserves legacy materialized facts
	Reason   string      `json:"reason,omitempty"`
	Parent   string      `json:"parent,omitempty"` // pathology addendum -> colonoscopy report id
	Prog     string      `json:"prog,omitempty"`
	FollowUp string      `json:"follow_up,omitempty"` // typed CRC downstream referral/appointment category
	Ev       *TwoVerdict `json:"evv,omitempty"`
	Strength float64     `json:"strength"`
	Weak     Edge        `json:"weak"`
	Hops     int         `json:"hops"`
	NearMiss bool        `json:"near_miss,omitempty"` // linked only by a sub-threshold score (possible missed link)
	// v7 (set only when V7Cal / V7Guard are on; omitempty keeps v6 facts byte-identical)
	PCal        float64 `json:"pcal7,omitempty"`     // min calibrated edge probability along the widest path (eid/human edges = 1)
	AllHuman    bool    `json:"all_human,omitempty"` // every edge on the path is a human-confirmed link
	RecPk       string  `json:"rec_pk,omitempty"`    // downstream registration (endpoint of the path)
	RecDob      string  `json:"rec_dob,omitempty"`
	EIDSecond   bool    `json:"eid_second,omitempty"`   // the Emirates ID on both registrations confirms the pair (eidSecond)
	EIDContra   string  `json:"eid_contra,omitempty"`   // eidguard.go: an Emirates-ID path ends on, or passes through, a registration that contradicts the referral's (sex|dob); never trusted for closure
	IDConfirmed bool    `json:"id_confirmed,omitempty"` // a human confirmed this registration pair on another loop (ReuseIDConfirm)
}

// reportParentLineage records only explicit report-to-report parent links. Eligibility filtering is deliberately
// separate: an ineligible intermediate report cannot decide a loop, but its declared parent can still establish the
// lineage of an eligible correction.
func reportParentLineage(down []DownFact, prog, rtype string) map[string]string {
	parents := map[string]string{}
	for _, d := range down {
		if d.Prog == prog && d.RType == rtype && d.ReportID != "" && d.Parent != "" {
			parents[d.ReportID] = d.Parent
		}
	}
	return parents
}

// inExplicitReportLineage follows declared parent identity, never chronology or content similarity. The visited set
// prevents malformed parent cycles from turning evaluation into an unbounded walk.
func inExplicitReportLineage(root, reportID string, parents map[string]string) bool {
	if root == "" || reportID == "" {
		return false
	}
	seen := map[string]bool{}
	for reportID != "" && !seen[reportID] {
		if reportID == root {
			return true
		}
		seen[reportID] = true
		reportID = parents[reportID]
	}
	return false
}

// unsupersededReportRows removes ancestors of eligible explicit corrections before chronology is used to choose
// between remaining branches. A child's exam day or lexical ID need not be later than its parent's. Callers supply
// only eligible rows in the authorized index lineage; ineligible intermediates remain usable through parents.
func unsupersededReportRows(reports []row, parents map[string]string) []row {
	var current []row
	for i := range reports {
		id := reports[i].d.ReportID
		superseded := false
		for j := range reports {
			other := reports[j].d.ReportID
			if id != other && inExplicitReportLineage(id, other, parents) && !inExplicitReportLineage(other, id, parents) {
				superseded = true
				break
			}
		}
		if !superseded {
			current = append(current, reports[i])
		}
	}
	return current
}

// reportVersionKey scopes active evidence decisions and review items to the
// exact report version reviewed. Versions zero and one share the historical
// replay key; corrections at v2 and above receive distinct identities.
func reportVersionKey(reportID string, version int) string {
	if version <= 1 {
		return reportID
	}
	return reportID + "#v" + itoa(version)
}

func evidenceReviewItemID(loopID, reportID string, version int, purpose string) string {
	key := reportVersionKey(reportID, version)
	if purpose == OverrideConfirm {
		key += "#confirm"
	} else if purpose == CRCDispositionPurpose {
		key += "#crc-disposition"
	} else if suffix := pilotKey(purpose); suffix != "" {
		key += suffix
	}
	return "rv-" + sha([]byte(loopID + "|evidence|" + key))[:16]
}

func evidenceOverride(over map[string]string, d DownFact, suffix string) (string, bool) {
	value, ok := over[reportVersionKey(d.ReportID, d.Version)+suffix]
	return value, ok
}

func currentReportVersion(f *LoopFacts, reportID string) int {
	version := 0
	for _, d := range f.Down {
		if d.ReportID == reportID && d.Version > version {
			version = d.Version
		}
	}
	return version
}

func sameReportReviewVersion(itemVersion, currentVersion int) bool {
	if currentVersion <= 1 {
		return itemVersion <= 1
	}
	return itemVersion == currentVersion
}

// LoopFacts: everything the linker knows about one FIT+ loop, recomputed whenever its patient cluster changes.
type LoopFacts struct {
	LoopID    string            `json:"loop"`
	InSeq     uint64            `json:"in_seq"`
	Provider  string            `json:"prov"`
	SRDay     float64           `json:"sr_day"`
	SRRec     string            `json:"sr_rec"`
	SRLang    string            `json:"sr_lang,omitempty"`
	Prog      string            `json:"prog,omitempty"`
	Priority  string            `json:"prio,omitempty"`
	Cat       string            `json:"icat,omitempty"`
	IndexDay  float64           `json:"iday,omitempty"`
	Comms     []CommFact        `json:"comms,omitempty"`
	Tasks     []TaskFact        `json:"tasks"`
	Down      []DownFact        `json:"down"`
	Signoffs  []Signoff         `json:"signoffs,omitempty"`
	EvOver    map[string]string `json:"ev_over,omitempty"`     // reportID -> human evidence label
	MsgOptOut bool              `json:"msg_opt_out,omitempty"` // v6: referral patient withheld consent to automated outreach
	Contact   string            `json:"contact,omitempty"`     // stage 3: contact permission at fact time: unknown|granted|denied|opted_out
	SRDob     string            `json:"sr_dob,omitempty"`      // v7: referral registration DOB (second-identifier guard)
	Lung      *LungSR           `json:"lung,omitempty"`        // lung-nodule loop: scope and risk from the tracking ServiceRequest
	DR        *DRSR             `json:"dr,omitempty"`          // diabetic retinopathy loop (dr.go)
	Breast    *BreastSR         `json:"breast,omitempty"`      // breast recall loop (breast.go)
	LDCT      *LDCTSR           `json:"ldct,omitempty"`        // lung cancer screening (LDCT) loop (ldct.go)
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
	Arr      *float64   `json:"arr,omitempty"` // HistClock: arrival day of the input that produced the facts
}

type Block struct {
	Type string                 `json:"type"`
	Body map[string]interface{} `json:"body"`
	Prev string                 `json:"prev"`
	Hash string                 `json:"hash"`
}

type ReviewItem struct {
	ItemID         string    `json:"item"`
	LoopID         string    `json:"loop"`
	Kinds          []string  `json:"kinds"`
	Pair           [2]string `json:"pair"`
	P              float64   `json:"p"`
	ReportID       string    `json:"report"`
	ReportVersion  int       `json:"report_version,omitempty"`
	Verdict        string    `json:"verdict"`
	Conf           float64   `json:"conf"`
	Quote          string    `json:"quote"`
	Chain          []Block   `json:"chain"`
	Head           string    `json:"head"`
	InSeq          uint64    `json:"in_seq"`
	OverdueAtRaise bool      `json:"overdue"`
	ObligationID   string    `json:"obligation_id,omitempty"` // CRC: exactly one source-bound care need
	Reason         string    `json:"reason,omitempty"`        // v7: second_identifier (outreach guard confirmation)
	Day            float64   `json:"day,omitempty"`           // pilot items (shadow_confirm, auto_audit): simulated day raised
}

type Decision struct {
	ID             string              `json:"id"`
	LoopID         string              `json:"loop"`
	InSeq          uint64              `json:"in_seq"`
	Provider       string              `json:"prov"`
	SRDay          float64             `json:"sr_day"`
	Now            float64             `json:"now"`
	State          string              `json:"state"`
	Overdue        bool                `json:"overdue"`
	Unowned        bool                `json:"unowned"`
	Owner          string              `json:"owner,omitempty"`
	Escalations    []string            `json:"esc,omitempty"`
	ReportedClosed bool                `json:"reported_closed"`
	ClosedDay      float64             `json:"closed_day,omitempty"`
	ClosingReport  string              `json:"closing_report,omitempty"`
	Pending        []string            `json:"pending,omitempty"`
	NotifDay       float64             `json:"notif_day"` // -1 = none
	AckDay         float64             `json:"ack_day"`
	RefStatus      string              `json:"ref_status"`
	RefDay         float64             `json:"ref_day"`
	CommFlags      []string            `json:"comm_flags,omitempty"`
	Dropoff        string              `json:"dropoff"`
	HumanAssisted  bool                `json:"human_assisted,omitempty"`
	Prog           string              `json:"prog,omitempty"`
	Priority       string              `json:"prio,omitempty"`
	Deadline       float64             `json:"deadline,omitempty"`
	ProcDay        float64             `json:"proc_day"`           // first trusted fulfilled procedure for this programme; -1 none
	EcState        string              `json:"ec_state,omitempty"` // none|awaiting_reply|accept|conditional|needs_info|redirect|reject|unparsed
	EcTurns        int                 `json:"ec_turns,omitempty"`
	TBState        string              `json:"tb_state,omitempty"` // none|prompted|understood|partial|misunderstood|needs_human|unparsed
	PrepQ          string              `json:"prep_q,omitempty"`   // ungraded prep: awaiting_unit|escalated|clinician_day15|unit_graded|cannot_determine
	Chain          []Block             `json:"chain,omitempty"`
	Policy         string              `json:"policy,omitempty"`      // gate policy that produced this decision (v6 | v7strict+...)
	ConsentLog     []ConsentTransition `json:"consent_log,omitempty"` // stage 3: hold/release/cancel transitions at this decision
	Contact        string              `json:"contact,omitempty"`     // contact permission of the referral patient at this decision
	Sig            string              `json:"sig"`
	Outreach       []Outreach          `json:"outreach,omitempty"`       // v6: patient-facing outreach intents (sent or suppressed)
	Lung           *LungDecision       `json:"lung,omitempty"`           // lung-nodule loop: Fleischner recommendation, window, outcome (lung.go)
	DR             *DRDecision         `json:"dr,omitempty"`             // diabetic retinopathy loop: grade, rule, deadline, eye-clinic outcome (dr.go)
	Breast         *BreastDecision     `json:"breast,omitempty"`         // breast recall loop: BI-RADS category, rule, deadline, assessment outcome (breast.go)
	LDCT           *LDCTDecision       `json:"ldct,omitempty"`           // lung cancer screening loop: Lung-RADS category, rule, clocks, follow-up / work-up outcome (ldct.go)
	Handoff        *HandoffPacket      `json:"handoff,omitempty"`        // finally rejected referral: hand-off packet to the referrer (handoff.go)
	AuditReopened  bool                `json:"audit_reopened,omitempty"` // pilot (-autoaudit): a post-closure audit disagreed and the loop re-opened
	Ifhas          *IfhasDecision      `json:"ifhas,omitempty"`          // IFHAS loop: screening cells, rule, per-domain follow-up status (ifhas.go)
	CRC            *CRCDecision        `json:"crc,omitempty"`            // CRC post-procedure disposition and owned downstream tracking (crc.go)
}

// Outreach is one patient-facing message the closure layer would trigger from a downstream (cross-provider) fact.
// It is only released if the link that carried the fact is trusted (p_cal>=tau | EID | human-confirmed), the patient has
// not opted out, and a coordinator dispatches it (RBAC).
type Outreach struct {
	Kind     string    `json:"kind"`  // appointment_reminder|rebook_after_noshow|referral_rejected|repeat_colonoscopy
	Event    string    `json:"ev"`    // downstream event id that triggered it
	Basis    string    `json:"basis"` // trusted link basis: p_cal|eid|human|below_tau
	PCal     float64   `json:"p_cal"`
	Released bool      `json:"released"`
	Suppress string    `json:"suppress,omitempty"` // link_below_tau_unconfirmed|consent_withheld|second_identifier_unconfirmed
	Guard    string    `json:"guard,omitempty"`    // v7: dob_match|human_path|held
	Pair     [2]string `json:"pair,omitempty"`     // v7: referral registration, downstream registration
	Channel  string    `json:"channel,omitempty"`  // lung outreach: phone_then_letter
	Sender   string    `json:"sender,omitempty"`   // lung outreach: the facility the message is sent in the name of
	Template string    `json:"tpl,omitempty"`      // lung / DR outreach: the approved template id to send (rules.go, -progtpl)
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
