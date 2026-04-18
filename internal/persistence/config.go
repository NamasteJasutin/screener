package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/NamasteJasutin/screener/internal/core"
)

type Config struct {
	Version          int                      `json:"version"`
	Profiles         []core.ProfileDefinition `json:"profiles"`
	ActiveProfile    string                   `json:"active_profile"`
	ConnectionPolicy core.ConnectionPolicy    `json:"connection_policy"`
	KnownDevices     []core.KnownDevice       `json:"known_devices,omitempty"`
	Theme            string                   `json:"theme,omitempty"`
}

func DefaultConfig() Config {
	profiles := core.DefaultProfiles()
	activeProfile := ""
	for _, profile := range profiles {
		if profile.IsDefault {
			activeProfile = profile.Name
			break
		}
	}
	if activeProfile == "" && len(profiles) > 0 {
		activeProfile = profiles[0].Name
	}
	return Config{
		Version:       1,
		Profiles:      profiles,
		ActiveProfile: activeProfile,
		ConnectionPolicy: core.ConnectionPolicy{
			PreferUSB: true,
			AllowTCP:  true,
		},
	}
}

func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".config", "screener", "config.json")
	}
	return filepath.Join(home, ".config", "screener", "config.json")
}

func DefaultProfilesExchangePath(configPath string) string {
	if strings.TrimSpace(configPath) == "" {
		configPath = DefaultConfigPath()
	}
	return filepath.Join(filepath.Dir(configPath), "profiles.json")
}

func DefaultLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".local", "state", "screener", "screener.log")
	}
	return filepath.Join(home, ".local", "state", "screener", "screener.log")
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return DefaultConfig(), err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return DefaultConfig(), fmt.Errorf("config parse error: %w", err)
	}
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = core.DefaultProfiles()
	}
	ensureBuiltInProfiles(&cfg)
	ensureSingleDefaultProfile(&cfg)
	if cfg.ActiveProfile == "" {
		for _, profile := range cfg.Profiles {
			if profile.IsDefault {
				cfg.ActiveProfile = profile.Name
				break
			}
		}
		if cfg.ActiveProfile == "" {
			cfg.ActiveProfile = cfg.Profiles[0].Name
		}
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if !profileNameExists(cfg.Profiles, cfg.ActiveProfile) {
		for _, profile := range cfg.Profiles {
			if profile.IsDefault {
				cfg.ActiveProfile = profile.Name
				break
			}
		}
		if cfg.ActiveProfile == "" {
			cfg.ActiveProfile = cfg.Profiles[0].Name
		}
	}
	return cfg, nil
}

func ExportProfiles(path string, profiles []core.ProfileDefinition) error {
	b, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(path, b, ".profiles-*.tmp"); err != nil {
		return err
	}
	return nil
}

func ImportProfiles(path string) ([]core.ProfileDefinition, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var profiles []core.ProfileDefinition
	if err := json.Unmarshal(b, &profiles); err != nil {
		return nil, fmt.Errorf("profiles parse error: %w", err)
	}

	seenIDs := map[string]struct{}{}
	seenNames := map[string]struct{}{}
	for i := range profiles {
		profiles[i].Name = strings.TrimSpace(profiles[i].Name)
		if profiles[i].Name == "" {
			return nil, fmt.Errorf("profiles validation error: profile[%d] name is required", i)
		}

		nameKey := strings.ToLower(profiles[i].Name)
		if _, exists := seenNames[nameKey]; exists {
			return nil, fmt.Errorf("profiles validation error: duplicate profile name %q", profiles[i].Name)
		}
		seenNames[nameKey] = struct{}{}

		profiles[i].ProfileID = strings.TrimSpace(profiles[i].ProfileID)
		if profiles[i].ProfileID == "" {
			continue
		}
		if _, exists := seenIDs[profiles[i].ProfileID]; exists {
			return nil, fmt.Errorf("profiles validation error: duplicate profile_id %q", profiles[i].ProfileID)
		}
		seenIDs[profiles[i].ProfileID] = struct{}{}
	}

	return profiles, nil
}

