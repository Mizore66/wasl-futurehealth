package main

// Authenticated, non-clinical person reads over the canonical identity artifact.
// No route in this file writes a run or ledger file, infers identity, or changes
// a clinical decision. The response is assembled through an explicit allowlist.
import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	personJourneySchema = "wasl-person-journey/1"
	personNextRule      = "wasl-person-next-action/1"
)

var (
	personIDPattern  = regexp.MustCompile(`^person_[0-9a-f]{64}$`)
	sha256Pattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
	personProgrammes = []string{"crc", "cervical", "lung_nodule", "diabetic_retinopathy", "breast", "lung_ldct", "ifhas"}
)

type personCluster struct {
	registrations []string
	caseIDs       []string
}

type personProjection struct {
	artifactSHA    string
	policy         string
	clusters       []*personCluster
	byPerson       map[string]*personCluster
	byCase         map[string]*personCluster
	referralByCase map[string]string
}

func personProgrammeRank(programme string) int {
	for i, candidate := range personProgrammes {
		if candidate == programme {
			return i
		}
	}
	return len(personProgrammes)
}

func personCaseLess(s *apiServer, a, b string) bool {
	ca, cb := s.cases[a], s.cases[b]
	ra, rb := personProgrammeRank(ca.Programme), personProgrammeRank(cb.Programme)
	return ra < rb || (ra == rb && a < b)
}

func decodeIdentitySnapshot(body []byte) (*identityClusterSnapshot, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var snapshot identityClusterSnapshot
	if err := dec.Decode(&snapshot); err != nil {
		return nil, err
	}
	if err := dec.Decode(new(interface{})); err != io.EOF {
		return nil, fmt.Errorf("identity projection has trailing data")
	}
	return &snapshot, nil
}

// loadPersonProjection validates the complete snapshot and its binding to the
// exact materialised logs on every request. Older runs therefore fail closed
// only on person routes; existing case routes remain available.
func (s *apiServer) loadPersonProjection() (*personProjection, error) {
	body, err := os.ReadFile(filepath.Join(s.run, "identity_clusters.json"))
	if err != nil {
		return nil, err
	}
	snapshot, err := decodeIdentitySnapshot(body)
	if err != nil || snapshot.Schema != identityClusterSchema || !exportOpCode.MatchString(snapshot.Policy) || len(snapshot.Policy) > 160 ||
		!sha256Pattern.MatchString(snapshot.FHIRSHA256) || !sha256Pattern.MatchString(snapshot.OutLogSHA256) || len(snapshot.Clusters) == 0 || snapshot.Cases == nil {
		return nil, fmt.Errorf("invalid identity projection")
	}
	fhirSHA, err := fileSHA256(filepath.Join(s.run, "fhir_log.ndjson"))
	if err != nil || fhirSHA != snapshot.FHIRSHA256 {
		return nil, fmt.Errorf("stale FHIR hash")
	}
	outSHA, err := fileSHA256(filepath.Join(s.run, "out_log.ndjson"))
	if err != nil || outSHA != snapshot.OutLogSHA256 {
		return nil, fmt.Errorf("stale OUT hash")
	}
	mismatch, ok := s.report["live_vs_replay_final_mismatch"].(float64)
	if !ok || mismatch != 0 {
		return nil, fmt.Errorf("live and replay decisions differ")
	}

	projection := &personProjection{artifactSHA: sha(body), policy: snapshot.Policy,
		clusters: []*personCluster{}, byPerson: map[string]*personCluster{}, byCase: map[string]*personCluster{}, referralByCase: map[string]string{}}
	clusterByRoot := map[string]*personCluster{}
	registrationOwner := map[string]*personCluster{}
	lastRoot := ""
	for _, raw := range snapshot.Clusters {
		if raw.Root == "" || raw.Root <= lastRoot || len(raw.Registrations) == 0 {
			return nil, fmt.Errorf("empty identity cluster")
		}
		cluster := &personCluster{registrations: append([]string(nil), raw.Registrations...), caseIDs: []string{}}
		if !sort.StringsAreSorted(cluster.registrations) {
			return nil, fmt.Errorf("unsorted identity cluster")
		}
		foundRoot := false
		for _, registration := range cluster.registrations {
			if registration == "" || registrationOwner[registration] != nil {
				return nil, fmt.Errorf("empty or overlapping registration")
			}
			foundRoot = foundRoot || registration == raw.Root
			registrationOwner[registration] = cluster
		}
		if !foundRoot || clusterByRoot[raw.Root] != nil {
			return nil, fmt.Errorf("invalid or duplicate cluster root")
		}
		clusterByRoot[raw.Root] = cluster
		lastRoot = raw.Root
		projection.clusters = append(projection.clusters, cluster)
		person := personIDForRegistrations(cluster.registrations)
		if projection.byPerson[person] != nil {
			return nil, fmt.Errorf("duplicate person")
		}
		projection.byPerson[person] = cluster
	}
	seenCases := map[string]bool{}
	lastCase := ""
	for _, mapping := range snapshot.Cases {
		if mapping.CaseID == "" || mapping.CaseID <= lastCase || seenCases[mapping.CaseID] || s.cases[mapping.CaseID] == nil {
			return nil, fmt.Errorf("duplicate or unknown case mapping")
		}
		seenCases[mapping.CaseID] = true
		lastCase = mapping.CaseID
		cluster := registrationOwner[mapping.Registration]
		if cluster == nil {
			return nil, fmt.Errorf("case refers to an unmapped registration")
		}
		projection.byCase[mapping.CaseID] = cluster
		projection.referralByCase[mapping.CaseID] = mapping.Registration
	}
	// Surveillance children inherit only their canonical parent loop's referral
	// membership. Every materialised case must resolve; singleton guesses are not allowed.
	for caseID := range s.cases {
		cluster := projection.byCase[caseID]
		if cluster == nil && baseLoop(caseID) != caseID {
			cluster = projection.byCase[baseLoop(caseID)]
			if cluster != nil {
				projection.byCase[caseID] = cluster
				projection.referralByCase[caseID] = projection.referralByCase[baseLoop(caseID)]
			}
		}
		if cluster == nil {
			return nil, fmt.Errorf("unmapped materialised case")
		}
		cluster.caseIDs = append(cluster.caseIDs, caseID)
	}
	for _, cluster := range projection.clusters {
		if len(cluster.caseIDs) == 0 {
			delete(projection.byPerson, personIDForRegistrations(cluster.registrations))
			continue
		}
		sort.Slice(cluster.caseIDs, func(i, j int) bool { return personCaseLess(s, cluster.caseIDs[i], cluster.caseIDs[j]) })
	}
	return projection, nil
}

