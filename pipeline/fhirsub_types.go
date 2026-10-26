package main

// FHIR R4 + Subscriptions R5 Backport IG (hl7.fhir.uv.subscriptions-backport, R4 profiles) wire types shared by the
// mock Malaffi server (malaffi.go) and the subscription consumer (fhirsub.go).
import (
	"encoding/json"
	"strconv"
	"strings"
)

const (
	BackportBase      = "http://hl7.org/fhir/uv/subscriptions-backport/StructureDefinition/"
	ProfSubscription  = BackportBase + "backport-subscription"
	ProfStatusR4      = BackportBase + "backport-subscription-status-r4"
	ProfNotifyR4      = BackportBase + "backport-subscription-notification-r4"
	ExtFilterCriteria = BackportBase + "backport-filter-criteria"
	ExtHeartbeat      = BackportBase + "backport-heartbeat-period"
	ExtTimeout        = BackportBase + "backport-timeout"
	ExtMaxCount       = BackportBase + "backport-max-count"
	ExtPayloadContent = BackportBase + "backport-payload-content"
	// Assumed Malaffi topic canonical (no public Malaffi SubscriptionTopic exists; see SUMMARY.md mapping notes).
	MalaffiTopic = "https://malaffi.ae/fhir/SubscriptionTopic/crc-fit-loop-events"
	MockToken    = "mock-malaffi-client-token" // consumer -> Malaffi REST (stands in for an OAuth2 client-credentials token)
	HookToken    = "wsi-hook-secret-7f3a"      // Malaffi -> consumer rest-hook (Subscription.channel.header)
)

type Ref struct {
	Reference string `json:"reference"`
}

type Param struct {
	Name           string  `json:"name"`
	ValueString    string  `json:"valueString,omitempty"`
	ValueCode      string  `json:"valueCode,omitempty"`
	ValueCanonical string  `json:"valueCanonical,omitempty"`
	ValueInstant   string  `json:"valueInstant,omitempty"`
	ValueReference *Ref    `json:"valueReference,omitempty"`
	Part           []Param `json:"part,omitempty"`
}

type Parameters struct {
	ResourceType string                 `json:"resourceType"`
	ID           string                 `json:"id,omitempty"`
	Meta         map[string]interface{} `json:"meta,omitempty"`
	Parameter    []Param                `json:"parameter"`
}

type BReq struct {
	Method string `json:"method"`
	URL    string `json:"url"`
}
type BResp struct {
	Status string `json:"status"`
	Etag   string `json:"etag,omitempty"`
}
type BEntry struct {
	FullURL  string          `json:"fullUrl,omitempty"`
	Resource json.RawMessage `json:"resource,omitempty"`
	Request  *BReq           `json:"request,omitempty"`
	Response *BResp          `json:"response,omitempty"`
}
type Bundle struct {
	ResourceType string                 `json:"resourceType"`
	ID           string                 `json:"id,omitempty"`
	Meta         map[string]interface{} `json:"meta,omitempty"`
	Type         string                 `json:"type"`
	Timestamp    string                 `json:"timestamp,omitempty"`
	Entry        []BEntry               `json:"entry"`
}

// NotifEvent is one notification-event part of a SubscriptionStatus.
type NotifEvent struct {
	Num      int64
	Focus    string // Type/id/_history/vid
	Occurred string // RFC3339Nano instant the event was raised at the HIE
}

// StatusInfo is the decoded SubscriptionStatus (Parameters) at entry[0] of a notification / $status / $events bundle.
type StatusInfo struct {
	Sub         string
	Topic       string
	Status      string // requested|active|error|off
	Type        string // handshake|heartbeat|event-notification|query-status|query-event
	EventsSince int64
	Events      []NotifEvent
}

func (s StatusInfo) toParams() Parameters {
	p := Parameters{ResourceType: "Parameters", Meta: map[string]interface{}{"profile": []string{ProfStatusR4}},
		Parameter: []Param{
			{Name: "subscription", ValueReference: &Ref{s.Sub}},
			{Name: "topic", ValueCanonical: s.Topic},
			{Name: "status", ValueCode: s.Status},
			{Name: "type", ValueCode: s.Type},
			{Name: "events-since-subscription-start", ValueString: strconv.FormatInt(s.EventsSince, 10)},
		}}
	for _, e := range s.Events {
		p.Parameter = append(p.Parameter, Param{Name: "notification-event", Part: []Param{
			{Name: "event-number", ValueString: strconv.FormatInt(e.Num, 10)},
			{Name: "timestamp", ValueInstant: e.Occurred},
			{Name: "focus", ValueReference: &Ref{e.Focus}},
		}})
	}
	return p
}

func parseStatus(raw json.RawMessage) (StatusInfo, bool) {
	var p Parameters
	if json.Unmarshal(raw, &p) != nil || p.ResourceType != "Parameters" {
		return StatusInfo{}, false
	}
	var s StatusInfo
	for _, x := range p.Parameter {
		switch x.Name {
		case "subscription":
			if x.ValueReference != nil {
				s.Sub = x.ValueReference.Reference
			}
		case "topic":
			s.Topic = x.ValueCanonical
		case "status":
			s.Status = x.ValueCode
		case "type":
			s.Type = x.ValueCode
		case "events-since-subscription-start":
			s.EventsSince, _ = strconv.ParseInt(x.ValueString, 10, 64)
		case "notification-event":
			var e NotifEvent
			for _, pt := range x.Part {
				switch pt.Name {
				case "event-number":
					e.Num, _ = strconv.ParseInt(pt.ValueString, 10, 64)
				case "timestamp":
					e.Occurred = pt.ValueInstant
				case "focus":
					if pt.ValueReference != nil {
						e.Focus = pt.ValueReference.Reference
					}
				}
			}
			s.Events = append(s.Events, e)
		}
	}
	return s, true
}

// splitFocus: "Type/id/_history/vid" (possibly with a base URL prefix) -> type, id, vid
func splitFocus(f string) (string, string, string) {
	if i := strings.Index(f, "/fhir/"); i >= 0 {
		f = f[i+6:]
	}
	p := strings.Split(f, "/")
	if len(p) >= 4 && p[2] == "_history" {
		return p[0], p[1], p[3]
	}
	if len(p) >= 2 {
		return p[0], p[1], ""
	}
	return "", "", ""
}

// subIDFor is deterministic so that a restarted consumer and a fresh mock agree without extra state.
func provFromSource(src string) string { return strings.TrimPrefix(src, "urn:ae:provider:") }
