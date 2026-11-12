package main

import "testing"

// The tracked asset is exactly the derivation of the pinned fixture template: no native output is read.
func TestTrackedAssetMatchesDerivation(t *testing.T) {
	if err := run("../../..", false); err != nil {
		t.Fatal(err)
	}
}
