//go:build linux && cgo && jack

package main

import (
	"testing"

	"github.com/xthexder/go-jack"
)

func TestJACKClientOpenAcceptsAdvisoryStatusWithClient(t *testing.T) {
	client := &jack.Client{}
	if !jackClientOpenSucceeded(client, jack.NameNotUnique) {
		t.Fatal("renamed JACK client was treated as an open failure")
	}
	if jackClientOpenSucceeded(nil, 0) {
		t.Fatal("nil JACK client was treated as an open success")
	}
}
