package main

import (
	"testing"
)

func TestGitLsRemote(t *testing.T) {
	references, err := inMemorylsRemote("https://github.com/bjorand/testbuilddocker.git", nil)
	if err != nil {
		t.Fatalf("Cannot list references: %v", err)
	}
	reference, err := resolveRawReference("master", references)
	if err != nil {
		t.Fatalf("Reference master not found %+v", err)
	}
	want := "refs/heads/master"
	if string(reference.Name()) != want {
		t.Fatalf("Reference name is %v, want %v", reference.Name(), want)
	}
}
