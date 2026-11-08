package main

import "testing"

func TestNATSBinaryPathStaysBesideExecutable(t *testing.T) {
	tests := []struct {
		name string
		exe  string
		want string
	}{
		{name: "standard layout", exe: "/repo/bin/wsi7", want: "/repo/bin/nats-server"},
		{name: "isolated seed layout", exe: "/repo/bin/parallel-seed/wsi7", want: "/repo/bin/parallel-seed/nats-server"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := natsBinaryPath(tt.exe); got != tt.want {
				t.Fatalf("natsBinaryPath(%q) = %q, want %q", tt.exe, got, tt.want)
			}
		})
	}
}
