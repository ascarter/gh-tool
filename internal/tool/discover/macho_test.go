package discover

import (
	"runtime"
	"testing"
)

func TestMachOArchsHostBinary(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O test requires a darwin host with /usr/bin/true")
	}
	archs, err := MachOArchs("/usr/bin/true")
	if err != nil {
		t.Fatalf("MachOArchs(/usr/bin/true): %v", err)
	}
	if len(archs) == 0 {
		t.Fatal("expected at least one Mach-O arch in /usr/bin/true")
	}
	// /usr/bin/true on modern macOS is universal (amd64 + arm64); on
	// older systems it may be thin. Either way, the running host's arch
	// must be present.
	if !archs[runtime.GOARCH] {
		t.Errorf("expected host arch %q in MachOArchs result, got %v", runtime.GOARCH, archs)
	}
}

func TestMachOArchsNonMachO(t *testing.T) {
	// Any text file should yield an empty (no error) result.
	archs, err := MachOArchs("macho.go")
	if err != nil {
		t.Fatalf("MachOArchs(macho.go): %v", err)
	}
	if len(archs) != 0 {
		t.Errorf("expected empty arch set for non-Mach-O file, got %v", archs)
	}
}
