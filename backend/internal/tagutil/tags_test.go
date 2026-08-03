package tagutil

import (
	"reflect"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"OpenCode":   "opencode",
		" opencode ": "opencode",
		"OPENCODE":   "opencode",
		"  ":         "",
		"":           "",
		"grok":       "grok",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeAll(t *testing.T) {
	got := NormalizeAll([]string{" OpenCode ", "opencode", "WORK", "", "work", "  ", "Grok"})
	want := []string{"opencode", "work", "grok"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeAll = %v, want %v", got, want)
	}
	// nil 入参返回空切片(非 nil),便于 JSON 序列化为 []
	if out := NormalizeAll(nil); out == nil || len(out) != 0 {
		t.Fatalf("NormalizeAll(nil) = %v, want empty non-nil slice", out)
	}
}

func TestContainsFold(t *testing.T) {
	tags := []string{"OpenCode", " grok "}
	if !ContainsFold(tags, "opencode") {
		t.Errorf("ContainsFold opencode should be true")
	}
	if !ContainsFold(tags, "GROK") {
		t.Errorf("ContainsFold GROK should be true")
	}
	if ContainsFold(tags, "vip") {
		t.Errorf("ContainsFold vip should be false")
	}
	if ContainsFold(tags, "  ") {
		t.Errorf("ContainsFold blank should be false")
	}
}
