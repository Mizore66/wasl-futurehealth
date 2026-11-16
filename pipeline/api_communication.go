package main

// Staff-assisted result notification and receipt acknowledgement (issue #197).
//
// A coordinator who tells a patient their screening result needs, by phone, video or in person, and later hears the
// patient confirm they received it, records each step as its own coordinator action:
//
//	initial_notification     {occurred_at, channel, template, identity_check}
//	receipt_acknowledgement  {occurred_at, channel, identity_check, notification_event}
//
// Each is one hash-chained event on the shared review ledger, replayed on every load. The served case then answers the
// communication layer of closure.go with the recorded fact folded in: the drop-off is recomputed with the engine's own
// dropoffCode and the engine's missing-notification / missing-acknowledgement escalations are hidden once a recorded
// fact answers them. The canonical decision (state, closure, escalations, signature) is never rewritten, and nothing
// here closes a loop: notification and receipt are communication facts, not clinical evidence.
//
// Kept apart on purpose:
//   - a generic contact outcome (reached, appointment agreed) or a note never records notification or receipt;
//   - receipt is not comprehension: teach-back state and teach-back callbacks are untouched;
//   - a notification must name the programme's approved notification wording in the patient's recorded language.

import (
	"errors"
	"strings"
	"time"
)

const (
	notificationKind = "initial_notification"
	receiptKind      = "receipt_acknowledgement"
	// canonicalNotification is the notification_event a receipt names when the engine already saw a compliant
	// notification Communication in the source record (Decision.NotifDay >= 0).
	canonicalNotification = "canonical"
	communicationConflict = "communication"
)

var (
	communicationChannels = map[string]bool{"phone": true, "video": true, "in_person": true}
	// identityChecks are the two-identifier checks a coordinator confirms before disclosing anything to the caller.
	identityChecks = map[string]bool{"name_and_dob": true, "name_and_emirates_id": true, "name_and_mrn": true}
)

func init() {
	actionFields[notificationKind] = []string{"occurred_at", "channel", "template", "identity_check"}
	actionFields[receiptKind] = []string{"occurred_at", "channel", "identity_check", "notification_event"}
}

func isCommunicationKind(kind string) bool { return kind == notificationKind || kind == receiptKind }

// validateCommunicationData checks the field shapes (shared by the write path and ledger replay). Case-dependent rules
// (programme template, language, order, consent, identity) live in communicationPreconditionLocked.
func validateCommunicationData(kind string, out map[string]interface{}, text func(k string, max int, required, multi bool) error) error {
	for _, f := range []struct {
		k        string
		max      int
		required bool
	}{{"occurred_at", 40, true}, {"channel", 16, true}, {"identity_check", 32, true}} {
		if err := text(f.k, f.max, f.required, false); err != nil {
			return err
		}
	}
	at, err := time.Parse(time.RFC3339, out["occurred_at"].(string))
	if err != nil {
		return errors.New("occurred_at must be an RFC 3339 timestamp")
	}
	out["occurred_at"] = at.UTC().Format(time.RFC3339)
	if !communicationChannels[out["channel"].(string)] {
		return errors.New("channel must be one of phone|video|in_person (staff-assisted only; no message is sent)")
	}
	if !identityChecks[out["identity_check"].(string)] {
		return errors.New("identity_check must be one of name_and_dob|name_and_emirates_id|name_and_mrn")
	}
	if kind == notificationKind {
		if err := text("template", 64, true, false); err != nil {
			return err
		}
		if t, ok := Templates[out["template"].(string)]; !ok || t.Kind != "notify" {
			return errors.New("template must be an approved result-notification template id")
		}
		return nil
	}
	return text("notification_event", 64, false, false)
}

// expectedLanguage is the language the engine checks a notification against: the referral's recorded preference,
// else the sending provider's default; anything but Arabic is English (closure.go communication layer, rules.go).
func expectedLanguage(c *apiCase) string {
	lang := c.lang
	if lang == "" {
		lang = provDefaultLang[c.Provider]
	}
	if lang != "ar" {
		lang = "en"
	}
	return lang
}

// notificationTemplateFor is the approved notification template id of the case's programme family in the given version.
func notificationTemplateFor(c *apiCase, version string) string {
	fam := Rules[c.Programme].Tpl
	if fam == "" {
		return ""
	}
	return fam + "-notify-" + version + "/" + expectedLanguage(c)
}

// currentNotification is what a receipt must answer: the recorded staff notification, else the engine's notification.
// ok is false when the case has none.
func currentNotification(c *apiCase) (ref string, at time.Time, ok bool) {
	if n := c.ov.notification; n != nil {
		t, _ := time.Parse(time.RFC3339, n["occurred_at"].(string))
		return n["event"].(string), t, true
	}
	if c.last != nil && c.last.NotifDay >= 0 {
		return canonicalNotification, dayDate(c.last.NotifDay), true
	}
	return "", time.Time{}, false
}

