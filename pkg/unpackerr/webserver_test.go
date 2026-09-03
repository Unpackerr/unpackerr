package unpackerr

import "testing"

func TestWebServerEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		server *WebServer
		want   bool
	}{
		{name: "nil", want: false},
		{name: "empty listen", server: &WebServer{ListenAddr: ""}, want: false},
		{name: "whitespace listen", server: &WebServer{ListenAddr: "  \t"}, want: false},
		{name: "default listen metrics off", server: &WebServer{ListenAddr: "0.0.0.0:5656"}, want: true},
		{name: "port only", server: &WebServer{ListenAddr: "5656"}, want: true},
		{name: "metrics does not gate", server: &WebServer{Metrics: true, ListenAddr: ""}, want: false},
		{name: "metrics on with listen", server: &WebServer{Metrics: true, ListenAddr: "127.0.0.1:5656"}, want: true},
		{name: "padded listen", server: &WebServer{ListenAddr: " 127.0.0.1:5656 "}, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.server.Enabled(); got != test.want {
				t.Fatalf("Enabled() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestWebServerBindAddr(t *testing.T) {
	t.Parallel()

	got := (&WebServer{ListenAddr: " 5656 "}).bindAddr()
	if got != "0.0.0.0:5656" {
		t.Fatalf("port only %q", got)
	}

	got = (&WebServer{ListenAddr: " 127.0.0.1:5659 "}).bindAddr()
	if got != "127.0.0.1:5659" {
		t.Fatalf("trimmed %q", got)
	}
}

func TestWebServerNormalizeURLBase(t *testing.T) {
	t.Parallel()

	server := &WebServer{URLBase: "custom"}
	server.normalizeURLBase()

	if server.URLBase != "/custom/" {
		t.Fatalf("urlbase %q", server.URLBase)
	}
}
