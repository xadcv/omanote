package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

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
	state, ok := parseRunState([]byte("sink=10\nremap=11\nsys=12\ndefault_sink=alsa_output.usb\n"))
	if !ok {
		t.Fatal("parseRunState() rejected keyed state")
	}
	if !state.Running || state.SinkMod != "10" || state.RemapMod != "11" || state.MicMod != "" || state.SysMod != "12" || state.SavedDefaultSink != "alsa_output.usb" {
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

func TestOmanoteModuleIDsFromShortFindsCurrentAndLegacyModules(t *testing.T) {
	modules := strings.Join([]string{
		"1\tlibpipewire-module-rt\t{}",
		"20\tmodule-null-sink\tsink_name=OmanoteMix sink_properties=device.description=OmanoteMix",
		"21\tmodule-remap-source\tsource_name=Omanote master=OmanoteMix.monitor",
		"22\tmodule-loopback\tsource=OmanoteMix.monitor sink=bluez_output.test latency_msec=40",
		"23\tmodule-loopback\tsource=bluez_output.test.monitor sink=OmanoteMix latency_msec=20",
		"24\tmodule-loopback\tsource=alsa_input.test sink=SomeOtherSink latency_msec=20",
	}, "\n")

	got := omanoteModuleIDsFromShort(modules)
	want := []string{"20", "21", "22", "23"}
	if len(got) != len(want) {
		t.Fatalf("omanoteModuleIDsFromShort() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("omanoteModuleIDsFromShort() = %#v, want %#v", got, want)
		}
	}
}

func TestClassifyRestoreErrForMissingDummySink(t *testing.T) {
	missing := fmt.Errorf("%w: auto_null", errSinkMissing)
	retry, ignore := classifyRestoreErr(missing, pipeWireDummySinkName, false)
	if retry || !ignore {
		t.Fatalf("missing sink gone: retry=%v ignore=%v, want ignore", retry, ignore)
	}

	retry, ignore = classifyRestoreErr(missing, pipeWireDummySinkName, true)
	if !retry || ignore {
		t.Fatalf("missing sink returned: retry=%v ignore=%v, want retry", retry, ignore)
	}

	other := errors.New("restore default sink auto_null: exit status 1")
	retry, ignore = classifyRestoreErr(other, pipeWireDummySinkName, false)
	if retry || ignore {
		t.Fatalf("other error: retry=%v ignore=%v, want keep", retry, ignore)
	}

	retry, ignore = classifyRestoreErr(nil, pipeWireDummySinkName, false)
	if retry || ignore {
		t.Fatalf("nil error: retry=%v ignore=%v, want keep", retry, ignore)
	}

	retry, ignore = classifyRestoreErr(missing, "alsa_output.usb", false)
	if retry || ignore {
		t.Fatalf("missing physical sink: retry=%v ignore=%v, want error preserved", retry, ignore)
	}
}

func TestSinkInputOwnerModuleParsesStringNumberAndNull(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "string", raw: `"536870919"`, want: "536870919"},
		{name: "number", raw: `42`, want: "42"},
		{name: "null", raw: `null`, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sinkInputOwnerModule(pactlSinkInput{OwnerModule: []byte(tt.raw)})
			if got != tt.want {
				t.Fatalf("sinkInputOwnerModule() = %q, want %q", got, tt.want)
			}
		})
	}
}
