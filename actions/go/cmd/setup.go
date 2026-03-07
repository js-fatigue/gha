package cmd

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	ac "github.com/bshore/gha/internal/actions-core"
)

func init() { Register("setup", runSetup) }

type SetupInput struct {
	GoVersion           string `json:"go_version"`
	GoVersionFile       string `json:"go_version_file"`
	CheckLatest         bool   `json:"check_latest"`
	CacheGoModules      bool   `json:"cache_go_modules"`
	CacheGoInstall      bool   `json:"cache_go_install"`
	CacheDependencyPath string `json:"cache_dependency_path"`
}

type goRelease struct {
	Version string   `json:"version"`
	Stable  bool     `json:"stable"`
	Files   []goFile `json:"files"`
}

type goFile struct {
	Filename string `json:"filename"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	Kind     string `json:"kind"`
}

func runSetup() error {
	inp := SetupInput{
		CacheGoModules:      true,
		CacheGoInstall:      true,
		CacheDependencyPath: "**/go.sum",
	}
	if err := ac.GetStructuredInput("input", &inp); err != nil {
		return fmt.Errorf("parsing input: %w", err)
	}

	// Auto-detect go.mod when no version info is specified.
	if inp.GoVersion == "" && inp.GoVersionFile == "" {
		if _, err := os.Stat("go.mod"); err == nil {
			inp.GoVersionFile = "go.mod"
			ac.Info("Auto-detected go.mod for Go version")
		}
	}

	// Resolve version from file if provided.
	if inp.GoVersionFile != "" {
		v, err := readVersionFile(inp.GoVersionFile)
		if err != nil {
			return fmt.Errorf("reading go version file %q: %w", inp.GoVersionFile, err)
		}
		inp.GoVersion = v
		ac.Info(fmt.Sprintf("Resolved version from %s: %s", inp.GoVersionFile, inp.GoVersion))
	}
	if inp.GoVersion == "" {
		inp.GoVersion = "stable"
	}

	// Restore module cache before installation.
	if inp.CacheGoModules {
		key, err := modulesCacheKey(inp.CacheDependencyPath)
		if err != nil {
			ac.Warning(fmt.Sprintf("module cache key: %v", err), nil)
		} else {
			cacheInp := ac.CacheInput{
				Action:      "restore",
				Path:        []string{"~/go/pkg/mod"},
				Key:         key,
				RestoreKeys: []string{fmt.Sprintf("go-modules-v2-%s-", os.Getenv("RUNNER_OS"))},
			}
			if err := ac.RestoreCache(cacheInp); err != nil {
				ac.Warning(fmt.Sprintf("module cache restore: %v", err), nil)
			}
		}
	}

	releases, err := fetchReleases()
	if err != nil {
		return fmt.Errorf("fetching go releases: %w", err)
	}

	version, file, err := resolveVersion(inp.GoVersion, releases)
	if err != nil {
		return fmt.Errorf("resolving version %q: %w", inp.GoVersion, err)
	}
	ac.Info(fmt.Sprintf("Resolved Go version: %s", version))

	toolCache := os.Getenv("RUNNER_TOOL_CACHE")
	if toolCache == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting home directory: %w", err)
		}
		toolCache = filepath.Join(home, "go", "sdk")
	}
	goroot := filepath.Join(toolCache, "go", version, runtime.GOOS+"-"+runtime.GOARCH)
	goBin := filepath.Join(goroot, "bin", "go")

	goInstallCacheKey := fmt.Sprintf("go-install-v1-%s-%s-%s",
		os.Getenv("RUNNER_OS"), os.Getenv("RUNNER_ARCH"), version)

	if inp.CacheGoInstall {
		restoreInp := ac.CacheInput{
			Action: "restore",
			Path:   []string{goroot},
			Key:    goInstallCacheKey,
		}
		if err := ac.RestoreCache(restoreInp); err != nil {
			ac.Warning(fmt.Sprintf("go install cache restore: %v", err), nil)
		}
	}

	downloaded := false
	if !inp.CheckLatest {
		if _, err := os.Stat(goBin); err == nil {
			ac.Info(fmt.Sprintf("Go %s already installed at %s (cache hit)", version, goroot))
		} else {
			if err := downloadAndInstall(version, file, goroot); err != nil {
				return err
			}
			downloaded = true
		}
	} else {
		if err := downloadAndInstall(version, file, goroot); err != nil {
			return err
		}
		downloaded = true
	}

	if inp.CacheGoInstall && downloaded {
		saveInp := ac.CacheInput{
			Action: "save",
			Path:   []string{goroot},
			Key:    goInstallCacheKey,
		}
		if err := ac.SaveCache(saveInp); err != nil {
			ac.Warning(fmt.Sprintf("go install cache save: %v", err), nil)
		}
	}

	if err := ac.ExportVariable("GOROOT", goroot); err != nil {
		ac.Warning(fmt.Sprintf("could not export GOROOT: %v", err), nil)
	}
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, _ := os.UserHomeDir()
		gopath = filepath.Join(home, "go")
	}
	if err := ac.ExportVariable("GOPATH", gopath); err != nil {
		ac.Warning(fmt.Sprintf("could not export GOPATH: %v", err), nil)
	}
	if err := ac.AddPath(filepath.Join(goroot, "bin")); err != nil {
		ac.Warning(fmt.Sprintf("could not add GOROOT/bin to PATH: %v", err), nil)
	}
	if err := ac.AddPath(filepath.Join(gopath, "bin")); err != nil {
		ac.Warning(fmt.Sprintf("could not add GOPATH/bin to PATH: %v", err), nil)
	}

	if err := ac.SetOutput("go_version", version); err != nil {
		ac.Warning(fmt.Sprintf("could not set go_version output: %v", err), nil)
	}

	ac.JobSummary.
		AddHeading("Go Setup", 2).
		AddTable([][]ac.SummaryTableCell{
			{
				{Data: "Version", Header: true},
				{Data: "GOROOT", Header: true},
				{Data: "GOPATH", Header: true},
				{Data: "Result", Header: true},
			},
			{
				{Data: version},
				{Data: goroot},
				{Data: gopath},
				{Data: "✅ success"},
			},
		})
	if err := ac.JobSummary.Write(nil); err != nil {
		ac.Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
	}

	// Save module cache after installation.
	if inp.CacheGoModules {
		key, err := modulesCacheKey(inp.CacheDependencyPath)
		if err != nil {
			ac.Warning(fmt.Sprintf("module cache key: %v", err), nil)
		} else {
			cacheInp := ac.CacheInput{
				Action: "save",
				Path:   []string{"~/go/pkg/mod"},
				Key:    key,
			}
			if err := ac.SaveCache(cacheInp); err != nil {
				ac.Warning(fmt.Sprintf("module cache save: %v", err), nil)
			}
		}
	}

	return nil
}

// modulesCacheKey computes a cache key based on all go.sum files matching pattern.
func modulesCacheKey(pattern string) (string, error) {
	runnerOS := os.Getenv("RUNNER_OS")
	if runnerOS == "" {
		runnerOS = "Linux"
	}

	var matches []string
	var err error

	switch {
	case !strings.ContainsAny(pattern, "*?["):
		// Explicit file path — use directly.
		matches = []string{pattern}
	case strings.Contains(pattern, "**"):
		// Recursive glob — filepath.Glob does not support **.
		base := filepath.Base(pattern)
		err = filepath.WalkDir(".", func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil // skip unreadable entries
			}
			if !d.IsDir() && filepath.Base(path) == base {
				matches = append(matches, path)
			}
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("walking workspace: %w", err)
		}
		sort.Strings(matches)
	default:
		// Standard single-level glob.
		matches, err = filepath.Glob(pattern)
		if err != nil {
			return "", fmt.Errorf("glob %q: %w", pattern, err)
		}
		sort.Strings(matches)
	}

	h := sha256.New()
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", path, err)
		}
		h.Write(data)
	}
	hash := hex.EncodeToString(h.Sum(nil))
	return fmt.Sprintf("go-modules-v2-%s-%s", runnerOS, hash), nil
}

// readVersionFile parses a go.mod or .go-version file and returns the Go version string.
func readVersionFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if filepath.Base(path) == "go.mod" {
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "go ") {
				return strings.TrimSpace(strings.TrimPrefix(line, "go ")), nil
			}
		}
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("no 'go' directive found in %s", path)
	}

	// .go-version or other: use first non-empty line, strip optional "go" prefix.
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			return strings.TrimPrefix(line, "go"), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no version found in %s", path)
}

// fetchReleases retrieves all Go releases from the go.dev download API.
func fetchReleases() ([]goRelease, error) {
	resp, err := http.Get("https://go.dev/dl/?mode=json&include=all")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status from go.dev: %s", resp.Status)
	}
	var releases []goRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	return releases, nil
}

// resolveVersion finds the concrete version string and matching archive file for wanted.
// wanted may be "stable", "oldstable", a partial version ("1.23"), or a full version ("1.23.4").
func resolveVersion(wanted string, releases []goRelease) (string, goFile, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	findFile := func(r goRelease) (goFile, bool) {
		for _, f := range r.Files {
			if f.OS == goos && f.Arch == goarch && f.Kind == "archive" {
				return f, true
			}
		}
		return goFile{}, false
	}

	switch wanted {
	case "stable":
		for _, r := range releases {
			if !r.Stable {
				continue
			}
			if f, ok := findFile(r); ok {
				return strings.TrimPrefix(r.Version, "go"), f, nil
			}
		}
		return "", goFile{}, fmt.Errorf("no stable release found for %s/%s", goos, goarch)

	case "oldstable":
		count := 0
		for _, r := range releases {
			if !r.Stable {
				continue
			}
			count++
			if count == 2 {
				if f, ok := findFile(r); ok {
					return strings.TrimPrefix(r.Version, "go"), f, nil
				}
			}
		}
		return "", goFile{}, fmt.Errorf("no oldstable release found for %s/%s", goos, goarch)

	default:
		// Partial version like "1.23" (one dot, no pre-release dash).
		if strings.Count(wanted, ".") < 2 && !strings.Contains(wanted, "-") {
			prefix := "go" + wanted + "."
			for _, r := range releases {
				if !r.Stable {
					continue
				}
				if strings.HasPrefix(r.Version, prefix) {
					if f, ok := findFile(r); ok {
						return strings.TrimPrefix(r.Version, "go"), f, nil
					}
				}
			}
			return "", goFile{}, fmt.Errorf("no stable release matching %q for %s/%s", wanted, goos, goarch)
		}

		// Exact version like "1.23.4".
		target := "go" + wanted
		for _, r := range releases {
			if r.Version == target {
				if f, ok := findFile(r); ok {
					return strings.TrimPrefix(r.Version, "go"), f, nil
				}
			}
		}
		return "", goFile{}, fmt.Errorf("release %q not found for %s/%s", wanted, goos, goarch)
	}
}

// downloadAndInstall fetches the Go archive, verifies its SHA256, and extracts it to goroot.
func downloadAndInstall(version string, file goFile, goroot string) error {
	url := fmt.Sprintf("https://go.dev/dl/%s", file.Filename)
	ac.Info(fmt.Sprintf("Downloading Go %s from %s", version, url))

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("downloading Go: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	tmp, err := os.CreateTemp("", "go-*.tar.gz")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("writing download: %w", err)
	}
	tmp.Close()

	if file.SHA256 != "" {
		got := hex.EncodeToString(h.Sum(nil))
		if got != file.SHA256 {
			return fmt.Errorf("SHA256 mismatch: got %s, want %s", got, file.SHA256)
		}
		ac.Info("SHA256 verified")
	}

	if err := os.MkdirAll(goroot, 0755); err != nil {
		return fmt.Errorf("creating install dir: %w", err)
	}

	ac.Info(fmt.Sprintf("Extracting to %s", goroot))
	out, err := exec.Command("tar", "-xzf", tmpName, "--strip-components=1", "-C", goroot).CombinedOutput()
	if err != nil {
		return fmt.Errorf("extracting archive: %w\n%s", err, out)
	}

	return nil
}
