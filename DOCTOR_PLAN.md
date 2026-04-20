# screener doctor — Implementation Plan

A self-contained health-check and auto-repair subcommand. Runs outside the TUI as a plain terminal tool.

---

## Command interface

```
screener doctor                 check everything, auto-fix what can be fixed
screener doctor --check-only    report status without installing or modifying anything
screener doctor --clean-start   prompt to wipe config and start fresh
```

---

## User-facing output (target)

```
screener doctor v0.420  ·  linux/amd64

  Checking requirements
  ✓  adb        1.0.41          /usr/bin/adb
  ✗  scrcpy     not found
     ➜  installing via dnf...   ✓ done
  ✓  scrcpy     3.1             /usr/bin/scrcpy

  Checking config
  ✓  config     valid           ~/.config/screener/config.json
  ✓  profiles   3 profiles, 1 default
  ✓  devices    2 known devices

  All good. Run screener to start.
```

If `--check-only`:
```
  ✗  scrcpy     not found       (run without --check-only to install)
```

If something cannot be auto-installed:
```
  ✗  scrcpy     not found
     ✗  no supported package manager found
     ➜  manual install: https://github.com/Genymobile/scrcpy
     ➜  or: screener doctor --install-dir ~/.screener/bin (downloads binary)
```

---

## Package manager detection matrix

Doctor probes for package managers in priority order by checking if the binary exists in PATH.

### Windows

| Priority | Manager | Binary | Install command template |
|----------|---------|--------|--------------------------|
| 1 | winget | `winget.exe` | `winget install --id {pkg} -e --silent` |
| 2 | Chocolatey | `choco.exe` | `choco install {pkg} -y` |
| 3 | Scoop | `scoop.cmd` | `scoop install {pkg}` |
| — | fallback | — | download binary to `~/.screener/bin/` |

### macOS

| Priority | Manager | Binary | Install command template |
|----------|---------|--------|--------------------------|
| 1 | Homebrew | `brew` | `brew install {pkg}` |
| 2 | MacPorts | `port` | `sudo port install {pkg}` |
| — | fallback | — | download binary to `~/.screener/bin/` |

### Linux

| Priority | Manager | Binary | Distros |
|----------|---------|--------|---------|
| 1 | Homebrew | `brew` | any (some users) |
| 2 | dnf | `dnf` | Fedora, RHEL, CentOS Stream |
| 3 | apt | `apt` | Debian, Ubuntu, Mint, Pop!_OS |
| 4 | pacman | `pacman` | Arch, Manjaro, EndeavourOS |
| 5 | zypper | `zypper` | openSUSE |
| 6 | apk | `apk` | Alpine |
| 7 | xbps-install | `xbps-install` | Void Linux |
| 8 | emerge | `emerge` | Gentoo — warn only, too complex |
| — | fallback | — | download binary to `~/.screener/bin/` |

---

## Tool definitions

### adb

| Channel | Package name |
|---------|-------------|
| winget | `Google.PlatformTools` |
| choco | `adb` |
| scoop | `adb` |
| brew | `android-platform-tools` |
| port | `android-platform-tools` |
| dnf | `android-tools` |
| apt | `adb` |
| pacman | `android-tools` |
| zypper | `android-tools` |
| apk | `android-tools` |
| xbps | `android-tools` |

**Fallback download** — Google's official platform-tools zip (always has the latest stable adb):

| OS | URL |
|----|-----|
| Linux | `https://dl.google.com/android/repository/platform-tools-latest-linux.zip` |
| macOS | `https://dl.google.com/android/repository/platform-tools-latest-darwin.zip` |
| Windows | `https://dl.google.com/android/repository/platform-tools-latest-windows.zip` |

Extract only `adb` (Linux/macOS) or `adb.exe` + `AdbWinApi.dll` + `AdbWinUsbApi.dll` (Windows) to `~/.screener/bin/`.

### scrcpy

| Channel | Package name |
|---------|-------------|
| winget | `Genymobile.scrcpy` |
| choco | `scrcpy` |
| scoop | `scrcpy` |
| brew | `scrcpy` |
| dnf | `scrcpy` |
| apt | `scrcpy` |
| pacman | `scrcpy` |
| zypper | `scrcpy` |

