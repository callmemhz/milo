package auth

import "testing"

func TestGenerateProducesUnique(t *testing.T) {
	a, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Generate()
	if a == b {
		t.Fatal("expected uniqueness")
	}
	if len(a) < 32 {
		t.Fatalf("token too short: %d", len(a))
	}
}

func TestHashIsDeterministic(t *testing.T) {
	h1 := Hash("foo")
	h2 := Hash("foo")
	if h1 != h2 {
		t.Fatal("hash not deterministic")
	}
	if Hash("foo") == Hash("bar") {
		t.Fatal("collision")
	}
}
