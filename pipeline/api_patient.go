package main

// Patient self-service is a separate, case-bound overlay on the shared durable
// API ledger. It records synthetic preferences and education progress only: no
// dispatch, reservation, canonical decision or cross-case identity operation.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	patientEventType       = "patient_journey"
	patientJourneySchema   = "wasl-patient-journey/1"
	patientRole            = "patient_link"
	patientSelfService     = "patient_self_service"
	maxPatientBodyBytes    = 8 << 10
	teachBackQuestionID    = "crc-next-step-v1"
	prepClearFluidsAndPrep = "clear_fluids_and_prep"
	prepEscortHome         = "escort_home"
)

var (
	patientRequestIDRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	patientKinds       = map[string]bool{"teach_back_completed": true, "contact_permission_changed": true, "slot_selected": true, "preparation_changed": true}
	patientPermissions = map[string]bool{"granted": true, "denied": true, "opted_out": true}
)

// This contract contains only CRC teach-back, colonoscopy offers and preparation.
// Other programmes must not acquire links or replay CRC-specific patient events.
func patientCaseSupported(c *apiCase) bool {
	return c != nil && c.Programme == "crc"
}

type patientFieldState struct {
	Status        string
	CompletedAt   string
	UpdatedAt     string
	SelectedAt    string
	Event         string
	QuestionID    string
	OffersVersion string
	SlotID        string
}

type patientPrepState struct {
	Completed bool
	UpdatedAt string
	Event     string
}

type patientCaseState struct {
	Head    string
	Seq     int
	Teach   patientFieldState
	Contact patientFieldState
	Slot    patientFieldState
	Prep    map[string]patientPrepState
	LastAt  string
}

type patientRequestRecord struct {
	Fingerprint string
}

type patientOffer struct {
	SlotID   string `json:"slot_id"`
	Unit     string `json:"unit"`
	StartsAt string `json:"starts_at"`
}

var patientSlotOffers = []patientOffer{
	{SlotID: "skmc-endo:2026-10-09:09:30", Unit: "skmc-endo", StartsAt: "2026-10-09T09:30:00+04:00"},
	{SlotID: "skmc-endo:2026-10-08:11:00", Unit: "skmc-endo", StartsAt: "2026-10-08T11:00:00+04:00"},
	{SlotID: "skmc-endo:2026-10-12:13:00", Unit: "skmc-endo", StartsAt: "2026-10-12T13:00:00+04:00"},
}

func patientOffersVersion() string {
	ids := make([]string, 0, len(patientSlotOffers))
	for _, offer := range patientSlotOffers {
		ids = append(ids, offer.SlotID)
	}
	sort.Strings(ids)
	h := sha256.Sum256([]byte(strings.Join(ids, "\n")))
	return hex.EncodeToString(h[:])
}

func patientOfferedSlot(id string) bool {
	for _, offer := range patientSlotOffers {
		if offer.SlotID == id {
			return true
		}
	}
	return false
}

func (s *apiServer) patientRequestsLocked() map[string]patientRequestRecord {
	if s.patientRequests == nil {
		s.patientRequests = map[string]patientRequestRecord{}
	}
	return s.patientRequests
}

func patientCanonicalSignature(c *apiCase) string {
	if c.last != nil {
		return c.last.Sig
	}
	return ""
}

func patientHead(c *apiCase) string {
	if c.patient.Head != "" {
		return c.patient.Head
	}
	if sig := patientCanonicalSignature(c); sig != "" {
		return sig
	}
	return "genesis"
}

func initialPatientPrep() map[string]patientPrepState {
	return map[string]patientPrepState{prepClearFluidsAndPrep: {}, prepEscortHome: {}}
}

