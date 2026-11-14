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
	artifactSchema string
	catalog        map[string]retainedIdentityCase
	historical     map[string][]string // person ID -> proven non-actionable references
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
	if err := rejectSemanticDuplicateKeys(body); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var header struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(body, &header); err != nil {
		return nil, err
	}
	// Decode each version with its own closed wire type. In particular, /1 may
	// not silently acquire catalog/unavailable fields from the /2 representation.
	type legacy struct {
		Schema       string            `json:"schema"`
		Policy       string            `json:"policy"`
		FHIRSHA256   string            `json:"fhir_sha256"`
		OutLogSHA256 string            `json:"out_log_sha256"`
		Clusters     []identityCluster `json:"clusters"`
		Cases        []identityCase    `json:"cases"`
	}
	type retained struct {
		Schema       string                 `json:"schema"`
		Policy       string                 `json:"policy"`
		FHIRSHA256   string                 `json:"fhir_sha256"`
		OutLogSHA256 string                 `json:"out_log_sha256"`
		Clusters     []identityCluster      `json:"clusters"`
		Catalog      []retainedIdentityCase `json:"catalog"`
	}
	var snapshot identityClusterSnapshot
	switch header.Schema {
	case identityClusterSchema:
		var old legacy
		if err := dec.Decode(&old); err != nil {
			return nil, err
		}
		snapshot = identityClusterSnapshot{Schema: old.Schema, Policy: old.Policy, FHIRSHA256: old.FHIRSHA256, OutLogSHA256: old.OutLogSHA256, Clusters: old.Clusters, Cases: old.Cases}
	case retainedIdentitySchema:
		var doc retained
		if err := dec.Decode(&doc); err != nil {
			return nil, err
		}
		snapshot = identityClusterSnapshot{Schema: doc.Schema, Policy: doc.Policy, FHIRSHA256: doc.FHIRSHA256, OutLogSHA256: doc.OutLogSHA256, Clusters: doc.Clusters, Catalog: doc.Catalog}
	default:
		return nil, fmt.Errorf("unsupported identity projection schema")
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
	if err != nil || !exportOpCode.MatchString(snapshot.Policy) || len(snapshot.Policy) > 160 ||
		!sha256Pattern.MatchString(snapshot.FHIRSHA256) || !sha256Pattern.MatchString(snapshot.OutLogSHA256) ||
		(snapshot.Schema == identityClusterSchema && (len(snapshot.Clusters) == 0 || snapshot.Cases == nil)) || (snapshot.Schema == retainedIdentitySchema && (snapshot.Catalog == nil || snapshot.Clusters == nil)) {
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
		clusters: []*personCluster{}, byPerson: map[string]*personCluster{}, byCase: map[string]*personCluster{}, referralByCase: map[string]string{},
		artifactSchema: snapshot.Schema, catalog: map[string]retainedIdentityCase{}, historical: map[string][]string{}}
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
	if snapshot.Schema == retainedIdentitySchema {
		if err := s.loadRetainedPersonCatalog(snapshot, projection, registrationOwner); err != nil {
			return nil, err
		}
		return projection, nil
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
	for caseID, c := range s.cases {
		if c.last != nil && c.last.Inactive {
			return nil, fmt.Errorf("legacy identity artifact cannot represent retained cases")
		}
		cluster := projection.byCase[caseID]
		parent := materializedParent(c.last)
		if cluster == nil && parent != caseID {
			cluster = projection.byCase[parent]
			if cluster != nil {
				projection.byCase[caseID] = cluster
				projection.referralByCase[caseID] = projection.referralByCase[parent]
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

func (s *apiServer) loadRetainedPersonCatalog(snapshot *identityClusterSnapshot, p *personProjection, owners map[string]*personCluster) error {
	actual, err := retainedOutDecisions(s.run)
	if err != nil {
		return err
	}
	if len(snapshot.Catalog) != len(actual) || len(actual) != len(s.cases) {
		return fmt.Errorf("incomplete retained case catalog")
	}
	lastID := ""
	for _, row := range snapshot.Catalog {
		history := actual[row.CaseID]
		c := s.cases[row.CaseID]
		if row.CaseID <= lastID || len(history) == 0 || c == nil || string(canon(c.decisions)) != string(canon(history)) {
			return fmt.Errorf("duplicate, unknown or stale retained case")
		}
		lastID = row.CaseID
		last := history[len(history)-1]
		lifecycle := "active"
		if last.Inactive {
			lifecycle = "retained"
		}
		parent, err := retainedParent(row.CaseID, actual)
		if err != nil || row.ParentCaseID != parent || row.Lifecycle != lifecycle || len(row.History) != len(history) {
			return fmt.Errorf("invalid retained lifecycle, parent or history")
		}
		if !last.Inactive && (last.State == "inactive" || personProgrammeRank(last.Prog) == len(personProgrammes)) {
			return fmt.Errorf("invalid active case")
		}
		if last.Inactive && (last.State != "inactive" || last.ReportedClosed || last.Overdue || len(last.Outreach) != 0) {
			return fmt.Errorf("actionable retained tombstone")
		}
		if last.Inactive {
			if row.Current != unavailableAssociation("inactive_request") || row.Participation != "reference_only" {
				return fmt.Errorf("retained case has current identity authority")
			}
		} else if row.Participation == "reference_only" {
			// This is not an inactive clinical decision or missing-map forgiveness.
			// Exact origin, transition and signed parent lineage are validated below.
			if parent == "" || row.Current != unavailableAssociation("detached_child") {
				return fmt.Errorf("invalid detached child participation")
			}
		} else {
			if row.Participation != "current" {
				return fmt.Errorf("missing positive current participation")
			}
			cluster := owners[row.Current.Registration]
			if row.Current.Status != "resolved" || cluster == nil {
				return fmt.Errorf("active case has no proven current association")
			}
			p.byCase[row.CaseID] = cluster
			p.referralByCase[row.CaseID] = row.Current.Registration
			cluster.caseIDs = append(cluster.caseIDs, row.CaseID)
		}
		seenPeople := map[string]bool{}
		for i, ctx := range row.History {
			d := history[i]
			if ctx.DecisionID != d.ID || ctx.DecisionSig != d.Sig || d.Policy != snapshot.Policy || !sha256Pattern.MatchString(d.Sig) || d.Sig != decSig(d) || !verifyChain(d.Chain) || ctx.FactsSeq == 0 {
				return fmt.Errorf("invalid same-case history signature")
			}
			if ctx.Association.Status == "resolved" {
				cluster := owners[ctx.Association.Registration]
				if cluster == nil {
					return fmt.Errorf("unmapped historical registration")
				}
				person := personIDForRegistrations(cluster.registrations)
				if !seenPeople[person] {
					p.historical[person] = append(p.historical[person], row.CaseID)
					seenPeople[person] = true
				}
			} else if ctx.Association.Status != "unavailable" {
				return fmt.Errorf("invalid history availability")
			}
		}
		p.catalog[row.CaseID] = row
	}
	canonical := map[string]string{}
	for registration, cluster := range owners {
		canonical[registration] = personIDForRegistrations(cluster.registrations)
	}
	if err := validateRetainedSources(s.run, snapshot.Catalog, actual, canonical); err != nil {
		return err
	}
	for person, cluster := range p.byPerson {
		if len(cluster.caseIDs) == 0 && len(p.historical[person]) == 0 {
			delete(p.byPerson, person)
			continue
		}
		sort.Slice(cluster.caseIDs, func(i, j int) bool { return personCaseLess(s, cluster.caseIDs[i], cluster.caseIDs[j]) })
		sort.Strings(p.historical[person])
	}
	// A reference-only case (retired or clinically active but detached) resolves
	// only a UNIQUE historical subject. Never guess between truthful subjects.
	for id, row := range p.catalog {
		if row.Participation != "reference_only" {
			continue
		}
		var selected *personCluster
		ambiguous := false
		for _, ctx := range row.History {
			if ctx.Association.Status != "resolved" {
				continue
			}
			candidate := owners[ctx.Association.Registration]
			if selected != nil && selected != candidate {
				ambiguous = true
			}
			selected = candidate
		}
		if !ambiguous {
			p.byCase[id] = selected
		}
	}
	return nil
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
		if cluster == nil && projection.artifactSchema == retainedIdentitySchema {
			row := projection.catalog[caseID]
			reason := "multiple_historical_subjects"
			resolved := false
			for _, ctx := range row.History {
				if ctx.Association.Status == "resolved" {
					resolved = true
				} else {
					reason = ctx.Association.Reason
				}
			}
			if resolved {
				reason = "multiple_historical_subjects"
			}
			writeJSONResp(w, http.StatusConflict, s.envelope(map[string]interface{}{"schema": "wasl-person-association-unavailable/2", "case_id": caseID, "lifecycle": row.Lifecycle,
				"association": map[string]interface{}{"status": "unavailable", "reason": reason}, "history_access": "authorized_case_audit", "actionable": false}))
			return
		}
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
	if c == nil || c.last == nil || c.last.Inactive {
		return out
	}
	d := c.last
	switch c.Programme {
	case "crc", "cervical":
		if c.Programme == "crc" && d.CRC != nil {
			cd := d.CRC
			if cd.Parent != "" {
				switch {
				case cd.Withdrawn:
					out = append(out, obligation(c.ID, c.Programme, "crc.due_day", cd.DueDay, 0, "withdrawn_review", "tracking_without_deadline"))
				case cd.FollowUpState == "awaiting_acceptance_or_first_appointment":
					out = append(out, obligation(c.ID, c.Programme, "crc.disposition_day", cd.DispositionDay, 0, cd.FollowUpState, "tracking_without_deadline"))
				case !c.Closed:
					out = append(out, obligation(c.ID, c.Programme, "crc.due_day", cd.DueDay, 0, openStatus(c, cd.DueDay), ""))
				}
				break
			}
			for _, work := range cd.Coordination {
				if work.Status == "open" {
					out = append(out, obligation(c.ID, c.Programme, "crc.coordination.due_day", work.DueDay, 0, openStatus(c, work.DueDay), work.ID))
				}
			}
			if cd.ScreeningComplete {
				break // downstream care does not reopen the screening referral clock
			}
		}
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
	Historical        bool
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
				intent = personIntent{CaseID: c.ID, Programme: decision.Prog, FirstDecisionID: decision.ID}
				order = append(order, key)
			}
			intent.LatestDecisionID, intent.LatestDecisionSig = decision.ID, decision.Sig
			intent.Lifecycle, intent.Outreach = intentLifecycle(outreach), outreach
			intent.Programme = decision.Prog
			intent.Historical = c.last != nil && c.last.Inactive
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
	return !intent.Historical && (intent.Lifecycle == "released" || intent.Lifecycle == "held" || intent.Lifecycle == "recorded")
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
	body := map[string]interface{}{"case_id": intent.CaseID, "programme": intent.Programme,
		"first_decision_id": intent.FirstDecisionID, "latest_decision_id": intent.LatestDecisionID,
		"latest_decision_sig": intent.LatestDecisionSig, "lifecycle": intent.Lifecycle, "outreach": intent.Outreach}
	if intent.Historical {
		body["participation"] = "historical"
	}
	return body
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
		if obligation["case_id"] != caseID || obligation["status"] == "informational" || obligation["basis"] == "tracking_without_deadline" ||
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
		if c.last != nil && !c.last.Inactive {
			caseByID[c.ID] = c
		}
	}
	for _, flag := range flags {
		if flag["actionable"] != true {
			continue
		}
		intents := flag["intents"].([]map[string]interface{})
		if len(intents) == 0 {
			continue
		}
		anchor := ""
		for _, intent := range intents {
			if caseByID[intent["case_id"].(string)] != nil && intent["participation"] != "historical" {
				anchor = intent["case_id"].(string)
				break
			}
		}
		if anchor == "" {
			continue
		}
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
		if c.last == nil || c.last.Inactive {
			continue
		}
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

func (s *apiServer) personHistoryReferences(p *personProjection, cluster *personCluster) []map[string]interface{} {
	out := []map[string]interface{}{}
	person := personIDForRegistrations(cluster.registrations)
	members := map[string]bool{}
	for _, registration := range cluster.registrations {
		members[registration] = true
	}
	for _, id := range p.historical[person] {
		row, c := p.catalog[id], s.cases[id]
		decisions := []map[string]interface{}{}
		s.mu.Lock()
		historical := *c
		s.mu.Unlock()
		historical.decisions = nil
		programmes := []string{}
		seenProgrammes := map[string]bool{}
		for i, ctx := range row.History {
			if ctx.Association.Status != "resolved" || !members[ctx.Association.Registration] {
				continue
			}
			d := c.decisions[i]
			ref := map[string]interface{}{"decision_id": d.ID, "sig": d.Sig, "in_seq": d.InSeq, "facts_seq": ctx.FactsSeq, "inactive": d.Inactive}
			if personProgrammeRank(d.Prog) < len(personProgrammes) {
				ref["programme"] = d.Prog
				if !seenProgrammes[d.Prog] {
					programmes = append(programmes, d.Prog)
					seenProgrammes[d.Prog] = true
				}
			}
			decisions = append(decisions, ref)
			historical.decisions = append(historical.decisions, d)
		}
		intents := []map[string]interface{}{}
		for _, intent := range foldPersonIntents(&historical) {
			intent.Historical = true
			body := intentBody(intent)
			// References carry an operational summary, not new downstream identity
			// aliases. The unchanged original object stays on the authorized case.
			summary := map[string]interface{}{"kind": intent.Outreach.Kind, "ev": intent.Outreach.Event, "basis": intent.Outreach.Basis, "p_cal": intent.Outreach.PCal, "released": intent.Outreach.Released}
			if intent.Outreach.Suppress != "" {
				summary["suppress"] = intent.Outreach.Suppress
			}
			if intent.Outreach.Guard != "" {
				summary["guard"] = intent.Outreach.Guard
			}
			body["outreach"] = summary
			intents = append(intents, body)
		}
		ref := map[string]interface{}{"case_id": id, "lifecycle": row.Lifecycle, "availability": "reference_only", "history_access": "authorized_case_audit", "actionable": false,
			"programmes": programmes, "decisions": decisions, "outreach_intents": intents}
		if row.ParentCaseID != "" {
			ref["parent_case_id"] = row.ParentCaseID
		}
		out = append(out, ref)
	}
	return out
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
		if projection.artifactSchema == retainedIdentitySchema {
			// /2 current detail is deliberately latest-only. Old subject decisions
			// remain separately provenance-scoped references, never this journey's
			// current clinical/outreach history after same-ID reassociation.
			s.mu.Lock()
			current := *c
			s.mu.Unlock()
			current.decisions = nil
			row := projection.catalog[caseID]
			latest := row.History[len(row.History)-1].Association
			for _, registration := range cluster.registrations {
				if latest.Status == "resolved" && latest.Registration == registration {
					current.decisions = []*Decision{c.last}
					break
				}
			}
			c = &current
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
			delete(summary, "api_patient")
		}
		if projection.artifactSchema == retainedIdentitySchema && len(c.decisions) == 0 {
			for _, key := range []string{"links", "holds", "outreach", "decisions", "timeline", "identities"} {
				allowed[key] = []interface{}{}
			}
		}
		overlay, _ := s.apiContactOverlay(c, viewer)
		if projection.artifactSchema == retainedIdentitySchema && len(c.decisions) == 0 {
			overlay["status"] = "none"
			overlay["contact_blocked"] = true
		}
		referral := projection.referralByCase[caseID]
		parent := materializedParent(c.last)
		if referral == "" && parent != caseID {
			referral = projection.referralByCase[parent]
		}
		if referral == "" {
			return nil, fmt.Errorf("case has no validated referral registration")
		}
		body := map[string]interface{}{"case_id": caseID, "referral_registration": pseudonymizeRegistration(referral), "detail": allowed,
			"contact": map[string]interface{}{"canonical": c.Consent, "overlay_status": overlay["status"],
				"contact_blocked": overlay["contact_blocked"] == true || c.Consent != "granted", "canonical_unchanged": true}}
		if parent != caseID {
			body["parent_case_id"] = parent
		}
		if projection.artifactSchema == retainedIdentitySchema {
			body["lifecycle"] = "active"
			body["history_availability"] = "reference_only"
			if parent := projection.catalog[caseID].ParentCaseID; parent != "" {
				body["parent_case_id"] = parent
			}
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
	allCaseIDs := append([]string{}, cluster.caseIDs...)
	references := s.personHistoryReferences(projection, cluster)
	for _, ref := range references {
		id := ref["case_id"].(string)
		found := false
		for _, current := range allCaseIDs {
			found = found || current == id
		}
		if !found {
			allCaseIDs = append(allCaseIDs, id)
		}
		if last := s.cases[id].last; last != nil && last.Now > asOf {
			asOf = last.Now
		}
	}
	if len(allCaseIDs) == 0 {
		return nil, fmt.Errorf("person has no validated participation")
	}
	current := map[string]*Decision{}
	for _, c := range cases {
		current[c.ID] = c.last
	}
	body := map[string]interface{}{"schema": personJourneySchema, "materialization": materializationMetadata(current),
		"person": map[string]interface{}{"id": personIDForRegistrations(cluster.registrations), "anchor_case_id": allCaseIDs[0],
			"as_of_day": round4(asOf), "case_ids": allCaseIDs, "programmes": programmes,
			"registrations": registrations,
			"identity_provenance": map[string]interface{}{"source": "canonical_linker_clusters", "artifact_schema": projection.artifactSchema,
				"artifact_sha256": projection.artifactSHA, "policy": projection.policy}},
		"cases": caseBodies, "obligations": obligations, "outreach_flags": flags, "next_action": next}
	if projection.artifactSchema == retainedIdentitySchema {
		body["schema"] = "wasl-person-journey/2"
		body["retained_references"] = references
		if s.dashboard["materialization"] == nil && s.report["materialization"] == nil {
			// Legacy root-only retained exports did not declare CRC materialization.
			// Preserve their exact payloads; never rewrite frozen export digests.
			delete(body, "materialization")
		}
	}
	generic, err := exportGeneric(body)
	if err != nil {
		return nil, err
	}
	return pseudonymizeRegistrationFields(generic, "").(map[string]interface{}), nil
}
