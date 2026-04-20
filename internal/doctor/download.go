package doctor

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DownloadADB downloads Google platform-tools and extracts adb to destDir.
func DownloadADB(destDir string) error {
	url := platformToolsURL()
	zipPath, err := downloadTemp(url, "platform-tools-*.zip")
	if err != nil {
		return fmt.Errorf("download platform-tools: %w", err)
	}
	defer os.Remove(zipPath)
	return extractADBFromZip(zipPath, destDir)
}

// DownloadScrcpy fetches the latest scrcpy release from GitHub and extracts
// the appropriate binary to destDir.
func DownloadScrcpy(destDir string) error {
	url, err := latestScrcpyURL()
	if err != nil {
		return fmt.Errorf("resolve scrcpy release: %w", err)
	}

	isTar := strings.HasSuffix(url, ".tar.gz") || strings.HasSuffix(url, ".tgz")

	ext := "*.zip"
	if isTar {
		ext = "*.tar.gz"
	}
	archivePath, err := downloadTemp(url, "scrcpy-"+ext)
	if err != nil {
		return fmt.Errorf("download scrcpy: %w", err)
	}
	defer os.Remove(archivePath)

	if isTar {
		return extractScrcpyFromTar(archivePath, destDir)
	}
	return extractScrcpyFromZip(archivePath, destDir)
}

// ── GitHub release lookup ──────────────────────────────────────────────────────

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

func latestScrcpyURL() (string, error) {
	resp, err := http.Get("https://api.github.com/repos/Genymobile/scrcpy/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}

	want := scrcpyAssetPattern()
	for _, a := range rel.Assets {
		if matchesPattern(a.Name, want) {
			return a.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("no matching asset for %s/%s in release %s (pattern: %v)",
		runtime.GOOS, runtime.GOARCH, rel.TagName, want)
}

// scrcpyAssetPattern returns substrings that must all appear in the asset name.
func scrcpyAssetPattern() []string {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "arm64" {
			return []string{"win", "arm64"}
		}
		return []string{"win64"}
	case "darwin":
		return []string{"macos"}
	default:
		// Linux
		switch runtime.GOARCH {
		case "arm64":
			return []string{"linux", "aarch64"}
		default:
			return []string{"linux", "x86_64"}
		}
	}
}

func matchesPattern(name string, parts []string) bool {
	low := strings.ToLower(name)
	for _, p := range parts {
		if !strings.Contains(low, strings.ToLower(p)) {
			return false
		}
	}
	return true
}

// ── Download helper ───────────────────────────────────────────────────────────

func downloadTemp(url, pattern string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}

	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// ── Extraction helpers ────────────────────────────────────────────────────────

func extractADBFromZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	adbName := "adb"
	if runtime.GOOS == "windows" {
		adbName = "adb.exe"
	}

	for _, f := range r.File {
		base := filepath.Base(f.Name)
		// Extract adb binary and Windows DLLs needed by adb.exe
		if base != adbName && base != "AdbWinApi.dll" && base != "AdbWinUsbApi.dll" {
			continue
		}
		if err := extractZipEntry(f, destDir); err != nil {
			return err
		}
	}
	return nil
}

func extractZipEntry(f *zip.File, destDir string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	dest := filepath.Join(destDir, filepath.Base(f.Name))
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

func extractScrcpyFromZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(f.Name)
		if !isScrcpyFile(base) {
			continue
		}
		if err := extractZipEntry(f, destDir); err != nil {
			return err
		}
	}
	return nil
}

func extractScrcpyFromTar(tarPath, destDir string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		base := filepath.Base(hdr.Name)
		if !isScrcpyFile(base) {
			continue
		}
		dest := filepath.Join(destDir, base)
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		out.Close()
	}
	return nil
}

// isScrcpyFile returns true for files that must be co-located with scrcpy.
func isScrcpyFile(base string) bool {
	name := strings.ToLower(base)
	return name == "scrcpy" || name == "scrcpy.exe" ||
		strings.HasPrefix(name, "scrcpy-server") ||
		strings.HasSuffix(name, ".dll")
}
