package admin

import (
	"reflect"
	"testing"
)

func TestParseFlags(t *testing.T) {
	got := parseFlags([]string{"--email", "a@example.com", "--name=test", "-user-id", "42"})
	want := map[string]string{
		"email":   "a@example.com",
		"name":    "test",
		"user-id": "42",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseFlags mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestParseFlagsMissingValue(t *testing.T) {
	got := parseFlags([]string{"--label"})
	if _, ok := got["label"]; ok {
		t.Fatalf("expected label to be absent when value is missing")
	}
}
