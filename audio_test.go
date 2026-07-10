package main

import "testing"

func TestSelectableDevicesIncludeNoneFirst(t *testing.T) {
	devices := selectableDevices([]AudioDevice{
		{Name: "alsa_input.usb", Description: "USB Mic"},
	})

	if len(devices) != 2 {
		t.Fatalf("selectableDevices() len = %d, want 2", len(devices))
	}
	if devices[0].Name != noDeviceName || devices[0].Description != "None" {
		t.Fatalf("first device = %#v, want None sentinel", devices[0])
	}
}

func TestFindDeviceCanRestoreNoneSelection(t *testing.T) {
	devices := selectableDevices([]AudioDevice{
		{Name: "alsa_input.usb", Description: "USB Mic"},
	})

	got := findDevice(devices, noDeviceName, "alsa_input.usb")
	if got != 0 {
		t.Fatalf("findDevice() = %d, want None at index 0", got)
	}
}

func TestParseRunStateKeyedOptionalLoopbacks(t *testing.T) {
	state, ok := parseRunState([]byte("sink=10\nremap=11\nsys=12\n"))
	if !ok {
		t.Fatal("parseRunState() rejected keyed state")
	}
	if !state.Running || state.SinkMod != "10" || state.RemapMod != "11" || state.MicMod != "" || state.SysMod != "12" {
		t.Fatalf("parseRunState() = %#v", state)
	}

	ids := state.moduleIDs()
	want := []string{"10", "11", "12"}
	if len(ids) != len(want) {
		t.Fatalf("moduleIDs() = %#v, want %#v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("moduleIDs() = %#v, want %#v", ids, want)
		}
	}
}

func TestParseRunStateLegacyFourLineFormat(t *testing.T) {
	state, ok := parseRunState([]byte("1\n2\n3\n4\n"))
	if !ok {
		t.Fatal("parseRunState() rejected legacy state")
	}
	if !state.Running || state.SinkMod != "1" || state.RemapMod != "2" || state.MicMod != "3" || state.SysMod != "4" {
		t.Fatalf("parseRunState() = %#v", state)
	}
}

func TestModuleLoadedMatchesModuleIDField(t *testing.T) {
	modules := "12\tmodule-null-sink\targument\n112\tmodule-loopback\targument\n"
	if !moduleLoaded(modules, "12") {
		t.Fatal("moduleLoaded() did not find exact module ID")
	}
	if moduleLoaded(modules, "2") {
		t.Fatal("moduleLoaded() matched a partial module ID")
	}
}