**Fallback download** — GitHub releases API:
```
GET https://api.github.com/repos/Genymobile/scrcpy/releases/latest
```

Asset name patterns per platform:
- Linux x86-64: `scrcpy-linux-x86_64-*.tar.gz`
- macOS: `scrcpy-macos-*.tar.gz`
- Windows x86-64: `scrcpy-win64-*.zip`

Extract server + binary to `~/.screener/bin/`.

---

## Fallback install directory

`~/.screener/bin/` is the drop location for any binary downloaded by doctor.

After placing binaries there, doctor must ensure it is on the user's PATH:

**Linux / macOS** — append to shell rc files if not already present:
- `~/.bashrc`
- `~/.zshrc`
- `~/.config/fish/config.fish` (if exists)
- `~/.profile` (always, as fallback)

Appended line:
```sh
export PATH="$HOME/.screener/bin:$PATH"
```

**Windows** — use `setx` to add to the user PATH environment variable:
```
setx PATH "%USERPROFILE%\.screener\bin;%PATH%"
```

Doctor prints a reminder to restart the terminal (or `source ~/.zshrc`) after modifying PATH.

---

## Config validation

Doctor validates `~/.config/screener/config.json` (or `--config` override) in this order:

| Check | Auto-fix |
|-------|----------|
| File exists | Create default config |
| Valid JSON | Cannot auto-fix — report and offer `--clean-start` |
| `profiles` array non-empty | Insert built-in default profile |
| Each profile has `name` field | Generate name `"Profile N"` |
| No duplicate profile names | Append ` (2)`, ` (3)` etc. |
| `active_profile` points to existing profile | Reset to first profile |
| `known_devices` entries have `alias` or `serial` | Remove empty/null entries |

---

## `--clean-start` flow

```
screener doctor --clean-start

  ⚠  This will permanently delete your screener configuration.
     All saved devices and profiles will be removed.

  Config path: /home/justin/.config/screener/config.json

  Create a clean config file? [Y/n]:
```

- **Y** (or Enter, default yes): delete config file, print confirmation, exit 0
- **n**: print "Aborted.", exit 0
- **Ctrl+C**: exit 1 silently

After deletion, running `screener` creates a fresh config automatically on next launch.

---

## Architecture

### New files

```
internal/doctor/
  doctor.go          Run(opts DoctorOptions) — entry point, orchestrates all checks
  check.go           CheckTool(name) ToolStatus — probe binary, get version
  pkgmanager.go      DetectPM() PackageManager — probe OS for available managers
  install.go         Install(pm, tool) error — run the package manager command
  download.go        DownloadTool(tool, destDir) error — fallback HTTP download + extract
  pathutil.go        EnsureOnPath(dir) — add dir to shell rc / Windows PATH
  configcheck.go     CheckConfig(path) ConfigStatus, RepairConfig(path) — validate + fix
  cleanstart.go      CleanStart(configPath) — interactive prompt + delete
```

### Integration in `cmd/screener/main.go`

Subcommand dispatch before flag parsing:
```go
if len(os.Args) > 1 && os.Args[1] == "doctor" {
    os.Exit(doctor.Main(os.Args[2:], versionString()))
}
```

`doctor.Main` parses its own flags (`--check-only`, `--clean-start`, `--install-dir`) independently of the TUI flags.

### Key types

```go
// pkgmanager.go
type PackageManager struct {
    Name        string
    InstallArgs func(pkg string) []string
    NeedsSudo   bool
}

// check.go
type ToolStatus struct {
    Name      string
    Found     bool
    Path      string
    Version   string
    Installed bool  // true if doctor just installed it this run
    Error     string
}

// configcheck.go
type ConfigStatus struct {
    Path    string
    Exists  bool
    Valid   bool
    Repairs []string  // human-readable list of what was fixed
    Fatal   string    // non-empty = cannot auto-repair
}

// doctor.go
type DoctorOptions struct {
    CheckOnly  bool
    CleanStart bool
    InstallDir string  // default: ~/.screener/bin
    ConfigPath string
    Version    string
}
```

---

## Implementation steps

