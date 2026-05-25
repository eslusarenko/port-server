package db

import "testing"

func TestHashKey(t *testing.T) {
	got := hashKey("abc")
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Fatalf("hashKey mismatch: got %q want %q", got, want)
	}
	if len(got) != 64 {
		t.Fatalf("hashKey length = %d, want 64", len(got))
	}
}

func TestHashKeyDeterministic(t *testing.T) {
	a := hashKey("same-key")
	b := hashKey("same-key")
	if a != b {
		t.Fatalf("hashKey should be deterministic: %q != %q", a, b)
	}
}
