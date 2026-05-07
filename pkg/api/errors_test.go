package api

import "testing"

func TestErrorString(t *testing.T) {
	e := &Error{Code: ErrNotFound, Message: "user not found"}
	got := e.Error()
	want := "not_found: user not found"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