### Step 1 — Subcommand dispatch in main.go (15 min)

Add before `flag.Parse()`:
```go
if len(os.Args) > 1 && os.Args[1] == "doctor" {
    os.Exit(doctor.Main(os.Args[2:], versionString()))
}
```

Update `flag.Usage` to mention the `doctor` subcommand.

### Step 2 — Package manager detection (30 min)

`internal/doctor/pkgmanager.go`

- `DetectPM() []PackageManager` returns all available managers in priority order
- Use `exec.LookPath` to probe each binary
- Return all found (not just first) so install can try in order if one fails

### Step 3 — Tool checking (20 min)

`internal/doctor/check.go`

- `CheckTool(name string) ToolStatus`
- Uses `exec.LookPath` to find binary, then runs version command to confirm it works
- Version extraction: parse first line of stdout matching a semver-ish pattern

### Step 4 — Package manager install (30 min)

`internal/doctor/install.go`

- `InstallViaPM(pm PackageManager, toolName string, dryRun bool) error`
- Runs `pm.InstallArgs(packageName)` as a subprocess with visible output (streamed, not captured)
- Returns error if exit code != 0
- After install, calls `CheckTool` again to confirm it worked

### Step 5 — Fallback download (1 hr)

`internal/doctor/download.go`

- `DownloadADB(destDir string) error`
  - Download platform-tools zip from Google
  - Extract only the adb binary
- `DownloadScrcpy(destDir string) error`
  - Hit GitHub releases API (no auth needed, public repo)
  - Find the right asset for current OS/arch
  - Download + extract

### Step 6 — PATH management (30 min)

`internal/doctor/pathutil.go`

- `EnsureOnPath(dir string) (modified bool, err error)`
- Linux/macOS: check each rc file for the export line, append if absent
- Windows: read current user PATH via registry, prepend if absent, use `setx`
- Return `modified=true` if any file was changed so doctor can print the "restart terminal" note

### Step 7 — Config validation and repair (45 min)

`internal/doctor/configcheck.go`

- `CheckConfig(path string) ConfigStatus`
- `RepairConfig(path string) (ConfigStatus, error)` — applies all auto-fixable repairs in place using the existing `persistence` package

### Step 8 — `--clean-start` (20 min)

`internal/doctor/cleanstart.go`

- `PromptCleanStart(configPath string) error`
- Raw terminal prompt (not Bubble Tea), reads single byte: Y/y/Enter = yes, anything else = no
- Windows-safe (use `bufio.NewReader(os.Stdin).ReadString('\n')`)

### Step 9 — Orchestration (30 min)

`internal/doctor/doctor.go`

- Wire everything together in `Run(opts)`:
  1. Print header
  2. If `--clean-start`: call `PromptCleanStart`, exit
  3. Detect package managers
  4. For each required tool: check → if missing and not `--check-only` → try PM install → try download fallback
  5. Check config → if issues and not `--check-only` → repair
  6. Print summary: all good / N issues found / N issues fixed

### Step 10 — Tests (1 hr)

- `TestDetectPM` — mock PATH, verify correct manager detected
- `TestCheckToolFound` / `TestCheckToolMissing` — mock exec
- `TestCheckConfig` — valid, missing, invalid JSON, missing default profile
- `TestRepairConfig` — verify each repair type applies correctly
- `TestCleanStart` — mock stdin, verify file deletion

---

## Update `scripts/publish.sh`

No changes needed for doctor — it ships inside the same binary. No separate download required.

---

## Help text addition

`flag.Usage` in `main.go` gets a new section:

```
Subcommands:
  screener doctor               check + install adb, scrcpy; validate config
  screener doctor --check-only  report status without changing anything
  screener doctor --clean-start wipe config and start fresh
```

---

## Estimated total effort

| Step | Time |
|------|------|
| Subcommand dispatch | 15 min |
| Package manager detection | 30 min |
| Tool checking | 20 min |
| PM install | 30 min |
| Fallback download | 60 min |
| PATH management | 30 min |
| Config validation/repair | 45 min |
| `--clean-start` | 20 min |
| Orchestration + output | 30 min |
| Tests | 60 min |
| **Total** | **~6 hours** |