func MergeProfiles(base []core.ProfileDefinition, incoming []core.ProfileDefinition) []core.ProfileDefinition {
	merged := append([]core.ProfileDefinition(nil), base...)
	builtInSetByID := map[string]struct{}{}
	builtInSetByName := map[string]struct{}{}
	for _, builtIn := range core.DefaultProfiles() {
		if builtIn.ProfileID != "" {
			builtInSetByID[builtIn.ProfileID] = struct{}{}
		}
		if builtIn.Name != "" {
			builtInSetByName[strings.ToLower(builtIn.Name)] = struct{}{}
		}
	}

	for _, imported := range incoming {
		imported.Name = strings.TrimSpace(imported.Name)
		imported.ProfileID = strings.TrimSpace(imported.ProfileID)
		if imported.Name == "" {
			continue
		}

		idx := -1
		if imported.ProfileID != "" {
			for i := range merged {
				if merged[i].ProfileID == imported.ProfileID {
					idx = i
					break
				}
			}
		} else {
			for i := range merged {
				if strings.EqualFold(merged[i].Name, imported.Name) {
					idx = i
					break
				}
			}
		}

		if idx >= 0 {
			_, builtInByID := builtInSetByID[merged[idx].ProfileID]
			_, builtInByName := builtInSetByName[strings.ToLower(merged[idx].Name)]
			if builtInByID || builtInByName {
				continue
			}
			merged[idx] = imported
			continue
		}

		duplicate := false
		for i := range merged {
			if imported.ProfileID != "" && merged[i].ProfileID != "" && merged[i].ProfileID == imported.ProfileID {
				duplicate = true
				break
			}
			if strings.EqualFold(merged[i].Name, imported.Name) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		merged = append(merged, imported)
	}

	return merged
}

func ensureBuiltInProfiles(cfg *Config) {
	builtIns := core.DefaultProfiles()
	for _, builtIn := range builtIns {
		if hasProfileByID(cfg.Profiles, builtIn.ProfileID) {
			continue
		}
		if builtIn.ProfileID == "" && profileNameExists(cfg.Profiles, builtIn.Name) {
			continue
		}
		if builtIn.ProfileID != "" && profileNameExists(cfg.Profiles, builtIn.Name) {
			continue
		}
		copyProfile := builtIn
		copyProfile.IsDefault = false
		cfg.Profiles = append(cfg.Profiles, copyProfile)
	}
}

func ensureSingleDefaultProfile(cfg *Config) {
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = core.DefaultProfiles()
	}
	defaultIdx := -1
	for i := range cfg.Profiles {
		if cfg.Profiles[i].IsDefault {
			if defaultIdx == -1 {
				defaultIdx = i
			} else {
				cfg.Profiles[i].IsDefault = false
			}
		}
	}
	if defaultIdx != -1 {
		return
	}
	for i := range cfg.Profiles {
		if cfg.Profiles[i].ProfileID == "tv-console-main-display" {
			cfg.Profiles[i].IsDefault = true
			return
		}
	}
	cfg.Profiles[0].IsDefault = true
}

func hasProfileByID(profiles []core.ProfileDefinition, profileID string) bool {
	if profileID == "" {
		return false
	}
	for _, profile := range profiles {
		if profile.ProfileID == profileID {
			return true
		}
	}
	return false
}

func profileNameExists(profiles []core.ProfileDefinition, name string) bool {
	if name == "" {
		return false
	}
	for _, profile := range profiles {
		if strings.EqualFold(profile.Name, name) {
			return true
		}
	}
	return false
}

func SaveAtomic(path string, cfg Config) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, b, ".cfg-*.tmp")
}

func writeAtomic(path string, content []byte, tempPattern string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), tempPattern)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}
