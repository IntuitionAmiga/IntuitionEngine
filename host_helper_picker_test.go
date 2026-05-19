package main

import "testing"

func TestHostWiFiPickerListNavigationAndSubmitOpenNetwork(t *testing.T) {
	picker := NewHostWiFiPicker([]HostWiFiNetwork{
		{SSID: "Home", Signal: 80, Security: "wpa2-psk"},
		{SSID: "Cafe", Signal: 62, Security: "open"},
		{SSID: "Office", Signal: 91, Security: "wpa3-sae"},
	})

	if got := picker.SelectedIndex(); got != 0 {
		t.Fatalf("initial selection = %d, want 0", got)
	}

	if action := picker.HandleInput(HostWiFiPickerInput{Key: HostWiFiPickerKeyDown}); action != HostWiFiPickerActionNone {
		t.Fatalf("down action = %d, want none", action)
	}
	if got := picker.SelectedIndex(); got != 1 {
		t.Fatalf("selection after down = %d, want 1", got)
	}

	if action := picker.HandleInput(HostWiFiPickerInput{Key: HostWiFiPickerKeyEnter}); action != HostWiFiPickerActionSubmit {
		t.Fatalf("enter on open network action = %d, want submit", action)
	}
	if got := picker.Mode(); got != HostWiFiPickerModeSubmitted {
		t.Fatalf("mode after open submit = %d, want submitted", got)
	}
	got := picker.SubmittedRequest()
	want := HostWiFiJoinRequest{SSID: "Cafe", Password: ""}
	if got != want {
		t.Fatalf("submitted request = %#v, want %#v", got, want)
	}
}

func TestHostWiFiPickerClampsListNavigation(t *testing.T) {
	picker := NewHostWiFiPicker([]HostWiFiNetwork{
		{SSID: "Home", Signal: 80, Security: "wpa2-psk"},
		{SSID: "Office", Signal: 91, Security: "wpa3-sae"},
	})

	picker.HandleInput(HostWiFiPickerInput{Key: HostWiFiPickerKeyUp})
	if got := picker.SelectedIndex(); got != 0 {
		t.Fatalf("selection after up at top = %d, want 0", got)
	}

	picker.HandleInput(HostWiFiPickerInput{Key: HostWiFiPickerKeyDown})
	picker.HandleInput(HostWiFiPickerInput{Key: HostWiFiPickerKeyDown})
	if got := picker.SelectedIndex(); got != 1 {
		t.Fatalf("selection after down at bottom = %d, want 1", got)
	}
}

func TestHostWiFiPickerPasswordEntryBackspaceAndSubmit(t *testing.T) {
	picker := NewHostWiFiPicker([]HostWiFiNetwork{
		{SSID: "Office", Signal: 91, Security: "wpa3-sae"},
	})

	if action := picker.HandleInput(HostWiFiPickerInput{Key: HostWiFiPickerKeyEnter}); action != HostWiFiPickerActionNone {
		t.Fatalf("enter on secured network action = %d, want none", action)
	}
	if got := picker.Mode(); got != HostWiFiPickerModePassword {
		t.Fatalf("mode after secured enter = %d, want password", got)
	}

	for _, ch := range "secret" {
		picker.HandleInput(HostWiFiPickerInput{Rune: ch})
	}
	picker.HandleInput(HostWiFiPickerInput{Key: HostWiFiPickerKeyBackspace})
	picker.HandleInput(HostWiFiPickerInput{Rune: '7'})

	if got := picker.Password(); got != "secre7" {
		t.Fatalf("password buffer = %q, want %q", got, "secre7")
	}

	if action := picker.HandleInput(HostWiFiPickerInput{Key: HostWiFiPickerKeyEnter}); action != HostWiFiPickerActionSubmit {
		t.Fatalf("enter after password action = %d, want submit", action)
	}
	got := picker.SubmittedRequest()
	want := HostWiFiJoinRequest{SSID: "Office", Password: "secre7"}
	if got != want {
		t.Fatalf("submitted request = %#v, want %#v", got, want)
	}
}

func TestHostWiFiPickerAllowsRawWPAHexPSK(t *testing.T) {
	picker := NewHostWiFiPicker([]HostWiFiNetwork{
		{SSID: "Office", Signal: 91, Security: "wpa2-psk"},
	})

	picker.HandleInput(HostWiFiPickerInput{Key: HostWiFiPickerKeyEnter})
	psk := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, ch := range psk {
		picker.HandleInput(HostWiFiPickerInput{Rune: ch})
	}
	picker.HandleInput(HostWiFiPickerInput{Rune: 'x'})

	if got := picker.Password(); got != psk {
		t.Fatalf("password buffer = %q, want %q", got, psk)
	}

	if action := picker.HandleInput(HostWiFiPickerInput{Key: HostWiFiPickerKeyEnter}); action != HostWiFiPickerActionSubmit {
		t.Fatalf("enter after raw PSK action = %d, want submit", action)
	}
	got := picker.SubmittedRequest()
	want := HostWiFiJoinRequest{SSID: "Office", Password: psk}
	if got != want {
		t.Fatalf("submitted request = %#v, want %#v", got, want)
	}
}

func TestHostWiFiPickerRescanAndCancel(t *testing.T) {
	picker := NewHostWiFiPicker([]HostWiFiNetwork{
		{SSID: "Home", Signal: 80, Security: "wpa2-psk"},
	})

	if action := picker.HandleInput(HostWiFiPickerInput{Key: HostWiFiPickerKeyR}); action != HostWiFiPickerActionRescan {
		t.Fatalf("R action = %d, want rescan", action)
	}
	if got := picker.Mode(); got != HostWiFiPickerModeList {
		t.Fatalf("mode after rescan request = %d, want list", got)
	}

	if action := picker.HandleInput(HostWiFiPickerInput{Key: HostWiFiPickerKeyEscape}); action != HostWiFiPickerActionCancel {
		t.Fatalf("Esc action = %d, want cancel", action)
	}
	if got := picker.Mode(); got != HostWiFiPickerModeCancelled {
		t.Fatalf("mode after Esc = %d, want cancelled", got)
	}
}

func TestHostWiFiPickerSetNetworksClampsSelection(t *testing.T) {
	picker := NewHostWiFiPicker([]HostWiFiNetwork{
		{SSID: "Home", Signal: 80, Security: "wpa2-psk"},
		{SSID: "Office", Signal: 91, Security: "wpa3-sae"},
		{SSID: "Cafe", Signal: 62, Security: "open"},
	})
	picker.HandleInput(HostWiFiPickerInput{Key: HostWiFiPickerKeyDown})
	picker.HandleInput(HostWiFiPickerInput{Key: HostWiFiPickerKeyDown})

	picker.SetNetworks([]HostWiFiNetwork{
		{SSID: "Only", Signal: 50, Security: "open"},
	})

	if got := picker.SelectedIndex(); got != 0 {
		t.Fatalf("selection after shrinking network list = %d, want 0", got)
	}
}
