package metrics

import (
	"errors"
	"testing"
)

func TestStatusClass(t *testing.T) {
	cases := []struct {
		code int
		want string
	}{
		{100, "1xx"},
		{200, "2xx"}, {204, "2xx"}, {299, "2xx"},
		{301, "3xx"},
		{400, "4xx"}, {404, "4xx"}, {499, "4xx"},
		{500, "5xx"}, {503, "5xx"},
	}
	for _, tc := range cases {
		if got := StatusClass(tc.code); got != tc.want {
			t.Errorf("StatusClass(%d)=%q, want %q", tc.code, got, tc.want)
		}
	}
}

func TestOutcome(t *testing.T) {
	if got := Outcome(nil); got != "ok" {
		t.Errorf("Outcome(nil)=%q, want ok", got)
	}
	if got := Outcome(errors.New("x")); got != "error" {
		t.Errorf("Outcome(err)=%q, want error", got)
	}
}
