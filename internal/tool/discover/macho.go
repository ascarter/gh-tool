package discover

import (
	"debug/macho"
	"os"
)

// machoCPUs maps recognized Mach-O CPU types to gh-tool GOARCH names.
var machoCPUs = map[macho.Cpu]string{
	macho.CpuAmd64: "amd64",
	macho.CpuArm64: "arm64",
}

// MachOArchs returns the set of architectures present in a Mach-O file at
// path, with keys "amd64" and/or "arm64". A fat (universal) binary
// contributes one entry per embedded arch; a thin Mach-O contributes one.
// Returns an empty map (no error) when the file is not a recognizable
// Mach-O — callers can use that to distinguish "non-native asset" from
// "native asset of unknown arch".
func MachOArchs(path string) (map[string]bool, error) {
	out := map[string]bool{}

	if fat, err := macho.OpenFat(path); err == nil {
		defer fat.Close()
		for _, a := range fat.Arches {
			if name, ok := machoCPUs[a.Cpu]; ok {
				out[name] = true
			}
		}
		return out, nil
	}

	f, err := macho.Open(path)
	if err != nil {
		// Not a Mach-O (or unreadable). Return empty set; let caller
		// distinguish from os.Stat errors via a separate check if it
		// cares. We swallow the macho-specific error because callers
		// typically just want "is this a darwin native binary?".
		if _, statErr := os.Stat(path); statErr != nil {
			return nil, statErr
		}
		return out, nil
	}
	defer f.Close()
	if name, ok := machoCPUs[f.Cpu]; ok {
		out[name] = true
	}
	return out, nil
}