func (s *apiServer) personError(w http.ResponseWriter, status int, message string) {
	writeJSONResp(w, status, s.envelope(map[string]interface{}{"error": message}))
}

func (s *apiServer) peopleRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.personError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	caseID, person := "", ""
	if r.URL.Path == "/api/people" {
		query := r.URL.Query()
		if len(query) != 1 || len(query["case_id"]) != 1 || query.Get("case_id") == "" {
			s.personError(w, http.StatusBadRequest, "exactly one case_id query parameter is required")
			return
		}
		caseID = query.Get("case_id")
	} else if strings.HasPrefix(r.URL.Path, "/api/people/") {
		if len(r.URL.Query()) != 0 {
			s.personError(w, http.StatusBadRequest, "person route does not accept query parameters")
			return
		}
		raw := strings.TrimPrefix(r.URL.Path, "/api/people/")
		var unescapeErr error
		person, unescapeErr = url.PathUnescape(raw)
		if unescapeErr != nil || !personIDPattern.MatchString(person) {
			s.personError(w, http.StatusBadRequest, "malformed person ID")
			return
		}
	} else {
		s.personError(w, http.StatusBadRequest, "malformed person route")
		return
	}
	projection, err := s.loadPersonProjection()
	if err != nil {
		s.personError(w, http.StatusServiceUnavailable, "person_projection_unavailable")
		return
	}
	var cluster *personCluster
	if caseID != "" {
		if s.cases[caseID] == nil {
			s.personError(w, http.StatusNotFound, "unknown case")
			return
		}
		cluster = projection.byCase[caseID]
	} else {
		cluster = projection.byPerson[person]
	}
	if cluster == nil {
		s.personError(w, http.StatusNotFound, "unknown person")
		return
	}
	body, err := s.personJourney(projection, cluster, requestViewer(r))
	if err != nil {
		s.personError(w, http.StatusInternalServerError, "person journey could not be encoded")
		return
	}
	writeJSONResp(w, http.StatusOK, s.envelope(body))
}