// servedDropoffLocked is the drop-off the API serves: the canonical one, unless a recorded staff notification or receipt
// answers the communication step it names. Recomputed with the engine's dropoffCode from the decision's own facts.
func servedDropoffLocked(c *apiCase) string {
	if c.last == nil || (c.ov.notification == nil && c.ov.receipt == nil) {
		return c.Dropoff
	}
	if c.Dropoff != "never_notified" && c.Dropoff != "no_acknowledgement" {
		return c.Dropoff
	}
	notified := c.last.NotifDay >= 0 || c.ov.notification != nil
	acked := c.last.AckDay >= 0 || c.ov.receipt != nil
	return dropoffCode(c.last.State, false, c.last.RefStatus, notified, acked, c.last.RefDay >= 0)
}

func (s *apiServer) servedDropoff(c *apiCase) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return servedDropoffLocked(c)
}

// communicationAnswers reports whether a recorded staff fact answers this canonical escalation.
func communicationAnswers(c *apiCase, escalation string) bool {
	switch escalation {
	case "not_notified_21d":
		return c.ov.notification != nil
	case "no_ack_7d":
		return c.ov.receipt != nil
	}
	return false
}

func hasCommunicationRecord(c *apiCase) bool { return c.ov.notification != nil || c.ov.receipt != nil }

// communicationPreconditionLocked holds the case-state rules of both actions, shared by the write path and replay.
// `at` is the event's recording time: a communication cannot be recorded before it happened.
// communicationClockSkew tolerates a browser clock slightly ahead of the API when it stamps occurred_at at the click.
const communicationClockSkew = 2 * time.Minute

func (s *apiServer) communicationPreconditionLocked(c *apiCase, kind string, data map[string]interface{}, at time.Time) *actionRefusal {
	if c.last == nil {
		return &actionRefusal{409, communicationConflict, "the case has no current decision"}
	}
	if !s.contactGuardLocked(c).Allowed {
		return &actionRefusal{409, "consent", "recording a patient communication needs canonical contact permission granted and no denial or opt-out"}
	}
	for _, o := range c.last.Outreach {
		if o.Suppress == "second_identifier_unconfirmed" {
			return &actionRefusal{409, "identity", "the case has an unconfirmed second identifier; resolve identity before recording a communication"}
		}
	}
	occurred, _ := time.Parse(time.RFC3339, data["occurred_at"].(string))
	if occurred.After(at.Add(communicationClockSkew)) {
		return &actionRefusal{400, "", "occurred_at must not be in the future"}
	}
	if occurred.Before(dayDate(c.SRDay)) {
		return &actionRefusal{400, "", "occurred_at must not be before the referral"}
	}
	served := servedDropoffLocked(c)
	if kind == notificationKind {
		if served != "never_notified" {
			return &actionRefusal{409, communicationConflict, "no initial result notification is outstanding on this case"}
		}
		tpl := data["template"].(string)
		if tpl != notificationTemplateFor(c, "v1") && tpl != notificationTemplateFor(c, "v2") {
			return &actionRefusal{409, "content", "template must be the approved " + c.Programme + " result-notification wording in the patient's recorded language (" + expectedLanguage(c) + ")"}
		}
		return nil
	}
	if served != "no_acknowledgement" {
		return &actionRefusal{409, communicationConflict, "no acknowledgement of a notification is outstanding on this case"}
	}
	ref, notifiedAt, ok := currentNotification(c)
	if !ok || data["notification_event"] != ref {
		return &actionRefusal{409, communicationConflict, "a receipt must answer the case's current notification"}
	}
	if occurred.Before(notifiedAt) {
		return &actionRefusal{400, "", "a receipt cannot be acknowledged before the notification"}
	}
	return nil
}

// communicationViewLocked is api_overlay.communication: the recorded facts and the obligation they leave. The summary
// form omits the recording actor and identity check, like notes.
func communicationViewLocked(c *apiCase, full bool) map[string]interface{} {
	if !hasCommunicationRecord(c) {
		return nil
	}
	pick := func(e map[string]interface{}, keys ...string) map[string]interface{} {
		out := map[string]interface{}{}
		for _, k := range keys {
			if v, ok := e[k]; ok {
				out[k] = v
			}
		}
		if full {
			for _, k := range []string{"actor", "identity_check"} {
				if v, ok := e[k]; ok {
					out[k] = v
				}
			}
		}
		return out
	}
	understanding := "none"
	if c.last != nil && c.last.TBState != "" {
		understanding = c.last.TBState
	}
	m := map[string]interface{}{"canonical_dropoff": c.Dropoff, "obligation": servedDropoffLocked(c), "understanding": understanding,
		"note": "staff-recorded communication facts; canonical decisions, closure and teach-back are unchanged"}
	if n := c.ov.notification; n != nil {
		m["notification"] = pick(n, "at", "event", "occurred_at", "channel", "template", "language")
	}
	if r := c.ov.receipt; r != nil {
		m["receipt"] = pick(r, "at", "event", "occurred_at", "channel", "notification_event")
	}
	return m
}

// applyCommunication projects one validated communication event onto the overlay.
func (c *apiCase) applyCommunication(kind string, entry map[string]interface{}) {
	if kind == notificationKind {
		if tpl, ok := entry["template"].(string); ok {
			entry["language"] = Templates[tpl].Lang
		}
		c.ov.notification = entry
		return
	}
	c.ov.receipt = entry
}

// communicationKindsList is the action list wording for error messages.
func communicationKindsList() string {
	return strings.Join([]string{notificationKind, receiptKind}, "|")
}