func (s *apiServer) patientViewLocked(c *apiCase, full bool) map[string]interface{} {
	p := &c.patient
	if p.Prep == nil {
		p.Prep = initialPatientPrep()
	}
	version := patientOffersVersion()
	ledger := "ok"
	if s.ledgerErr != "" {
		ledger = "invalid"
	}
	out := map[string]interface{}{
		"schema": patientJourneySchema, "head": patientHead(c), "seq": p.Seq, "ledger_status": ledger,
		"last_action_at": p.LastAt,
	}
	if !full {
		out["teach_back"] = map[string]interface{}{"status": patientOr(p.Teach.Status, "pending")}
		out["contact_permission"] = map[string]interface{}{"status": patientOr(p.Contact.Status, "unknown")}
		out["slot_selection"] = map[string]interface{}{"status": patientOr(p.Slot.Status, "unselected"), "slot_id": p.Slot.SlotID}
	} else {
		out["teach_back"] = map[string]interface{}{
			"question_id": patientOr(p.Teach.QuestionID, teachBackQuestionID), "status": patientOr(p.Teach.Status, "pending"),
			"completed_at": p.Teach.CompletedAt, "event": p.Teach.Event,
		}
		out["contact_permission"] = map[string]interface{}{
			"status": patientOr(p.Contact.Status, "unknown"), "updated_at": p.Contact.UpdatedAt, "event": p.Contact.Event,
		}
		out["slot_selection"] = map[string]interface{}{
			"status": patientOr(p.Slot.Status, "unselected"), "offers_version": patientOr(p.Slot.OffersVersion, version),
			"slot_id": p.Slot.SlotID, "selected_at": p.Slot.SelectedAt, "event": p.Slot.Event,
		}
	}
	items := []map[string]interface{}{}
	completed := 0
	for _, id := range []string{prepClearFluidsAndPrep, prepEscortHome} {
		item := p.Prep[id]
		if item.Completed {
			completed++
		}
		row := map[string]interface{}{"item_id": id, "completed": item.Completed}
		if full {
			row["updated_at"], row["event"] = item.UpdatedAt, item.Event
		}
		items = append(items, row)
	}
	out["preparation"] = map[string]interface{}{"items": items, "completed": completed, "total": 2}
	return out
}

func patientOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func patientOffersView() map[string]interface{} {
	return map[string]interface{}{"version": patientOffersVersion(), "slots": patientSlotOffers}
}

func patientPublicEvent(ev map[string]interface{}) map[string]interface{} {
	if ev == nil {
		return nil
	}
	return map[string]interface{}{"hash": ev["hash"], "kind": ev["kind"], "at": ev["at"], "seq": ev["seq"], "data": ev["data"]}
}

func patientRequestFingerprint(link patientLink, kind, head string, data map[string]interface{}) string {
	return string(canon(map[string]interface{}{"case": link.Case, "link_id": link.LinkID, "kind": kind, "expected_head": head, "data": data}))
}

func validatePatientData(kind string, in map[string]interface{}) (map[string]interface{}, error) {
	if in == nil || !patientKinds[kind] {
		return nil, errors.New("unknown patient action or missing data")
	}
	exact := func(keys ...string) error {
		if len(in) != len(keys) {
			return errors.New("data fields do not match the patient action")
		}
		for _, key := range keys {
			if _, ok := in[key]; !ok {
				return fmt.Errorf("data.%s is required", key)
			}
		}
		return nil
	}
	str := func(key string) (string, error) {
		v, ok := in[key].(string)
		if !ok || v == "" || len(v) > 160 || strings.TrimSpace(v) != v {
			return "", fmt.Errorf("data.%s must be a bounded non-empty string", key)
		}
		return v, nil
	}
	out := map[string]interface{}{}
	switch kind {
	case "teach_back_completed":
		if err := exact("question_id", "answer_id"); err != nil {
			return nil, err
		}
		q, qerr := str("question_id")
		a, aerr := str("answer_id")
		if qerr != nil || aerr != nil || q != teachBackQuestionID || a != "colonoscopy" {
			return nil, errors.New("teach-back question or answer is not correct")
		}
		out["question_id"], out["answer_id"] = q, a
	case "contact_permission_changed":
		if err := exact("permission"); err != nil {
			return nil, err
		}
		p, err := str("permission")
		if err != nil || !patientPermissions[p] {
			return nil, errors.New("permission must be granted|denied|opted_out")
		}
		out["permission"] = p
	case "slot_selected":
		if err := exact("slot_id", "offers_version"); err != nil {
			return nil, err
		}
		id, err1 := str("slot_id")
		version, err2 := str("offers_version")
		if err1 != nil || err2 != nil {
			return nil, errors.New("slot_id and offers_version are required")
		}
		out["slot_id"], out["offers_version"] = id, version
	case "preparation_changed":
		if err := exact("item_id", "completed"); err != nil {
			return nil, err
		}
		id, err := str("item_id")
		completed, ok := in["completed"].(bool)
		if err != nil || !ok || (id != prepClearFluidsAndPrep && id != prepEscortHome) {
			return nil, errors.New("preparation item or completed value is invalid")
		}
		out["item_id"], out["completed"] = id, completed
	}
	return out, nil
}