func obligation(caseID, programme, source string, due, windowStart float64, status string, basis string) map[string]interface{} {
	o := map[string]interface{}{"case_id": caseID, "programme": programme, "kind": "programme_deadline",
		"due_day": round4(due), "status": status, "source_field": source}
	if windowStart != 0 {
		o["window_start_day"] = round4(windowStart)
	}
	if basis != "" {
		o["basis"] = basis
	}
	return o
}

func openStatus(c *apiCase, due float64) string {
	if c.Overdue && c.last != nil && c.last.Now > due {
		return "overdue"
	}
	return "open"
}

// personObligations reads existing canonical clocks only. In particular, it
// never computes a new interval or a due-soon threshold.
func personObligations(c *apiCase, sharedSurveillance ...map[string]bool) []map[string]interface{} {
	out := []map[string]interface{}{}
	if c == nil || c.last == nil {
		return out
	}
	d := c.last
	switch c.Programme {
	case "crc", "cervical":
		if !c.Closed && c.Deadline != 0 {
			out = append(out, obligation(c.ID, c.Programme, "sr_day+deadline", c.SRDay+c.Deadline, 0, openStatus(c, c.SRDay+c.Deadline), ""))
		}
	case "lung_nodule":
		if d.Lung != nil {
			if !c.Closed && d.Lung.WindowEnd != 0 {
				out = append(out, obligation(c.ID, c.Programme, "lung.window_end_day", d.Lung.WindowEnd, d.Lung.WindowStart, openStatus(c, d.Lung.WindowEnd), ""))
			}
			if d.State == "awaiting_tissue" && d.Lung.TissueDue != 0 {
				status := "open"
				if d.Overdue && d.Now > d.Lung.TissueDue {
					status = "overdue"
				}
				out = append(out, obligation(c.ID, c.Programme, "lung.tissue_due_day", d.Lung.TissueDue, 0, status, ""))
			}
			seen := map[string]bool{}
			if len(sharedSurveillance) > 0 && sharedSurveillance[0] != nil {
				seen = sharedSurveillance[0]
			}
			for surveillance := d.Lung.Surveillance; surveillance != nil; surveillance = surveillance.Next {
				if surveillance.State != "due" && surveillance.State != "overdue" {
					continue
				}
				key := fmt.Sprintf("%s\x00%d\x00%.4f", surveillance.Loop, surveillance.Round, surveillance.DueDay)
				if seen[key] {
					continue
				}
				seen[key] = true
				status := "scheduled"
				if surveillance.State == "overdue" {
					status = "overdue"
				}
				out = append(out, obligation(c.ID, c.Programme, "lung.surveillance.due_day", surveillance.DueDay, surveillance.WindowStart, status, surveillance.Basis))
			}
		}
	case "diabetic_retinopathy":
		if d.DR != nil {
			if !c.Closed && d.DR.DeadlineDay != 0 {
				out = append(out, obligation(c.ID, c.Programme, "dr.deadline_day", d.DR.DeadlineDay, 0, openStatus(c, d.DR.DeadlineDay), ""))
			}
			if next := d.DR.Next; next != nil && !next.Seen {
				status := "scheduled"
				if next.Overdue {
					status = "overdue"
				}
				out = append(out, obligation(c.ID, c.Programme, "dr.next_due.due_day", next.DueDay, 0, status, next.Basis))
			}
		}
	case "breast":
		if d.Breast != nil {
			if !c.Closed && d.Breast.DeadlineDay != 0 {
				out = append(out, obligation(c.ID, c.Programme, "breast.deadline_day", d.Breast.DeadlineDay, 0, openStatus(c, d.Breast.DeadlineDay), ""))
			}
			if d.Breast.TreatmentDueDay != 0 {
				out = append(out, obligation(c.ID, c.Programme, "breast.treatment_referral_due_day", d.Breast.TreatmentDueDay, 0, "informational", "recorded_only"))
			}
		}
	case "lung_ldct":
		if d.LDCT != nil {
			annual := d.LDCT.Rec != nil && d.LDCT.Rec.Action == "annual"
			if annual && d.LDCT.DueDay != 0 {
				out = append(out, obligation(c.ID, c.Programme, "ldct.due_day", d.LDCT.DueDay, 0, "scheduled", ""))
			} else if !c.Closed && d.LDCT.DueDay != 0 {
				out = append(out, obligation(c.ID, c.Programme, "ldct.due_day", d.LDCT.DueDay, 0, openStatus(c, d.LDCT.DueDay), ""))
			}
			if !annual && !c.Closed && d.LDCT.DeadlineDay != 0 && d.LDCT.DeadlineDay != d.LDCT.DueDay {
				out = append(out, obligation(c.ID, c.Programme, "ldct.deadline_day", d.LDCT.DeadlineDay, 0, openStatus(c, d.LDCT.DeadlineDay), ""))
			}
		}
	case "ifhas":
		if d.Ifhas != nil && !c.Closed && d.Ifhas.DeadlineDay != 0 {
			out = append(out, obligation(c.ID, c.Programme, "ifhas.deadline_day", d.Ifhas.DeadlineDay, 0, openStatus(c, d.Ifhas.DeadlineDay), ""))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		ad, bd := a["due_day"].(float64), b["due_day"].(float64)
		return ad < bd || (ad == bd && (a["source_field"].(string) < b["source_field"].(string) ||
			(a["source_field"] == b["source_field"] && a["case_id"].(string) < b["case_id"].(string))))
	})
	return out
}

