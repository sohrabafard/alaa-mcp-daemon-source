//go:build !windows

package instance

import "testing"

func TestAcquireRejectsSecondInstance(t *testing.T) {
	state := t.TempDir()
	first, err := Acquire("config.json", state)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := Acquire("config.json", state); err == nil {
		t.Fatal("expected second lock to fail")
	}
}