func (s *apiServer) patientSemanticNoOpLocked(c *apiCase, kind string, data map[string]interface{}) bool {
	switch kind {
	case "teach_back_completed":
		return c.patient.Teach.Status == "completed"
	case "contact_permission_changed":
		overlay, _ := s.overlayLocked(c)
		return c.patient.Contact.Status == data["permission"] && overlay["status"] == data["permission"]
	case "slot_selected":
		return c.patient.Slot.Status == "selected" && c.patient.Slot.SlotID == data["slot_id"]
	case "preparation_changed":
		return c.patient.Prep[data["item_id"].(string)].Completed == data["completed"].(bool)
	}
	return false
}

func (c *apiCase) applyPatientEvent(ev map[string]interface{}) {
	kind, _ := ev["kind"].(string)
	data, _ := ev["data"].(map[string]interface{})
	at, hash := sstr(ev, "at"), sstr(ev, "hash")
	p := &c.patient
	if p.Prep == nil {
		p.Prep = initialPatientPrep()
	}
	switch kind {
	case "teach_back_completed":
		p.Teach = patientFieldState{QuestionID: teachBackQuestionID, Status: "completed", CompletedAt: at, Event: hash}
	case "contact_permission_changed":
		p.Contact = patientFieldState{Status: sstr(data, "permission"), UpdatedAt: at, Event: hash}
	case "slot_selected":
		p.Slot = patientFieldState{Status: "selected", OffersVersion: sstr(data, "offers_version"), SlotID: sstr(data, "slot_id"), SelectedAt: at, Event: hash}
	case "preparation_changed":
		p.Prep[sstr(data, "item_id")] = patientPrepState{Completed: data["completed"].(bool), UpdatedAt: at, Event: hash}
	}
	p.Head, p.Seq, p.LastAt = hash, int(ev["seq"].(float64)), at
}

func patientConsentDecision(permission string) string { return "consent_" + permission }

func (s *apiServer) patientConflictLocked(c *apiCase, conflict, message string) (int, map[string]interface{}) {
	return http.StatusConflict, map[string]interface{}{"error": message, "conflict": conflict, "current_head": patientHead(c), "journey": s.patientViewLocked(c, true)}
}

func (s *apiServer) patientJourney(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		writeJSONResp(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed", "allow": "GET, POST"})
		return
	}
	link, failure := s.patientCredential(r)
	if failure != 0 {
		if failure == http.StatusUnauthorized {
			w.Header().Set("WWW-Authenticate", `Bearer realm="wasl-synthetic-patient"`)
		}
		message := "patient link is missing or invalid"
		if failure == http.StatusGone {
			message = "patient link has expired"
		}
		writeJSONResp(w, failure, map[string]interface{}{"error": message})
		return
	}
	// Browser-supplied identity headers are never authority on this route. The
	// server-resolved registry record is the only case and actor context used.
	r.Header.Del("X-Wasl-Case")
	r.Header.Set("X-Wasl-Role", patientRole)
	r.Header.Set("X-Wasl-Actor", "patient-link/"+link.LinkID)
	if r.Method == http.MethodGet {
		s.mu.Lock()
		body := s.envelope(map[string]interface{}{"journey": s.patientViewLocked(s.cases[link.Case], true), "offers": patientOffersView(), "expires_at": link.ExpiresAt})
		s.mu.Unlock()
		writeJSONResp(w, http.StatusOK, body)
		return
	}
	s.postPatientJourney(w, r, link)
}

