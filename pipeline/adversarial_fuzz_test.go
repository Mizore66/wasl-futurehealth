package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func FuzzNormaliseNeverPanics(f *testing.F) {
	for _, seed := range []string{
		`null`, `{}`, `[]`, `{"resourceType":"Provenance"}`,
		`{"resourceType":"Provenance","extension":[null]}`,
		`{"resourceType":"ServiceRequest","category":[null],"contained":[{"identifier":[]}]}`,
		`{"resourceType":"DiagnosticReport","code":{"coding":[null]},"contained":[]}`,
		`{"resourceType":"Communication","payload":[{"contentString":"ok"}]}`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > adversarialHookLimit {
			return
		}
		e, so := normalise("p1", 0, []byte(raw))
		if e != nil && so != nil {
			t.Fatal("one resource produced both an event and a sign-off")
		}
		if e != nil {
			switch e.RType {
			case "ServiceRequest", "Communication", "Task", "Appointment", "DiagnosticReport", "Encounter", "Consent":
			default:
				t.Fatalf("empty or unsupported resource type accepted: %q", e.RType)
			}
		}
	})
}

func FuzzHL7ParseAndResourceNeverPanics(f *testing.F) {
	f.Add([]byte(validHL7Header() + "\r"))
	f.Add([]byte(validHL7Header() + "\rZWS|bad|not-base64|1\r"))
	f.Add([]byte("noise\rMSH|short\r"))
	f.Add(bytes.Repeat([]byte{'x'}, 128))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > adversarialMLLPLimit {
			return
		}
		m, err := ParseHL7(raw)
		if err != nil {
			return
		}
		prov, mid, res, err := m.ToResource()
		if err != nil {
			return
		}
		if prov == "" || mid == "" || res == nil || gs(res, "resourceType") == "" {
			t.Fatalf("incomplete resource accepted: prov=%q id=%q resource=%v", prov, mid, res)
		}
	})
}

func FuzzMLLPBoundedFrame(f *testing.F) {
	f.Add([]byte("noise" + string(mllpFrame([]byte(validHL7Header()+"\r")))))
	f.Add([]byte{mllpStart, 'M', 'S', 'H', mllpEnd1, 'X'})
	f.Add([]byte("unterminated"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 2*adversarialMLLPLimit {
			return
		}
		body, err := readMLLP(bufio.NewReader(bytes.NewReader(raw)))
		if err == nil && len(body) > adversarialMLLPLimit {
			t.Fatalf("frame exceeded limit: %d", len(body))
		}
	})
}

func FuzzSubscriptionStatusNeverPanics(f *testing.F) {
	f.Add(validSubscriptionStatus)
	f.Add(`null`)
	f.Add(`{"resourceType":"Parameters","parameter":[null]}`)
	f.Add(`{"resourceType":"Parameters","parameter":[{"name":"notification-event","part":[null]}]}`)
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > adversarialHookLimit {
			return
		}
		st, ok := parseStatus(json.RawMessage(raw))
		if !ok {
			return
		}
		if st.Sub == "" || st.Status == "" || st.Type == "" || st.EventsSince < 0 {
			t.Fatalf("incomplete status accepted: %+v", st)
		}
		for _, ev := range st.Events {
			typ, id, _ := splitFocus(ev.Focus)
			if ev.Num <= 0 || ev.Occurred == "" || typ == "" || id == "" {
				t.Fatalf("invalid event accepted: %+v", ev)
			}
		}
	})
}

func FuzzAPIReviewBodyNeverPanics(f *testing.F) {
	for _, seed := range []string{
		`{"decision":"same"}`,
		`{"decision":"same","decision":"different"}`,
		`{"decision":"same","Decision":"different"}`,
		`{"decision":"same","unexpected":true}`,
		`{"decision":"same"} {}`,
		`{"decision":"same"} ` + strings.Repeat(" ", 32),
		`null`, `[]`, `{`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > adversarialReviewLimit+1024 {
			return
		}
		var body struct {
			Item     string `json:"item"`
			Decision string `json:"decision"`
			Note     string `json:"note"`
		}
		req := httptest.NewRequest(http.MethodPost, "/api/cases/"+undoCaseA+"/review", strings.NewReader(raw))
		rec := httptest.NewRecorder()
		err := decodeAPIBody(rec, req, adversarialReviewLimit, &body)
		if len(raw) > adversarialReviewLimit && err == nil {
			t.Fatal("body beyond the configured limit was accepted")
		}
	})
}