func sortObligations(s *apiServer, obligations []map[string]interface{}) {
	sort.Slice(obligations, func(i, j int) bool {
		a, b := obligations[i], obligations[j]
		ad, bd := a["due_day"].(float64), b["due_day"].(float64)
		if ad != bd {
			return ad < bd
		}
		ar, br := personProgrammeRank(a["programme"].(string)), personProgrammeRank(b["programme"].(string))
		if ar != br {
			return ar < br
		}
		ac, bc := a["case_id"].(string), b["case_id"].(string)
		if ac != bc {
			return ac < bc
		}
		ak, bk := a["kind"].(string), b["kind"].(string)
		if ak != bk {
			return ak < bk
		}
		return a["source_field"].(string) < b["source_field"].(string)
	})
}

type personIntent struct {
	CaseID            string
	Programme         string
	FirstDecisionID   string
	LatestDecisionID  string
	LatestDecisionSig string
	Lifecycle         string
	Outreach          Outreach
}

func intentKey(intent personIntent) string {
	return intent.CaseID + "\x00" + intent.Outreach.Event + "\x00" + intent.Outreach.Kind
}

func intentLifecycle(outreach Outreach) string {
	if outreach.Released {
		return "released"
	}
	if outreach.Suppress == SupConsent || outreach.Suppress == SupBelowTau {
		return "withheld"
	}
	if outreach.Suppress != "" {
		return "held"
	}
	return "recorded"
}

func foldPersonIntents(c *apiCase) []personIntent {
	latest := map[string]personIntent{}
	order := []string{}
	for _, decision := range c.decisions {
		for _, outreach := range decision.Outreach {
			key := c.ID + "\x00" + outreach.Event + "\x00" + outreach.Kind
			intent, exists := latest[key]
			if !exists {
				intent = personIntent{CaseID: c.ID, Programme: c.Programme, FirstDecisionID: decision.ID}
				order = append(order, key)
			}
			intent.LatestDecisionID, intent.LatestDecisionSig = decision.ID, decision.Sig
			intent.Lifecycle, intent.Outreach = intentLifecycle(outreach), outreach
			latest[key] = intent
		}
	}
	out := make([]personIntent, 0, len(order))
	for _, key := range order {
		out = append(out, latest[key])
	}
	return out
}

func actionableIntent(intent personIntent) bool {
	return intent.Lifecycle == "released" || intent.Lifecycle == "held" || intent.Lifecycle == "recorded"
}

func personFlagID(match string, intents []personIntent) string {
	keys := make([]string, 0, len(intents))
	for _, intent := range intents {
		keys = append(keys, intentKey(intent))
	}
	sort.Strings(keys)
	return "outreach_" + sha([]byte("wasl-person-outreach-v1\x00"+match+"\x00"+strings.Join(keys, "\x00")))
}

func intentBody(intent personIntent) map[string]interface{} {
	return map[string]interface{}{"case_id": intent.CaseID, "programme": intent.Programme,
		"first_decision_id": intent.FirstDecisionID, "latest_decision_id": intent.LatestDecisionID,
		"latest_decision_sig": intent.LatestDecisionSig, "lifecycle": intent.Lifecycle, "outreach": intent.Outreach}
}

