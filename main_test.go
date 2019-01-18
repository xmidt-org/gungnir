package main

import (
	"testing"
)

func TestGungnir(t *testing.T) {
	code := gungnir([]string{"-v"})
	t.Logf("Recieved Code %d", code)
	if code != 0 {
		t.Error("-v should result in a 0 error code")
	}
}
