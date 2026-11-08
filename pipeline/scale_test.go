package main

import "testing"

func TestNatsURLDefaultUnchanged(t *testing.T) {
	if got, want := natsURL("", 0), "nats://127.0.0.1:14822,nats://127.0.0.1:14823,nats://127.0.0.1:14824"; got != want {
		t.Fatalf("default NATS URL changed: %q", got)
	}
	if got, want := natsURL("", 100), "nats://127.0.0.1:14922,nats://127.0.0.1:14923,nats://127.0.0.1:14924"; got != want {
		t.Fatalf("offset NATS URL: %q", got)
	}
	ext := "nats://nats-0:4222,nats://nats-1:4222,nats://nats-2:4222"
	if got := natsURL(ext, 100); got != ext {
		t.Fatalf("WASL_NATS_URL not honoured: %q", got)
	}
}

func TestValidateNatsConfig(t *testing.T) {
	tests := []struct {
		name, rawURL, external string
		ok                     bool
	}{
		{name: "local defaults", ok: true},
		{name: "external pair", rawURL: "nats://nats-0:4222", external: "1", ok: true},
		{name: "URL only", rawURL: "nats://nats-0:4222"},
		{name: "external only", external: "1"},
		{name: "URL with external disabled", rawURL: "nats://nats-0:4222", external: "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateNatsConfig(tt.rawURL, tt.external); (err == nil) != tt.ok {
				t.Fatalf("validateNatsConfig(%q, %q) error = %v, valid = %v", tt.rawURL, tt.external, err, tt.ok)
			}
		})
	}
}

func TestNatsFirstHostPort(t *testing.T) {
	old := NatsURL
	defer func() { NatsURL = old }()
	NatsURL = "nats://nats-0:4222,nats://nats-1:4222"
	if got := natsFirstHostPort(); got != "nats-0:4222" {
		t.Fatalf("got %q", got)
	}
	NatsURL = natsURL("", 0)
	if got := natsFirstHostPort(); got != "127.0.0.1:14822" {
		t.Fatalf("got %q", got)
	}
}

func TestInGenSplit(t *testing.T) {
	old := GenSplit
	defer func() { GenSplit = old }()
	GenSplit = "eval"
	if !inGenSplit("eval") || inGenSplit("train") || inGenSplit("cal") {
		t.Fatal("eval split must keep only eval persons")
	}
	GenSplit = "all"
	for _, s := range []string{"eval", "train", "dev", "cal"} {
		if !inGenSplit(s) {
			t.Fatalf("all must keep %s", s)
		}
	}
}

func TestScaleKnobDefaults(t *testing.T) {
	if ChaosSpan != 0 || QuietTimeout.Minutes() != 5 {
		t.Fatalf("scale knobs must default to the normal-cohort behaviour: span %v timeout %v", ChaosSpan, QuietTimeout)
	}
}