func flagBody(match string, intents []personIntent) map[string]interface{} {
	actionable, count := 0, 0
	for _, intent := range intents {
		if actionableIntent(intent) {
			count++
		}
	}
	actionable = count
	bodies := make([]map[string]interface{}, 0, len(intents))
	for _, intent := range intents {
		bodies = append(bodies, intentBody(intent))
	}
	return map[string]interface{}{"id": personFlagID(match, intents), "match": match, "actionable": actionable >= 2, "intents": bodies}
}

func personOutreachFlags(s *apiServer, cases []*apiCase) []map[string]interface{} {
	intents := []personIntent{}
	for _, c := range cases {
		intents = append(intents, foldPersonIntents(c)...)
	}
	sort.Slice(intents, func(i, j int) bool {
		if intents[i].Programme != intents[j].Programme {
			return personProgrammeRank(intents[i].Programme) < personProgrammeRank(intents[j].Programme)
		}
		if intents[i].CaseID != intents[j].CaseID {
			return intents[i].CaseID < intents[j].CaseID
		}
		return intentKey(intents[i]) < intentKey(intents[j])
	})
	flags := []map[string]interface{}{}
	duplicates := map[string][]personIntent{}
	for _, intent := range intents {
		key := intent.Outreach.Kind + "\x00" + intent.Outreach.Event
		duplicates[key] = append(duplicates[key], intent)
	}
	keys := make([]string, 0, len(duplicates))
	for key := range duplicates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		group := duplicates[key]
		cases := map[string]bool{}
		for _, intent := range group {
			cases[intent.CaseID] = true
		}
		if len(cases) >= 2 {
			flags = append(flags, flagBody("duplicate", group))
		}
	}
	programmes, actionable := map[string]bool{}, 0
	for _, intent := range intents {
		if actionableIntent(intent) {
			actionable++
			programmes[intent.Programme] = true
		}
	}
	if len(programmes) >= 2 && len(intents) >= 2 {
		flag := flagBody("overlap", intents)
		flag["actionable"] = actionable >= 2
		flags = append(flags, flag)
	}
	return flags
}

var coordinatorDropoffs = map[string]bool{
	"never_notified": true, "no_acknowledgement": true, "not_booked": true,
	"referral_rejected": true, "referral_unanswered": true, "no_show": true,
	"no_followup_ct": true, "no_eye_exam": true, "ungradable_no_repeat": true,
	"technical_repeat_not_done": true, "incomplete_scan_not_completed": true,
	"followup_ldct_not_done": true, "no_followup_documented": true,
}

type personActionCandidate struct {
	kind, caseID, flagID string
	priority             string
	deadline             float64
	hasDeadline          bool
	overdue              bool
	reasons              []string
}

func candidateDeadline(obligations []map[string]interface{}, caseID string) (float64, bool) {
	earliest, found := 0.0, false
	for _, obligation := range obligations {
		if obligation["case_id"] != caseID || obligation["status"] == "informational" ||
			(obligation["source_field"] == "ldct.due_day" && obligation["status"] == "scheduled") {
			continue
		}
		due := obligation["due_day"].(float64)
		if !found || due < earliest {
			earliest, found = due, true
		}
	}
	return earliest, found
}

func appendReasons(candidate *personActionCandidate, c *apiCase) {
	if c.Priority == "urgent" {
		candidate.reasons = append(candidate.reasons, "case_priority_urgent")
	}
	if c.Overdue || candidate.overdue {
		candidate.reasons = append(candidate.reasons, "case_overdue")
	}
}

