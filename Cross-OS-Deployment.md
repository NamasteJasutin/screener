# Cross-OS Deployment Plan

Distribution channels ranked by effort, from easiest to hardest.

---

## 1. `go install` — 30 minutes, one code change

Anyone with Go installed can run:
```bash
go install github.com/NamasteJasutin/screener/cmd/screener@latest
```

### What needs to change

The module name in `go.mod` is currently `screener` (local name). It must match the GitHub URL for `go install` to resolve it.

**`go.mod` line 1 — change:**
```
module screener
```
**to:**
```
module github.com/NamasteJasutin/screener
```

Then update every internal import path in the codebase from `screener/internal/...` to `github.com/NamasteJasutin/screener/internal/...`.

Run after the change:
```bash
go build ./...
go test ./...
git add go.mod go.sum $(grep -rl 'screener/internal' .)
git commit -m "fix: rename module to github.com/NamasteJasutin/screener for go install"
git push origin main
```

**That's it.** Go's module proxy picks it up automatically from the tag. No registration needed.

---

## 2. Homebrew (macOS + Linux) — 1–2 hours, zero gatekeeping

Two tiers:

### Tier A: Your own tap (do this first — available immediately)

Users run:
```bash
brew tap NamasteJasutin/screener
brew install screener
```

Steps:
1. Create a new GitHub repo named exactly `homebrew-screener`  
   → `https://github.com/NamasteJasutin/homebrew-screener`
2. Add a formula file `Formula/screener.rb`:

```ruby
class Screener < Formula
  desc "Terminal UI for ADB + scrcpy Android device management"
  homepage "https://github.com/NamasteJasutin/screener"
  version "0.420"

  on_macos do
    on_arm do
      url "https://github.com/NamasteJasutin/screener/releases/download/v#{version}/screener-darwin-arm64"
      sha256 "REPLACE_WITH_SHA256_OF_screener-darwin-arm64"
    end
    on_intel do
      url "https://github.com/NamasteJasutin/screener/releases/download/v#{version}/screener-darwin-amd64"
      sha256 "REPLACE_WITH_SHA256_OF_screener-darwin-amd64"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/NamasteJasutin/screener/releases/download/v#{version}/screener-linux-arm64"
      sha256 "REPLACE_WITH_SHA256_OF_screener-linux-arm64"
    end
    on_intel do
      url "https://github.com/NamasteJasutin/screener/releases/download/v#{version}/screener-linux-amd64"
      sha256 "REPLACE_WITH_SHA256_OF_screener-linux-amd64"
    end
  end

  def install
    bin.install Dir["screener-*"].first => "screener"
  end

  test do
    assert_match "0.420", shell_output("#{bin}/screener --version")
  end
end
```

3. Get the SHA256 values by running on the release binaries:
```bash
sha256sum dist/screener-darwin-arm64 dist/screener-darwin-amd64 \
           dist/screener-linux-amd64 dist/screener-linux-arm64
```

4. Update `scripts/publish.sh` to regenerate the formula after each build (see §Automation below).

### Tier B: homebrew-core (weeks, optional later)

Homebrew Core has stricter criteria (notable project, stable, good test coverage). Apply once the project has traction. Not worth the effort right now.

---

## 3. winget (Windows) — 2–4 hours, automated review

Users run:
```powershell
winget install NamasteJasutin.screener
```

Steps:
1. Fork `https://github.com/microsoft/winget-pkgs`
2. Create the manifest directory:  
   `manifests/n/NamasteJasutin/screener/0.420/`
3. Three YAML files are required:

**`NamasteJasutin.screener.yaml`** (version manifest):
```yaml
PackageIdentifier: NamasteJasutin.screener
PackageVersion: 0.420
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.6.0
```

**`NamasteJasutin.screener.locale.en-US.yaml`** (metadata):
```yaml
PackageIdentifier: NamasteJasutin.screener
PackageVersion: 0.420
PackageLocale: en-US
Publisher: Justin
PackageName: screener
License: MIT
ShortDescription: Terminal UI for ADB + scrcpy Android device management
PackageUrl: https://github.com/NamasteJasutin/screener
ManifestType: defaultLocale
ManifestVersion: 1.6.0
```

**`NamasteJasutin.screener.installer.yaml`** (installer):
```yaml
PackageIdentifier: NamasteJasutin.screener
PackageVersion: 0.420
InstallerLocale: en-US
InstallerType: portable
Commands:
  - screener
Installers:
  - Architecture: x64
    InstallerUrl: https://github.com/NamasteJasutin/screener/releases/download/v0.420/screener-windows-amd64.exe
    InstallerSha256: REPLACE_WITH_SHA256_OF_screener-windows-amd64.exe
ManifestType: installer
ManifestVersion: 1.6.0
```