func (s *apiServer) postPatientJourney(w http.ResponseWriter, r *http.Request, link patientLink) {
	var req struct {
		Kind         string                 `json:"kind"`
		ExpectedHead string                 `json:"expected_head"`
		RequestID    string                 `json:"request_id"`
		Data         map[string]interface{} `json:"data"`
	}
	if err := decodeAPIBody(w, r, maxPatientBodyBytes, &req); err != nil || !patientKinds[req.Kind] || req.ExpectedHead == "" || len(req.ExpectedHead) > 128 || !patientRequestIDRE.MatchString(req.RequestID) {
		writeJSONResp(w, http.StatusBadRequest, s.envelope(map[string]interface{}{"error": "body must be one strict patient action with kind, expected_head, a UUID request_id and exact data"}))
		return
	}
	data, err := validatePatientData(req.Kind, req.Data)
	if err != nil {
		writeJSONResp(w, http.StatusBadRequest, s.envelope(map[string]interface{}{"error": err.Error()}))
		return
	}
	c := s.cases[link.Case]
	code, body := func() (int, map[string]interface{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.ledgerErr != "" {
			return http.StatusInternalServerError, map[string]interface{}{"error": "review ledger failed validation; writes refused"}
		}
		fingerprint := patientRequestFingerprint(link, req.Kind, req.ExpectedHead, data)
		if previous, found := s.patientRequestsLocked()[req.RequestID]; found {
			if previous.Fingerprint != fingerprint {
				return s.patientConflictLocked(c, "request", "request_id was already used for a different patient action")
			}
			return http.StatusOK, map[string]interface{}{"event": nil, "journey": s.patientViewLocked(c, true), "replayed": true}
		}
		if req.ExpectedHead != patientHead(c) {
			return s.patientConflictLocked(c, "head", "stale expected_head; review the current patient journey before retrying")
		}
		if req.Kind == "slot_selected" {
			if data["offers_version"] != patientOffersVersion() {
				return s.patientConflictLocked(c, "offers", "the offered times changed; review the current offers")
			}
			if !patientOfferedSlot(sstr(data, "slot_id")) {
				return http.StatusBadRequest, map[string]interface{}{"error": "slot_id is not in the current offer set"}
			}
		}
		if req.Kind == "contact_permission_changed" {
			overlay, _ := s.overlayLocked(c)
			if overlay["status"] == "opted_out" && data["permission"] != "opted_out" {
				return s.patientConflictLocked(c, "consent", "contact permission is terminal after opt-out")
			}
			// A staff opt-out does not record the patient's own first choice. The common
			// replay check below distinguishes that write from a true patient repeat.
		}
		requestOnly := s.patientSemanticNoOpLocked(c, req.Kind, data)
		action := patientSelfService
		ev := map[string]interface{}{
			"type": patientEventType, "kind": req.Kind, "case": link.Case, "link_id": link.LinkID,
			"role": patientRole, "actor": "patient-link/" + link.LinkID, "action": action,
			"at": s.clock().Format(time.RFC3339Nano), "mode": "demo", "request_id": req.RequestID,
			"head": req.ExpectedHead, "head_signed": patientCanonicalSignature(c), "seq": c.patient.Seq + 1,
			"data": data, "prev": s.lastHashLocked(),
		}
		if requestOnly {
			// Bind successful no-op request IDs durably without turning their
			// intentions into new patient or consent mutations during replay.
			ev["request_only"], ev["seq"] = true, c.patient.Seq
		} else if req.Kind == "contact_permission_changed" {
			ev["action"] = ActRecordConsent
			ev["decision"] = patientConsentDecision(sstr(data, "permission"))
		}
		stored, err := s.appendEventLocked(ev)
		if err != nil {
			return http.StatusInternalServerError, map[string]interface{}{"error": "could not append to the review ledger; nothing was recorded"}
		}
		s.patientRequestsLocked()[req.RequestID] = patientRequestRecord{Fingerprint: fingerprint}
		if requestOnly {
			return http.StatusOK, map[string]interface{}{"event": nil, "journey": s.patientViewLocked(c, true), "replayed": true}
		}
		c.applyPatientEvent(stored)
		return http.StatusCreated, map[string]interface{}{"event": patientPublicEvent(stored), "journey": s.patientViewLocked(c, true), "replayed": false}
	}()
	writeJSONResp(w, code, s.envelope(body))
}

var patientEventFields = map[string]bool{
	"type": true, "kind": true, "case": true, "link_id": true, "role": true, "actor": true, "action": true,
	"at": true, "mode": true, "request_id": true, "head": true, "head_signed": true, "seq": true,
	"data": true, "prev": true, "hash": true, "decision": true, "request_only": true,
}

// replayPatientEvent validates a historical patient mutation or no-op receipt.
// Receipts bind request IDs only; they never advance the patient head or consent.
// Link expiry is deliberately absent: this is ledger history, not a new login.
func (s *apiServer) replayPatientEvent(ev map[string]interface{}) error {
	for key := range ev {
		if !patientEventFields[key] {
			return fmt.Errorf("unknown patient event field %q", key)
		}
	}
	kind, id := sstr(ev, "kind"), sstr(ev, "case")
	c := s.cases[id]
	if c == nil {
		return errors.New("patient event for unknown case")
	}
	if !patientCaseSupported(c) {
		return errors.New("patient event for unsupported non-CRC case")
	}
	linkID := sstr(ev, "link_id")
	if !patientIDRE.MatchString(linkID) || sstr(ev, "role") != patientRole || sstr(ev, "actor") != "patient-link/"+linkID || sstr(ev, "mode") != "demo" {
		return errors.New("invalid patient event identity")
	}
	if _, err := time.Parse(time.RFC3339Nano, sstr(ev, "at")); err != nil {
		return errors.New("bad patient event timestamp")
	}
	// Canonical decisions can legitimately advance after a patient action. Its
	// signed evidence must belong to this case's retained history, not necessarily
	// be the latest decision. The first mutation roots at that historical signature;
	// subsequent mutations must still follow the patient-only head exactly.
	signature := sstr(ev, "head_signed")
	knownHead := false
	for _, d := range c.decisions {
		knownHead = knownHead || signature != "" && d.Sig == signature
	}
	if !knownHead {
		return errors.New("patient event signed against a head the case never had")
	}
	head := c.patient.Head
	if head == "" {
		head = signature
	}
	if sstr(ev, "head") != head {
		return errors.New("patient event does not follow the patient-only head")
	}
	requestOnly := false
	if value, present := ev["request_only"]; present {
		if value != true {
			return errors.New("invalid patient request-only marker")
		}
		requestOnly = true
	}
	wantSeq := c.patient.Seq + 1
	if requestOnly {
		wantSeq = c.patient.Seq
	}
	seq, ok := ev["seq"].(float64)
	if !ok || seq != float64(wantSeq) {
		return errors.New("bad patient event sequence")
	}
	if !patientRequestIDRE.MatchString(sstr(ev, "request_id")) {
		return errors.New("bad patient request id")
	}
	data, ok := ev["data"].(map[string]interface{})
	if !ok {
		return errors.New("patient event data must be an object")
	}
	norm, err := validatePatientData(kind, data)
	if err != nil || !reflect.DeepEqual(norm, data) {
		return errors.New("invalid or non-canonical patient event data")
	}
	expectedAction := patientSelfService
	if kind == "contact_permission_changed" {
		if !requestOnly {
			expectedAction = ActRecordConsent
			if sstr(ev, "decision") != patientConsentDecision(sstr(data, "permission")) {
				return errors.New("patient consent decision does not match permission")
			}
		}
		overlay, _ := s.overlayLocked(c)
		if overlay["status"] == "opted_out" && sstr(data, "permission") != "opted_out" {
			return errors.New("patient event attempts to append after terminal opt-out")
		}
		// A staff opt-out does not record the patient's own first choice. The common
		// semantic replay check below still rejects a true repeated patient opt-out.
	}
	if expectedAction != ActRecordConsent {
		if _, present := ev["decision"]; present {
			return errors.New("non-consent patient event or request receipt has a decision")
		}
	}
	if sstr(ev, "action") != expectedAction {
		return errors.New("patient event action does not match its kind")
	}
	if kind == "slot_selected" && (sstr(data, "offers_version") != patientOffersVersion() || !patientOfferedSlot(sstr(data, "slot_id"))) {
		return errors.New("patient event names an unoffered slot or offer version")
	}
	if s.patientSemanticNoOpLocked(c, kind, data) != requestOnly {
		return errors.New("patient event mutation or request receipt does not match current state")
	}
	link := patientLink{Case: id, LinkID: linkID}
	fingerprint := patientRequestFingerprint(link, kind, sstr(ev, "head"), data)
	requests := s.patientRequestsLocked()
	if _, duplicate := requests[sstr(ev, "request_id")]; duplicate {
		return errors.New("duplicate patient request id")
	}
	s.reviews = append(s.reviews, ev)
	if !requestOnly {
		c.applyPatientEvent(ev)
	}
	requests[sstr(ev, "request_id")] = patientRequestRecord{Fingerprint: fingerprint}
	return nil
}
