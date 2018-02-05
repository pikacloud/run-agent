package main

import (
	"testing"
)

func TestSemverParse(t *testing.T) {
	var versionsTest = []struct {
		versionInput string
		expected     string
	}{
		{"1.0", "1.0.0"},
		{"1.0.0", "1.0.0"},
		{"17.12.0-ce", "17.12.0-ce"},
	}
	for _, versionTest := range versionsTest {
		v, err := semverParse(versionTest.versionInput)
		if err != nil {
			t.Fatalf("Cannot parse %s: %v", versionTest.versionInput, err)
		}
		if v.String() != versionTest.expected {
			t.Errorf("want %s, got %s", versionTest.expected, v)
		}
	}

}
