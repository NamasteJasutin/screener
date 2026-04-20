package doctor

import (
	"fmt"
	"os"
	"strings"

	"github.com/NamasteJasutin/screener/internal/core"
	"github.com/NamasteJasutin/screener/internal/persistence"
)

// ConfigStatus summarises the health of the config file.
type ConfigStatus struct {
	Path    string
	Exists  bool
	Valid   bool
	Repairs []string // human-readable description of each fix applied
	Fatal   string   // non-empty = unrecoverable (e.g. invalid JSON we can't parse)
}

// CheckAndRepairConfig loads, validates, and auto-repairs the config at path.
// Repairs are applied in-place and persisted. Set dryRun to skip writing.
func CheckAndRepairConfig(path string, dryRun bool) ConfigStatus {
	st := ConfigStatus{Path: path}

	// Check existence before loading so we can distinguish "missing" from "corrupt".
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		st.Exists = false
		if !dryRun {
			cfg := persistence.DefaultConfig()
			if saveErr := persistence.SaveAtomic(path, cfg); saveErr != nil {
				st.Fatal = "could not create default config: " + saveErr.Error()
				return st
			}
			st.Repairs = append(st.Repairs, "created default config file")
		} else {
			st.Repairs = append(st.Repairs, "config file missing (would create default)")
		}
		st.Valid = true
		return st
	}

	cfg, err := persistence.Load(path)
	if err != nil {
		// File exists but couldn't be parsed.
		st.Exists = true
		st.Valid = false
		st.Fatal = fmt.Sprintf("invalid JSON — run screener doctor --clean-start to reset: %v", err)
		return st
	}

	st.Exists = true
	changed := false

	// ── Repair: ensure at least one profile ──────────────────────────────────
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = core.DefaultProfiles()
		st.Repairs = append(st.Repairs, "added missing default profiles")
		changed = true
	}

	// ── Repair: no duplicate profile names ───────────────────────────────────
	seen := map[string]int{}
	for i := range cfg.Profiles {
		name := cfg.Profiles[i].Name
		if seen[name] > 0 {
			newName := fmt.Sprintf("%s (%d)", name, seen[name]+1)
			cfg.Profiles[i].Name = newName
			st.Repairs = append(st.Repairs, fmt.Sprintf("renamed duplicate profile %q → %q", name, newName))
			changed = true
		}
		seen[name]++
	}

	// ── Repair: active profile must exist ────────────────────────────────────
	if cfg.ActiveProfile != "" {
		found := false
		for _, p := range cfg.Profiles {
			if p.Name == cfg.ActiveProfile {
				found = true
				break
			}
		}
		if !found {
			old := cfg.ActiveProfile
			cfg.ActiveProfile = cfg.Profiles[0].Name
			st.Repairs = append(st.Repairs, fmt.Sprintf("active profile %q not found — reset to %q", old, cfg.ActiveProfile))
			changed = true
		}
	}

	// ── Repair: ensure exactly one default profile ────────────────────────────
	defaults := 0
	for _, p := range cfg.Profiles {
		if p.IsDefault {
			defaults++
		}
	}
	if defaults == 0 {
		cfg.Profiles[0].IsDefault = true
		st.Repairs = append(st.Repairs, "no default profile set — marked first profile as default")
		changed = true
	} else if defaults > 1 {
		kept := false
		for i := range cfg.Profiles {
			if cfg.Profiles[i].IsDefault {
				if kept {
					cfg.Profiles[i].IsDefault = false
					st.Repairs = append(st.Repairs, fmt.Sprintf("removed duplicate default flag from profile %q", cfg.Profiles[i].Name))
					changed = true
				} else {
					kept = true
				}
			}
		}
	}

	// ── Repair: remove known_devices with no alias and no serial ─────────────
	var clean []core.KnownDevice
	for _, kd := range cfg.KnownDevices {
		if strings.TrimSpace(kd.Alias) == "" && strings.TrimSpace(kd.Serial) == "" {
			st.Repairs = append(st.Repairs, "removed empty device entry")
			changed = true
			continue
		}
		clean = append(clean, kd)
	}
	cfg.KnownDevices = clean

	if changed && !dryRun {
		if err := persistence.SaveAtomic(path, cfg); err != nil {
			st.Fatal = "repairs found but could not save: " + err.Error()
			return st
		}
	}

	st.Valid = true
	return st
}