func personActionCandidates(s *apiServer, cases []*apiCase, obligations []map[string]interface{}, flags []map[string]interface{}) []personActionCandidate {
	candidates := []personActionCandidate{}
	caseByID := map[string]*apiCase{}
	for _, c := range cases {
		caseByID[c.ID] = c
	}
	for _, flag := range flags {
		if flag["actionable"] != true {
			continue
		}
		intents := flag["intents"].([]map[string]interface{})
		if len(intents) == 0 {
			continue
		}
		anchor := intents[0]["case_id"].(string)
		candidate := personActionCandidate{kind: "review_duplicate_outreach", caseID: anchor, flagID: flag["id"].(string),
			priority: "routine", reasons: []string{"duplicate_outreach_requires_review"}}
		for _, raw := range intents {
			c := caseByID[raw["case_id"].(string)]
			if c == nil {
				continue
			}
			if c.Priority == "urgent" {
				candidate.priority = "urgent"
			}
			candidate.overdue = candidate.overdue || c.Overdue
			deadline, found := candidateDeadline(obligations, c.ID)
			if found && (!candidate.hasDeadline || deadline < candidate.deadline) {
				candidate.deadline, candidate.hasDeadline = deadline, true
			}
		}
		if candidate.priority == "urgent" {
			candidate.reasons = append(candidate.reasons, "case_priority_urgent")
		}
		if candidate.overdue {
			candidate.reasons = append(candidate.reasons, "case_overdue")
		}
		candidates = append(candidates, candidate)
	}
	for _, c := range cases {
		patientFollowup := false
		reason := ""
		if !c.Closed && coordinatorDropoffs[c.Dropoff] {
			patientFollowup = c.Dropoff != "referral_rejected" && c.Dropoff != "referral_unanswered"
			reason = "dropoff_" + c.Dropoff
		} else if c.last.DR != nil && c.last.DR.Next != nil && c.last.DR.Next.Overdue && !c.last.DR.Next.Seen {
			patientFollowup, reason = true, "dr_next_recall_overdue"
		}
		if reason == "" {
			continue
		}
		candidate := personActionCandidate{kind: "programme_follow_up", caseID: c.ID, priority: c.Priority,
			overdue: c.Overdue, reasons: []string{reason}}
		if c.last.DR != nil && c.last.DR.Next != nil && c.last.DR.Next.Overdue {
			candidate.overdue = true
		}
		candidate.deadline, candidate.hasDeadline = candidateDeadline(obligations, c.ID)
		if candidate.priority != "urgent" {
			candidate.priority = "routine"
		}
		if patientFollowup && c.Consent != "granted" {
			candidate.kind = "review_contact_permission"
			candidate.reasons = append(candidate.reasons, "consent_not_granted")
		}
		appendReasons(&candidate, c)
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if (a.priority == "urgent") != (b.priority == "urgent") {
			return a.priority == "urgent"
		}
		if a.overdue != b.overdue {
			return a.overdue
		}
		if a.hasDeadline != b.hasDeadline {
			return a.hasDeadline
		}
		if a.hasDeadline && a.deadline != b.deadline {
			return a.deadline < b.deadline
		}
		rank := map[string]int{"review_duplicate_outreach": 0, "review_contact_permission": 1, "programme_follow_up": 2}
		if rank[a.kind] != rank[b.kind] {
			return rank[a.kind] < rank[b.kind]
		}
		if a.caseID != b.caseID {
			return a.caseID < b.caseID
		}
		return a.flagID < b.flagID
	})
	return candidates
}

// personCaseDetail restricts copied case data to the canonical trusted cluster.
// caseDetailFor intentionally retains full case history for the case routes; a
// person journey must neither label near-miss links as trusted nor copy their
// associated registrations and events into the unified timeline.
func (s *apiServer) personCaseDetail(c *apiCase, viewer *apiPrincipal, registrations []string) map[string]interface{} {
	detail := s.caseDetailFor(c, viewer)
	trustedRegistrations := map[string]bool{}
	for _, registration := range registrations {
		trustedRegistrations[registration] = true
	}
	links := []map[string]interface{}{}
	for _, link := range detail["links"].([]map[string]interface{}) {
		if nearMiss, _ := link["near_miss"].(bool); nearMiss {
			continue
		}
		links = append(links, link)
	}
	detail["links"] = links
	identities := []map[string]string{}
	for _, identity := range s.ids[c.ID] {
		if trustedRegistrations[identity["registration"]] {
			identities = append(identities, identity)
		}
	}
	detail["identities"] = identities
	timeline := []map[string]interface{}{}
	for _, event := range s.facts[c.ID] {
		registration, _ := event["patient_key"].(string)
		if !trustedRegistrations[registration] {
			continue
		}
		row := make(map[string]interface{}, len(event)-1)
		for key, value := range event {
			if key != "patient_key" {
				row[key] = value
			}
		}
		timeline = append(timeline, row)
	}
	detail["timeline"] = timeline
	holds := []map[string]interface{}{}
	seenHolds := map[string]bool{}
	if c.last != nil {
		for _, block := range c.last.Chain {
			if block.Type == "hold_decision" && !seenHolds[block.Hash] {
				seenHolds[block.Hash] = true
				holds = append(holds, block.Body)
			}
		}
	}
	detail["holds"] = holds
	return detail
}

