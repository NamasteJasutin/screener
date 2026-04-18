package persistence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"screener/internal/core"
)

func TestDefaultConfigIncludesBuiltIns(t *testing.T) {
	cfg := DefaultConfig()
	if len(cfg.Profiles) != len(defaultProfileNames) {
		t.Fatalf("expected %d profiles, got %d", len(defaultProfileNames), len(cfg.Profiles))
	}
	for i, name := range defaultProfileNames {
		if cfg.Profiles[i].Name != name {
			t.Fatalf("unexpected profile at %d: got %q want %q", i, cfg.Profiles[i].Name, name)
		}
	}
	if cfg.ActiveProfile != "TV Console - Main Display" {
		t.Fatalf("unexpected active profile %q", cfg.ActiveProfile)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "cfg.json")
	cfg := DefaultConfig()
	if err := SaveAtomic(p, cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	loaded, err := Load(p)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.ActiveProfile == "" || len(loaded.Profiles) == 0 {
		t.Fatalf("invalid loaded config: %+v", loaded)
	}
}

func TestMigrationDefaulting(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "cfg.json")
	if err := os.WriteFile(p, []byte(`{"version":1,"profiles":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Profiles) == 0 || loaded.ActiveProfile == "" {
		t.Fatalf("defaulting not applied: %+v", loaded)
	}
	if len(loaded.Profiles) != len(defaultProfileNames) {
		t.Fatalf("expected built-ins during migration, got %d profiles", len(loaded.Profiles))
	}
}

func TestLoadMigrationAppendsMissingBuiltInsNoDuplicates(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "cfg.json")
	content := `{
		"version": 1,
		"active_profile": "Custom",
		"profiles": [
			{"name":"TV Console - Main Display","profile_id":"tv-console-main-display","is_default":true,"display_id":0,"max_size":1920,"video_bitrate_mb":8},
			{"name":"Custom","profile_id":"custom","display_id":0,"max_size":1080,"video_bitrate_mb":6},
			{"name":"Game Mode - Main Display","display_id":0,"max_size":1440,"video_bitrate_mb":24}
		]
	}`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Profiles) != 5 {
		t.Fatalf("expected 5 profiles after migration, got %d", len(loaded.Profiles))
	}
	countByName := map[string]int{}
	for _, profile := range loaded.Profiles {
		countByName[profile.Name]++
	}
	if countByName["TV Console - Main Display"] != 1 {
		t.Fatalf("expected single TV profile, got %d", countByName["TV Console - Main Display"])
	}
	if countByName["Game Mode - Main Display"] != 1 {
		t.Fatalf("expected single Game profile, got %d", countByName["Game Mode - Main Display"])
	}
	if countByName["Extra Screen - Empty"] != 1 || countByName["Samsung DeX - Virtual Display"] != 1 {
		t.Fatalf("missing migrated built-ins: %+v", countByName)
	}
	if loaded.ActiveProfile != "Custom" {
		t.Fatalf("active profile should be preserved, got %q", loaded.ActiveProfile)
	}
	defaultCount := 0
	for _, profile := range loaded.Profiles {
		if profile.IsDefault {
			defaultCount++
		}
	}
	if defaultCount != 1 {
		t.Fatalf("expected exactly one default profile, got %d", defaultCount)
	}
}

func TestCorruptedConfigHandling(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "cfg.json")
	if err := os.WriteFile(p, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(p)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if loaded.ActiveProfile == "" {
		t.Fatal("expected resilient default config")
	}
}

func TestExportImportProfilesRoundTrip(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "profiles.json")

	input := []core.ProfileDefinition{
		{
			Name:           "Portable",
			ProfileID:      "portable-1",
			DisplayID:      3,
			MaxSize:        1600,
			VideoBitRateMB: 12,
			IsDefault:      true,
			ExtraArgs:      []string{"--no-cleanup"},
			Desired:        map[string]string{"max_fps": "60"},
		},
	}

	if err := ExportProfiles(p, input); err != nil {
		t.Fatalf("export failed: %v", err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(b) == 0 || b[0] != '[' {
		t.Fatalf("expected JSON array export, got %q", string(b))
	}
	if !strings.Contains(string(b), "\n  {") {
		t.Fatalf("expected pretty JSON output, got %q", string(b))
	}

	got, err := ImportProfiles(p)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 imported profile, got %d", len(got))
	}
	if got[0].Name != input[0].Name || got[0].ProfileID != input[0].ProfileID {
		t.Fatalf("roundtrip mismatch: got %+v want %+v", got[0], input[0])
	}
}

func TestMergeProfilesByIDAndNameNoDuplicates(t *testing.T) {
	base := append(core.DefaultProfiles(), core.ProfileDefinition{
		Name:           "Custom Existing",
		ProfileID:      "custom-existing",
		DisplayID:      0,
		MaxSize:        720,
		VideoBitRateMB: 4,
	})

	incoming := []core.ProfileDefinition{
		{
			Name:           "Custom Imported",
			ProfileID:      "custom-existing",
			DisplayID:      2,
			MaxSize:        1440,
			VideoBitRateMB: 20,
			IsDefault:      true,
		},
		{
			Name:           "Case Name",
			DisplayID:      1,
			MaxSize:        900,
			VideoBitRateMB: 7,
		},
		{
			Name:           "case name",
			DisplayID:      4,
			MaxSize:        1200,
			VideoBitRateMB: 11,
			IsDefault:      true,
		},
		{
			Name:           "TV Console - Main Display",
			ProfileID:      "tv-console-main-display",
			DisplayID:      9,
			MaxSize:        999,
			VideoBitRateMB: 99,
		},
	}

	merged := MergeProfiles(base, incoming)

	builtinFound := false
	customFound := false
	caseNameCount := 0
	for _, profile := range merged {
		if profile.ProfileID == "tv-console-main-display" {
			builtinFound = true
			if profile.DisplayID != 0 || profile.MaxSize != 1920 {
				t.Fatalf("built-in should be preserved, got %+v", profile)
			}
		}
		if profile.ProfileID == "custom-existing" {
			customFound = true
			if profile.Name != "Custom Imported" || profile.DisplayID != 2 || profile.MaxSize != 1440 || !profile.IsDefault {
				t.Fatalf("custom profile should be replaced by ID, got %+v", profile)
			}
		}
		if strings.EqualFold(profile.Name, "case name") {
			caseNameCount++
		}
	}

	if !builtinFound || !customFound {
		t.Fatalf("expected built-in and custom profiles in merged output: %+v", merged)
	}
	if caseNameCount != 1 {
		t.Fatalf("expected single case-insensitive name entry, got %d", caseNameCount)
	}
}

func TestImportProfilesMalformedFile(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "profiles.json")
	if err := os.WriteFile(p, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ImportProfiles(p)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestDefaultProfilesExchangePathStability(t *testing.T) {
	p := DefaultProfilesExchangePath("/tmp/screener/config.json")
	if p != "/tmp/screener/profiles.json" {
		t.Fatalf("unexpected exchange path %q", p)
	}
}

func TestKnownDevicesSaveLoadRoundTrip(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "cfg.json")
	cfg := DefaultConfig()
	cfg.KnownDevices = []core.KnownDevice{
		{
			Alias: "192.168.1.50",
			Model: "Pixel7",
			Endpoints: []core.Endpoint{
				{Host: "192.168.1.50", Port: 5555, Transport: "tcp"},
			},
			Tags: []string{"wireless-debug"},
		},
	}
	if err := SaveAtomic(p, cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	loaded, err := Load(p)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(loaded.KnownDevices) != 1 {
		t.Fatalf("expected 1 known device, got %d", len(loaded.KnownDevices))
	}
	kd := loaded.KnownDevices[0]
	if kd.Alias != "192.168.1.50" {
		t.Fatalf("unexpected alias %q", kd.Alias)
	}
	if len(kd.Endpoints) != 1 || kd.Endpoints[0].Port != 5555 {
		t.Fatalf("unexpected endpoints: %+v", kd.Endpoints)
	}
}

var defaultProfileNames = []string{
	"TV Console - Main Display",
	"Game Mode - Main Display",
	"Extra Screen - Empty",
	"Samsung DeX - Virtual Display",
}

// ── DefaultConfigPath / DefaultLogPath ────────────────────────────────────────

func TestDefaultConfigPathContainsScreener(t *testing.T) {
	p := DefaultConfigPath()
	if p == "" {
		t.Fatal("expected non-empty DefaultConfigPath")
	}
	if !strings.Contains(p, "screener") {
		t.Fatalf("expected 'screener' in config path: %q", p)
	}
	if !strings.HasSuffix(p, "config.json") {
		t.Fatalf("expected config.json suffix: %q", p)
	}
}

func TestDefaultLogPathContainsScreener(t *testing.T) {
	p := DefaultLogPath()
	if p == "" {
		t.Fatal("expected non-empty DefaultLogPath")
	}
	if !strings.Contains(p, "screener") {
		t.Fatalf("expected 'screener' in log path: %q", p)
	}
	if !strings.HasSuffix(p, "screener.log") {
		t.Fatalf("expected screener.log suffix: %q", p)
	}
}

func TestDefaultConfigPathUsesHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p := DefaultConfigPath()
	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(p, home) {
		t.Fatalf("config path %q should be under HOME %q", p, home)
	}
}

func TestDefaultLogPathUsesHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p := DefaultLogPath()
	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(p, home) {
		t.Fatalf("log path %q should be under HOME %q", p, home)
	}
}

// ── ensureSingleDefaultProfile deeper branches ────────────────────────────────

func TestEnsureSingleDefaultProfileMultipleDefaults(t *testing.T) {
	cfg := Config{
		Version: 1,
		Profiles: []core.ProfileDefinition{
			{Name: "A", ProfileID: "a", IsDefault: true},
			{Name: "B", ProfileID: "b", IsDefault: true},
			{Name: "C", ProfileID: "c", IsDefault: true},
		},
	}
	ensureSingleDefaultProfile(&cfg)
	defaultCount := 0
	for _, p := range cfg.Profiles {
		if p.IsDefault {
			defaultCount++
		}
	}
	if defaultCount != 1 {
		t.Fatalf("expected exactly 1 default, got %d", defaultCount)
	}
	// First profile should remain default
	if !cfg.Profiles[0].IsDefault {
		t.Fatal("expected first default to be kept")
	}
}

func TestEnsureSingleDefaultProfileNoDefaultFallsBackToBuiltIn(t *testing.T) {
	cfg := Config{
		Version: 1,
		Profiles: []core.ProfileDefinition{
			{Name: "TV Console - Main Display", ProfileID: "tv-console-main-display", IsDefault: false},
			{Name: "Custom", ProfileID: "custom", IsDefault: false},
		},
	}
	ensureSingleDefaultProfile(&cfg)
	// Should fall back to tv-console-main-display by profile ID
	found := false
	for _, p := range cfg.Profiles {
		if p.ProfileID == "tv-console-main-display" && p.IsDefault {
			found = true
		}
	}
	if !found {
		t.Fatal("expected tv-console-main-display to become default when no default found")
	}
}

func TestEnsureSingleDefaultProfileEmptyFallsBackToFirst(t *testing.T) {
	cfg := Config{
		Version: 1,
		Profiles: []core.ProfileDefinition{
			{Name: "Custom A", ProfileID: "custom-a", IsDefault: false},
			{Name: "Custom B", ProfileID: "custom-b", IsDefault: false},
		},
	}
	ensureSingleDefaultProfile(&cfg)
	// No tv-console-main-display → first profile becomes default
	if !cfg.Profiles[0].IsDefault {
		t.Fatal("expected first profile to become default when no built-in match found")
	}
}

// ── writeAtomic edge cases ────────────────────────────────────────────────────

func TestWriteAtomicCreatesDirectoryIfMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "dir")
	path := filepath.Join(dir, "file.json")
	if err := writeAtomic(path, []byte(`{}`), ".tmp-*.json"); err != nil {
		t.Fatalf("writeAtomic with missing dir: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(b) != "{}" {
		t.Fatalf("unexpected content: %q", b)
	}
}

func TestWriteAtomicRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.json")
	content := []byte(`{"version":1}`)
	if err := writeAtomic(path, content, ".tmp-*.json"); err != nil {
		t.Fatalf("writeAtomic: %v", err)
	}
	// Write again (overwrite)
	content2 := []byte(`{"version":2}`)
	if err := writeAtomic(path, content2, ".tmp-*.json"); err != nil {
		t.Fatalf("writeAtomic overwrite: %v", err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != `{"version":2}` {
		t.Fatalf("expected overwritten content, got %q", b)
	}
}

// ── Load edge cases ───────────────────────────────────────────────────────────

func TestLoadNonExistentReturnsDefault(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.json"))
	// Non-existent file should return default config without error
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if len(cfg.Profiles) == 0 {
		t.Fatal("expected default profiles")
	}
}

func TestLoadWithStaleActiveProfileFallsBackToDefault(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "cfg.json")
	content := `{"version":1,"active_profile":"NonExistentProfile","profiles":[{"name":"A","display_id":0,"max_size":1920,"video_bitrate_mb":8}]}`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// active_profile should have been corrected to an existing profile
	found := false
	for _, pr := range cfg.Profiles {
		if pr.Name == cfg.ActiveProfile {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("active_profile %q not in profiles list: %v", cfg.ActiveProfile, cfg.Profiles)
	}
}

// ── ExportProfiles error path ─────────────────────────────────────────────────

func TestExportProfilesCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	profiles := []core.ProfileDefinition{
		{Name: "Test", ProfileID: "test", DisplayID: 0, MaxSize: 1920, VideoBitRateMB: 8},
	}
	if err := ExportProfiles(path, profiles); err != nil {
		t.Fatalf("ExportProfiles: %v", err)
	}
	b, _ := os.ReadFile(path)
	if len(b) == 0 {
		t.Fatal("exported file is empty")
	}
}

// ── DefaultProfilesExchangePath ───────────────────────────────────────────────

func TestDefaultProfilesExchangePathWithEmptyInput(t *testing.T) {
	p := DefaultProfilesExchangePath("")
	if p == "" {
		t.Fatal("expected non-empty exchange path for empty input")
	}
	if !strings.HasSuffix(p, "profiles.json") {
		t.Fatalf("expected profiles.json suffix: %q", p)
	}
}

func TestDefaultProfilesExchangePathWithCustomInput(t *testing.T) {
	p := DefaultProfilesExchangePath("/home/user/.config/screener/config.json")
	if p != "/home/user/.config/screener/profiles.json" {
		t.Fatalf("unexpected exchange path: %q", p)
	}
}

// ── SaveAtomic — marshaling ────────────────────────────────────────────────────

func TestSaveAtomicMarshalAndLoad(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "config.json")
	cfg := DefaultConfig()
	cfg.Theme = "Dracula"
	if err := SaveAtomic(path, cfg); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load after SaveAtomic: %v", err)
	}
	if loaded.Theme != "Dracula" {
		t.Fatalf("expected Theme=Dracula, got %q", loaded.Theme)
	}
}

// ── hasProfileByID / profileNameExists ────────────────────────────────────────

func TestHasProfileByIDFound(t *testing.T) {
	profiles := []core.ProfileDefinition{
		{Name: "A", ProfileID: "a-id"},
	}
	if !hasProfileByID(profiles, "a-id") {
		t.Fatal("expected hasProfileByID=true")
	}
}

func TestHasProfileByIDEmptyID(t *testing.T) {
	profiles := []core.ProfileDefinition{
		{Name: "A", ProfileID: ""},
	}
	if hasProfileByID(profiles, "") {
		t.Fatal("hasProfileByID(\"\") must always return false")
	}
}

func TestHasProfileByIDNotFound(t *testing.T) {
	profiles := []core.ProfileDefinition{
		{Name: "A", ProfileID: "a-id"},
	}
	if hasProfileByID(profiles, "b-id") {
		t.Fatal("expected hasProfileByID=false for missing id")
	}
}

func TestProfileNameExistsFound(t *testing.T) {
	profiles := []core.ProfileDefinition{{Name: "My Profile"}}
	if !profileNameExists(profiles, "my profile") {
		t.Fatal("expected case-insensitive match")
	}
}

func TestProfileNameExistsEmptyName(t *testing.T) {
	profiles := []core.ProfileDefinition{{Name: "A"}}
	if profileNameExists(profiles, "") {
		t.Fatal("empty name should not match")
	}
}

// ── writeAtomic — sync and close paths ───────────────────────────────────────

func TestWriteAtomicEmptyContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.json")
	if err := writeAtomic(path, []byte{}, ".tmp-*.json"); err != nil {
		t.Fatalf("writeAtomic empty content: %v", err)
	}
	b, _ := os.ReadFile(path)
	if len(b) != 0 {
		t.Fatalf("expected empty file, got %q", b)
	}
}

// ── ensureBuiltInProfiles paths ───────────────────────────────────────────────

func TestEnsureBuiltInProfilesSkipsExistingByID(t *testing.T) {
	cfg := DefaultConfig()
	original := len(cfg.Profiles)
	// Call again — should not add duplicates
	ensureBuiltInProfiles(&cfg)
	if len(cfg.Profiles) != original {
		t.Fatalf("ensureBuiltInProfiles duplicated profiles: %d → %d", original, len(cfg.Profiles))
	}
}

func TestEnsureBuiltInProfilesAddsWhenMissing(t *testing.T) {
	// Remove a built-in by ID and verify it gets re-added
	cfg := Config{Version: 1, Profiles: []core.ProfileDefinition{
		{Name: "Custom", ProfileID: "my-custom", DisplayID: 0, MaxSize: 1080, VideoBitRateMB: 8},
	}}
	ensureBuiltInProfiles(&cfg)
	// All 4 built-ins should now be present
	if len(cfg.Profiles) < 5 { // custom + 4 built-ins
		t.Fatalf("expected >= 5 profiles after ensureBuiltInProfiles, got %d", len(cfg.Profiles))
	}
}

// ── DefaultConfigPath / DefaultLogPath — HOME unset fallback ─────────────────

func TestDefaultConfigPathNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	p := DefaultConfigPath()
	// Must return a non-empty path even without HOME
	if p == "" {
		t.Fatal("expected non-empty config path without HOME")
	}
}

func TestDefaultLogPathNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	p := DefaultLogPath()
	if p == "" {
		t.Fatal("expected non-empty log path without HOME")
	}
}

// ── ExportProfiles — error on unmarshalable data ──────────────────────────────

func TestExportProfilesEmptySlice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.json")
	if err := ExportProfiles(path, nil); err != nil {
		t.Fatalf("ExportProfiles nil: %v", err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "null\n" && string(b) != "null" && string(b) != "[]\n" && string(b) != "[]" {
		t.Logf("note: ExportProfiles(nil) produced %q", b)
	}
}

// ── SaveAtomic — error on marshaling ─────────────────────────────────────────

func TestSaveAtomicVersion0Upgraded(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "cfg.json")
	cfg := DefaultConfig()
	cfg.Version = 0
	if err := SaveAtomic(p, cfg); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}
	loaded, _ := Load(p)
	// Version 0 should be upgraded to 1 on reload
	if loaded.Version != 1 {
		t.Fatalf("expected Version=1 after reload of V0 config, got %d", loaded.Version)
	}
}

// ── MergeProfiles — built-in protection ──────────────────────────────────────

func TestMergeProfilesBuiltInNotOverwritten(t *testing.T) {
	base := core.DefaultProfiles()
	incoming := []core.ProfileDefinition{
		{
			Name:           "TV Console - Main Display",
			ProfileID:      "tv-console-main-display",
			DisplayID:      9, // attempt to override built-in
			MaxSize:        999,
			VideoBitRateMB: 99,
		},
	}
	merged := MergeProfiles(base, incoming)
	for _, p := range merged {
		if p.ProfileID == "tv-console-main-display" {
			if p.DisplayID != 0 || p.MaxSize != 1920 {
				t.Fatalf("built-in should be protected from override: %+v", p)
			}
		}
	}
}

// ── ImportProfiles — duplicate name error ─────────────────────────────────────

func TestImportProfilesDuplicateName(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "profiles.json")
	content := `[{"name":"Same","display_id":0,"max_size":1080,"video_bitrate_mb":8},
                  {"name":"same","display_id":1,"max_size":720,"video_bitrate_mb":4}]`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ImportProfiles(p)
	if err == nil {
		t.Fatal("expected error for case-insensitive duplicate name")
	}
}

func TestImportProfilesEmptyName(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "profiles.json")
	content := `[{"name":"","display_id":0,"max_size":1080,"video_bitrate_mb":8}]`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ImportProfiles(p)
	if err == nil {
		t.Fatal("expected error for empty profile name")
	}
}

// ── writeAtomic — MkdirAll failure (file at parent path) ─────────────────────

func TestWriteAtomicMkdirAllFailsWhenParentIsFile(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file where the directory parent would be
	parent := filepath.Join(dir, "notadir")
	if err := os.WriteFile(parent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Try to write to a path whose parent directory is actually a file
	path := filepath.Join(parent, "sub", "config.json")
	err := writeAtomic(path, []byte("{}"), ".tmp-*.json")
	if err == nil {
		t.Fatal("expected error when parent path is a regular file (MkdirAll should fail)")
	}
}

// ── writeAtomic — CreateTemp failure (read-only directory) ────────────────────

func TestWriteAtomicCreateTempFailsWithReadOnlyDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write to read-only dirs; skipping")
	}
	dir := t.TempDir()
	// Make the directory read-only so CreateTemp fails
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755) // restore permissions for cleanup

	path := filepath.Join(dir, "config.json")
	err := writeAtomic(path, []byte("{}"), ".tmp-*.json")
	if err == nil {
		t.Fatal("expected error when directory is read-only (CreateTemp should fail)")
	}
}

// ── DefaultConfig — connection policy ─────────────────────────────────────────

func TestDefaultConfigConnectionPolicy(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.ConnectionPolicy.PreferUSB {
		t.Fatal("expected PreferUSB=true in default config")
	}
	if !cfg.ConnectionPolicy.AllowTCP {
		t.Fatal("expected AllowTCP=true in default config")
	}
}

// ── MergeProfiles — empty incoming ────────────────────────────────────────────

func TestMergeProfilesEmptyIncomingReturnsBase(t *testing.T) {
	base := core.DefaultProfiles()
	merged := MergeProfiles(base, nil)
	if len(merged) != len(base) {
		t.Fatalf("expected unchanged base with nil incoming: %d", len(merged))
	}
}

func TestMergeProfilesNewProfileAdded(t *testing.T) {
	base := []core.ProfileDefinition{{Name: "Base", ProfileID: "base", DisplayID: 0, MaxSize: 1920, VideoBitRateMB: 8}}
	incoming := []core.ProfileDefinition{{Name: "New", ProfileID: "new-id", DisplayID: 1, MaxSize: 1080, VideoBitRateMB: 6}}
	merged := MergeProfiles(base, incoming)
	if len(merged) != 2 {
		t.Fatalf("expected 2 profiles after merge, got %d", len(merged))
	}
}

// ── Load — version 0 gets upgraded ────────────────────────────────────────────

func TestLoadVersionZeroUpgradedToOne(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "cfg.json")
	// Write a V0 config
	if err := os.WriteFile(p, []byte(`{"version":0,"profiles":[{"name":"A","display_id":0,"max_size":1920,"video_bitrate_mb":8,"is_default":true}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load V0 failed: %v", err)
	}
	if cfg.Version != 1 {
		t.Fatalf("expected Version=1 after V0 load, got %d", cfg.Version)
	}
}

// ── ensureBuiltInProfiles — name-only match (no profile ID) ──────────────────

func TestEnsureBuiltInProfilesNameMatchSkipsDuplicate(t *testing.T) {
	// A built-in with ProfileID="" but matching Name → should not add duplicate
	builtins := core.DefaultProfiles()
	for i := range builtins {
		builtins[i].ProfileID = "" // clear IDs to force name matching
	}
	cfg := Config{Version: 1, Profiles: builtins}
	originalLen := len(cfg.Profiles)
	ensureBuiltInProfiles(&cfg)
	// Should not have added more profiles since names already match
	for _, p := range cfg.Profiles {
		count := 0
		for _, q := range cfg.Profiles {
			if strings.EqualFold(p.Name, q.Name) {
				count++
			}
		}
		if count > 1 {
			t.Fatalf("duplicate profile added: %q (count=%d)", p.Name, count)
		}
	}
	_ = originalLen
}
