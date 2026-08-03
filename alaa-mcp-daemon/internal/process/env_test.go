package process

import "testing"

func TestSplitEnvEntryPreservesWindowsDriveEntry(t *testing.T) {
	key, value, ok := splitEnvEntry(`=C:=C:\Work`)
	if !ok || key != "=C:" || value != `C:\Work` {
		t.Fatalf("key=%q value=%q ok=%v", key, value, ok)
	}
}

func TestSplitEnvEntryNormal(t *testing.T) {
	key, value, ok := splitEnvEntry("NAME=value=with=equals")
	if !ok || key != "NAME" || value != "value=with=equals" {
		t.Fatalf("key=%q value=%q ok=%v", key, value, ok)
	}
}