4. Open a pull request against `microsoft/winget-pkgs`  
   → Microsoft's bot validates the manifest and the binary automatically  
   → Human review follows — typically 1–3 days for new packages

For each new version, repeat with the new version number and updated SHA256. There are community tools (`wingetcreate`) that automate this.

---

## 4. Chocolatey (Windows) — 2–4 hours, moderated review

Users run:
```powershell
choco install screener
```

Steps:
1. Create a free account at `https://community.chocolatey.org`
2. Install the Chocolatey CLI on Windows: `choco install chocolatey`
3. Create a package directory with two files:

**`screener.nuspec`**:
```xml
<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://schemas.microsoft.com/packaging/2015/06/nuspec.xsd">
  <metadata>
    <id>screener</id>
    <version>0.420</version>
    <title>screener</title>
    <authors>Justin</authors>
    <projectUrl>https://github.com/NamasteJasutin/screener</projectUrl>
    <licenseUrl>https://github.com/NamasteJasutin/screener/blob/main/LICENSE</licenseUrl>
    <requireLicenseAcceptance>false</requireLicenseAcceptance>
    <description>Terminal UI for ADB + scrcpy Android device management.</description>
    <tags>adb scrcpy android terminal tui</tags>
  </metadata>
</package>
```

**`tools/chocolateyInstall.ps1`**:
```powershell
$ErrorActionPreference = 'Stop'
$toolsDir = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"

$packageArgs = @{
  packageName   = $env:ChocolateyPackageName
  fileType      = 'exe'
  url64bit      = 'https://github.com/NamasteJasutin/screener/releases/download/v0.420/screener-windows-amd64.exe'
  checksum64    = 'REPLACE_WITH_SHA256_OF_screener-windows-amd64.exe'
  checksumType64= 'sha256'
  destination   = $toolsDir
}

Get-ChocolateyWebFile @packageArgs
Install-BinFile -Name 'screener' -Path "$toolsDir\screener-windows-amd64.exe"
```

4. Pack and push:
```powershell
choco pack
choco push screener.0.420.nupkg --source https://push.chocolatey.org
```

5. Moderators review the package — typically 1–5 days. Once approved it's auto-approved for future versions (trusted submitter status after a few releases).

---

## 5. dnf / RPM (Fedora/RHEL) — 4–8 hours, community hosted

Users run:
```bash
sudo dnf copr enable namastejasutin/screener
sudo dnf install screener
```

COPR (Cool Other Package Repo) is Fedora's community hosting — no gatekeeping, builds happen in Fedora's cloud for free.

Steps:
1. Create account at `https://copr.fedorainfracloud.org`
2. Create a new COPR project named `screener`
3. Write an RPM spec file `screener.spec`:

```spec
Name:           screener
Version:        0.420
Release:        1%{?dist}
Summary:        Terminal UI for ADB + scrcpy Android device management
License:        MIT
URL:            https://github.com/NamasteJasutin/screener
Source0:        https://github.com/NamasteJasutin/screener/releases/download/v%{version}/screener-linux-%{_arch_remap}

%global _arch_remap %{lua: print(rpm.expand("%{_arch}") == "x86_64" and "amd64" or "arm64")}

%description
screener is a keyboard-driven terminal UI for managing Android devices
via ADB and scrcpy. Supports USB, Wi-Fi, and Tailscale connections.

%prep

%build

%install
mkdir -p %{buildroot}%{_bindir}
install -m 0755 %{SOURCE0} %{buildroot}%{_bindir}/screener

%files
%{_bindir}/screener

%changelog
* Fri Apr 18 2026 Justin <ami@jasutin.site> - 0.420-1
- Initial package
```

4. Submit a build via COPR's web UI or CLI (`copr-cli`), pointing at the spec file and source binary.

For future releases: update the `Version:` line and submit a new build.

---

## Recommended order

| Step | Channel | Time | Why first |
|------|---------|------|-----------|
| 1 | `go install` | 30 min | Fixes the module name (needed anyway), instant availability |
| 2 | Homebrew tap | 1–2 hrs | macOS users, no review wait |
| 3 | winget | 2–4 hrs | Large Windows user base, automated review |
| 4 | Chocolatey | 2–4 hrs | Power users already using choco |
| 5 | dnf / COPR | 4–8 hrs | Linux users who won't use `go install` |

---

## Automation: updating publish.sh

Once channels are set up, add a section to `scripts/publish.sh` that regenerates version numbers and SHA256 checksums in the formula/manifests automatically after each build, then commits and pushes those files too. That way `./scripts/publish.sh v0.421` keeps everything in sync without any manual edits.