func (s *apiServer) personJourney(projection *personProjection, cluster *personCluster, viewer apiPrincipal) (map[string]interface{}, error) {
	cases := make([]*apiCase, 0, len(cluster.caseIDs))
	caseBodies := make([]map[string]interface{}, 0, len(cluster.caseIDs))
	obligations := []map[string]interface{}{}
	programmes := []string{}
	seenProgrammes := map[string]bool{}
	seenSurveillance := map[string]bool{}
	asOf := 0.0
	for _, caseID := range cluster.caseIDs {
		c := s.cases[caseID]
		if c == nil || c.last == nil {
			return nil, fmt.Errorf("incomplete case projection")
		}
		cases = append(cases, c)
		if len(cases) == 1 || c.last.Now > asOf {
			asOf = c.last.Now
		}
		if !seenProgrammes[c.Programme] {
			seenProgrammes[c.Programme] = true
			programmes = append(programmes, c.Programme)
		}
		detail := s.personCaseDetail(c, &viewer, cluster.registrations)
		allowed := map[string]interface{}{}
		for _, key := range []string{"case", "identities", "timeline", "links", "holds", "outreach", "decisions"} {
			allowed[key] = detail[key]
		}
		if summary, ok := allowed["case"].(map[string]interface{}); ok {
			delete(summary, "api_overlay")
		}
		overlay, _ := s.apiContactOverlay(c, viewer)
		referral := projection.referralByCase[caseID]
		if referral == "" && baseLoop(caseID) != caseID {
			referral = projection.referralByCase[baseLoop(caseID)]
		}
		if referral == "" {
			return nil, fmt.Errorf("case has no validated referral registration")
		}
		body := map[string]interface{}{"case_id": caseID, "referral_registration": pseudonymizeRegistration(referral), "detail": allowed,
			"contact": map[string]interface{}{"canonical": c.Consent, "overlay_status": overlay["status"],
				"contact_blocked": overlay["contact_blocked"] == true || c.Consent != "granted", "canonical_unchanged": true}}
		if baseLoop(caseID) != caseID {
			body["parent_case_id"] = baseLoop(caseID)
		}
		caseBodies = append(caseBodies, body)
		obligations = append(obligations, personObligations(c, seenSurveillance)...)
	}
	sortObligations(s, obligations)
	flags := personOutreachFlags(s, cases)
	candidates := personActionCandidates(s, cases, obligations, flags)
	next := map[string]interface{}{"kind": "none", "priority": "routine", "overdue": false, "reason_codes": []string{"no_open_coordinator_action"}, "rule": personNextRule}
	if len(candidates) > 0 {
		candidate := candidates[0]
		next = map[string]interface{}{"kind": candidate.kind, "case_id": candidate.caseID, "priority": candidate.priority,
			"overdue": candidate.overdue, "reason_codes": candidate.reasons, "rule": personNextRule}
		if candidate.flagID != "" {
			next["outreach_flag_id"] = candidate.flagID
		}
		if candidate.hasDeadline {
			next["deadline_day"] = round4(candidate.deadline)
		}
	}
	registrations := make([]map[string]interface{}, 0, len(cluster.registrations))
	for _, registration := range cluster.registrations {
		provider, _, _ := strings.Cut(registration, "|")
		registrations = append(registrations, map[string]interface{}{"registration": registration, "providers": []string{provider}})
	}
	body := map[string]interface{}{"schema": personJourneySchema,
		"person": map[string]interface{}{"id": personIDForRegistrations(cluster.registrations), "anchor_case_id": cluster.caseIDs[0],
			"as_of_day": round4(asOf), "case_ids": append([]string(nil), cluster.caseIDs...), "programmes": programmes,
			"registrations": registrations,
			"identity_provenance": map[string]interface{}{"source": "canonical_linker_clusters", "artifact_schema": identityClusterSchema,
				"artifact_sha256": projection.artifactSHA, "policy": projection.policy}},
		"cases": caseBodies, "obligations": obligations, "outreach_flags": flags, "next_action": next}
	generic, err := exportGeneric(body)
	if err != nil {
		return nil, err
	}
	return pseudonymizeRegistrationFields(generic, "").(map[string]interface{}), nil
}
