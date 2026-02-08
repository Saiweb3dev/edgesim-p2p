package main

import "testing"

func TestSplitAndTrim(t *testing.T) {
	items := splitAndTrim(" a, b , ,c ")
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0] != "a" || items[1] != "b" || items[2] != "c" {
		t.Fatalf("unexpected items: %v", items)
	}
}
