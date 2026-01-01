package main

import "testing"

func TestUI_FormattingNoColor(t *testing.T) {
	colorOnStderr = false

	if got := styledStatusCode(0); got != "0" {
		t.Fatalf("styledStatusCode(0)=%q", got)
	}
	if got := styledStatusCode(200); got != "200" {
		t.Fatalf("styledStatusCode(200)=%q", got)
	}
	if got := styledStatusKey(200); got != "status_200" {
		t.Fatalf("styledStatusKey(200)=%q", got)
	}
	if got := styledCategoryKey(CategorySystemLeak); got != "category_system_leak_responses" {
		t.Fatalf("styledCategoryKey(system_leak)=%q", got)
	}
	if got := styledMarkerKey("x"); got != "marker_x" {
		t.Fatalf("styledMarkerKey(x)=%q", got)
	}
	if got := styledErrorPrefix(); got != "error:" {
		t.Fatalf("styledErrorPrefix()=%q", got)
	}
	if got := styledDetailPrefix("  prompt="); got != "  prompt=" {
		t.Fatalf("styledDetailPrefix()=%q", got)
	}
	if got := intToString(-42); got != "-42" {
		t.Fatalf("intToString(-42)=%q", got)
	}
}
