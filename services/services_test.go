package services

import (
	"strings"
	"testing"
)

var npurl = "/2010-04-01/Accounts/AC58f1e8f2b1c6b88ca90a012a4be0c279/Messages.json?PageSize=50&Page=1&PageToken=PASM1f753eba6c2942858fd0be4608ead788"

// This test doesn't really do anything
func TestOpaque(t *testing.T) {
	key := NewRandomKey()
	out, err := Opaque(npurl, key)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, npurl) {
		t.Fatal("encrypted value should not contain the input")
	}
	exp, err := Unopaque(out, key)
	if err != nil {
		t.Fatal(err)
	}
	if exp != npurl {
		t.Fatalf("expected Unopaque(Opaque(%v)) to be the same, got %v", npurl, exp)
	}
}

func TestTruncateSid(t *testing.T) {
	if TruncateSid("MM1234567") != "MM123456" {
		t.Errorf("wrong answer")
	}
	if TruncateSid("MM123456") != "MM123456" {
		t.Errorf("wrong answer")
	}
	if TruncateSid("MM12345") != "MM12345" {
		t.Errorf("wrong answer")
	}
}
