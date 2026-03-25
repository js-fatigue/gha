package cmd

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	ac "github.com/js-fatigue/gha/internal/actions-core"
)

func init() { Register("setup", runSetup) }

type SetupInput struct {
	TerraformVersion string `json:"terraform_version" hcl:"terraform_version"`
	TflintVersion    string `json:"tflint_version"    hcl:"tflint_version"`
	CheckovVersion   string `json:"checkov_version"   hcl:"checkov_version"`
}

func runSetup() error {
	inp := SetupInput{
		TerraformVersion: "latest",
		TflintVersion:    "latest",
		CheckovVersion:   "latest",
	}
	if err := ac.GetStructuredInput("input", &inp); err != nil {
		return fmt.Errorf("parsing input: %w", err)
	}

	toolCache := os.Getenv("RUNNER_TOOL_CACHE")
	if toolCache == "" {
		toolCache = "/opt/hostedtoolcache"
	}
	runnerOS := os.Getenv("RUNNER_OS")
	if runnerOS == "" {
		runnerOS = "Linux"
	}
	runnerArch := os.Getenv("RUNNER_ARCH")
	if runnerArch == "" {
		runnerArch = "X64"
	}

	tfVersion, err := setupTerraform(inp.TerraformVersion, toolCache, runnerOS, runnerArch)
	if err != nil {
		return fmt.Errorf("setting up terraform: %w", err)
	}

	tflintVersion, err := setupTflint(inp.TflintVersion, toolCache, runnerOS, runnerArch)
	if err != nil {
		return fmt.Errorf("setting up tflint: %w", err)
	}

	checkovVersion, err := setupCheckov(inp.CheckovVersion, toolCache, runnerOS)
	if err != nil {
		return fmt.Errorf("setting up checkov: %w", err)
	}

	if err := ac.SetOutput("terraform_version", tfVersion); err != nil {
		ac.Warning(fmt.Sprintf("could not set terraform_version output: %v", err), nil)
	}
	if err := ac.SetOutput("tflint_version", tflintVersion); err != nil {
		ac.Warning(fmt.Sprintf("could not set tflint_version output: %v", err), nil)
	}
	if err := ac.SetOutput("checkov_version", checkovVersion); err != nil {
		ac.Warning(fmt.Sprintf("could not set checkov_version output: %v", err), nil)
	}

	ac.JobSummary.AddHeading("Terraform Setup", 2).AddTable([][]ac.SummaryTableCell{
		{
			{Data: "Tool", Header: true},
			{Data: "Version", Header: true},
			{Data: "Result", Header: true},
		},
		{{Data: "terraform"}, {Data: tfVersion}, {Data: "✅ success"}},
		{{Data: "tflint"}, {Data: tflintVersion}, {Data: "✅ success"}},
		{{Data: "checkov"}, {Data: checkovVersion}, {Data: "✅ success"}},
	})
	if err := ac.JobSummary.Write(nil); err != nil {
		ac.Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
	}

	return nil
}

// --- Terraform ---

func setupTerraform(version, toolCache, runnerOS, runnerArch string) (string, error) {
	if version == "latest" {
		v, err := resolveTerraformLatest()
		if err != nil {
			return "", fmt.Errorf("resolving latest terraform version: %w", err)
		}
		version = v
		ac.Info(fmt.Sprintf("Resolved latest Terraform version: %s", version))
	}

	goos := runtime.GOOS
	goarch := runtime.GOARCH
	installDir := filepath.Join(toolCache, "terraform", version, goos+"-"+goarch)

	cacheKey := fmt.Sprintf("terraform-install-v1-%s-%s-%s", runnerOS, runnerArch, version)
	restoreInp := ac.CacheInput{
		Action: "restore",
		Path:   []string{installDir},
		Key:    cacheKey,
	}
	if err := ac.RestoreCache(restoreInp); err != nil {
		ac.Warning(fmt.Sprintf("terraform install cache restore: %v", err), nil)
	}

	tfBin := filepath.Join(installDir, "terraform")
	downloaded := false
	if _, err := os.Stat(tfBin); err != nil {
		if err := downloadTerraform(version, goos, goarch, installDir); err != nil {
			return "", err
		}
		downloaded = true
	} else {
		ac.Info(fmt.Sprintf("Terraform %s already installed at %s (cache hit)", version, installDir))
	}

	if downloaded {
		saveInp := ac.CacheInput{
			Action: "save",
			Path:   []string{installDir},
			Key:    cacheKey,
		}
		if err := ac.SaveCache(saveInp); err != nil {
			ac.Warning(fmt.Sprintf("terraform install cache save: %v", err), nil)
		}
	}

	if err := ac.AddPath(installDir); err != nil {
		ac.Warning(fmt.Sprintf("could not add terraform to PATH: %v", err), nil)
	}

	return version, nil
}

func resolveTerraformLatest() (string, error) {
	resp, err := http.Get("https://api.releases.hashicorp.com/v1/releases/terraform?limit=1&license_class=oss")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status from HashiCorp API: %s", resp.Status)
	}
	var releases []struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", fmt.Errorf("decoding HashiCorp release response: %w", err)
	}
	if len(releases) == 0 {
		return "", fmt.Errorf("no releases returned from HashiCorp API")
	}
	return releases[0].Version, nil
}

func downloadTerraform(version, goos, goarch, installDir string) error {
	zipName := fmt.Sprintf("terraform_%s_%s_%s.zip", version, goos, goarch)
	zipURL := fmt.Sprintf("https://releases.hashicorp.com/terraform/%s/%s", version, zipName)
	checksumURL := fmt.Sprintf("https://releases.hashicorp.com/terraform/%s/terraform_%s_SHA256SUMS", version, version)

	ac.Info(fmt.Sprintf("Downloading Terraform %s from %s", version, zipURL))

	// Download checksum file first.
	csResp, err := http.Get(checksumURL)
	if err != nil {
		return fmt.Errorf("downloading terraform checksums: %w", err)
	}
	defer csResp.Body.Close()
	csBytes, err := io.ReadAll(csResp.Body)
	if err != nil {
		return fmt.Errorf("reading terraform checksums: %w", err)
	}
	expectedHash := parseChecksumFile(string(csBytes), zipName)
	if expectedHash == "" {
		return fmt.Errorf("terraform: %s not found in SHA256SUMS", zipName)
	}

	// Download zip.
	zipResp, err := http.Get(zipURL)
	if err != nil {
		return fmt.Errorf("downloading terraform zip: %w", err)
	}
	defer zipResp.Body.Close()
	if zipResp.StatusCode != http.StatusOK {
		return fmt.Errorf("terraform download returned %s", zipResp.Status)
	}

	tmp, err := os.CreateTemp("", "terraform-*.zip")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), zipResp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("writing terraform zip: %w", err)
	}
	tmp.Close()

	got := hex.EncodeToString(h.Sum(nil))
	if got != expectedHash {
		return fmt.Errorf("terraform SHA256 mismatch: got %s, want %s", got, expectedHash)
	}
	ac.Info("Terraform SHA256 verified")

	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("creating terraform install dir: %w", err)
	}

	if err := extractZipBinary(tmpName, "terraform", installDir); err != nil {
		return fmt.Errorf("extracting terraform: %w", err)
	}

	return nil
}

// --- tflint ---

func setupTflint(version, toolCache, runnerOS, runnerArch string) (string, error) {
	if version == "latest" {
		v, err := resolveGitHubLatestRelease("terraform-linters/tflint")
		if err != nil {
			return "", fmt.Errorf("resolving latest tflint version: %w", err)
		}
		version = v
		ac.Info(fmt.Sprintf("Resolved latest tflint version: %s", version))
	}

	goos := runtime.GOOS
	goarch := runtime.GOARCH
	installDir := filepath.Join(toolCache, "tflint", version, goos+"-"+goarch)

	cacheKey := fmt.Sprintf("tflint-install-v1-%s-%s-%s", runnerOS, runnerArch, version)
	restoreInp := ac.CacheInput{
		Action: "restore",
		Path:   []string{installDir},
		Key:    cacheKey,
	}
	if err := ac.RestoreCache(restoreInp); err != nil {
		ac.Warning(fmt.Sprintf("tflint install cache restore: %v", err), nil)
	}

	tflintBin := filepath.Join(installDir, "tflint")
	downloaded := false
	if _, err := os.Stat(tflintBin); err != nil {
		if err := downloadTflint(version, goos, goarch, installDir); err != nil {
			return "", err
		}
		downloaded = true
	} else {
		ac.Info(fmt.Sprintf("tflint %s already installed at %s (cache hit)", version, installDir))
	}

	if downloaded {
		saveInp := ac.CacheInput{
			Action: "save",
			Path:   []string{installDir},
			Key:    cacheKey,
		}
		if err := ac.SaveCache(saveInp); err != nil {
			ac.Warning(fmt.Sprintf("tflint install cache save: %v", err), nil)
		}
	}

	if err := ac.AddPath(installDir); err != nil {
		ac.Warning(fmt.Sprintf("could not add tflint to PATH: %v", err), nil)
	}

	return version, nil
}

func downloadTflint(version, goos, goarch, installDir string) error {
	zipName := fmt.Sprintf("tflint_%s_%s.zip", goos, goarch)
	zipURL := fmt.Sprintf("https://github.com/terraform-linters/tflint/releases/download/v%s/%s", version, zipName)
	checksumURL := fmt.Sprintf("https://github.com/terraform-linters/tflint/releases/download/v%s/checksums.txt", version)

	ac.Info(fmt.Sprintf("Downloading tflint %s from %s", version, zipURL))

	csResp, err := http.Get(checksumURL)
	if err != nil {
		return fmt.Errorf("downloading tflint checksums: %w", err)
	}
	defer csResp.Body.Close()
	csBytes, err := io.ReadAll(csResp.Body)
	if err != nil {
		return fmt.Errorf("reading tflint checksums: %w", err)
	}
	expectedHash := parseChecksumFile(string(csBytes), zipName)
	if expectedHash == "" {
		return fmt.Errorf("tflint: %s not found in checksums.txt", zipName)
	}

	zipResp, err := http.Get(zipURL)
	if err != nil {
		return fmt.Errorf("downloading tflint zip: %w", err)
	}
	defer zipResp.Body.Close()
	if zipResp.StatusCode != http.StatusOK {
		return fmt.Errorf("tflint download returned %s", zipResp.Status)
	}

	tmp, err := os.CreateTemp("", "tflint-*.zip")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), zipResp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("writing tflint zip: %w", err)
	}
	tmp.Close()

	got := hex.EncodeToString(h.Sum(nil))
	if got != expectedHash {
		return fmt.Errorf("tflint SHA256 mismatch: got %s, want %s", got, expectedHash)
	}
	ac.Info("tflint SHA256 verified")

	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("creating tflint install dir: %w", err)
	}

	if err := extractZipBinary(tmpName, "tflint", installDir); err != nil {
		return fmt.Errorf("extracting tflint: %w", err)
	}

	return nil
}

// --- Checkov ---

func setupCheckov(version, toolCache, runnerOS string) (string, error) {
	if version == "latest" {
		v, err := resolveCheckovLatest()
		if err != nil {
			return "", fmt.Errorf("resolving latest checkov version: %w", err)
		}
		version = v
		ac.Info(fmt.Sprintf("Resolved latest Checkov version: %s", version))
	}

	venvDir := filepath.Join(toolCache, "checkov", version)

	cacheKey := fmt.Sprintf("checkov-install-v1-%s-%s", runnerOS, version)
	restoreInp := ac.CacheInput{
		Action: "restore",
		Path:   []string{venvDir},
		Key:    cacheKey,
	}
	if err := ac.RestoreCache(restoreInp); err != nil {
		ac.Warning(fmt.Sprintf("checkov install cache restore: %v", err), nil)
	}

	checkovBin := filepath.Join(venvDir, "bin", "checkov")
	downloaded := false
	if _, err := os.Stat(checkovBin); err != nil {
		if err := installCheckov(version, venvDir); err != nil {
			return "", err
		}
		downloaded = true
	} else {
		ac.Info(fmt.Sprintf("Checkov %s already installed at %s (cache hit)", version, venvDir))
	}

	if downloaded {
		saveInp := ac.CacheInput{
			Action: "save",
			Path:   []string{venvDir},
			Key:    cacheKey,
		}
		if err := ac.SaveCache(saveInp); err != nil {
			ac.Warning(fmt.Sprintf("checkov install cache save: %v", err), nil)
		}
	}

	binDir := filepath.Join(venvDir, "bin")
	if err := ac.AddPath(binDir); err != nil {
		ac.Warning(fmt.Sprintf("could not add checkov to PATH: %v", err), nil)
	}

	return version, nil
}

func resolveCheckovLatest() (string, error) {
	resp, err := http.Get("https://pypi.org/pypi/checkov/json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status from PyPI: %s", resp.Status)
	}
	var pypi struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pypi); err != nil {
		return "", fmt.Errorf("decoding PyPI response: %w", err)
	}
	if pypi.Info.Version == "" {
		return "", fmt.Errorf("no version found in PyPI response")
	}
	return pypi.Info.Version, nil
}

func installCheckov(version, venvDir string) error {
	ac.Info(fmt.Sprintf("Installing Checkov %s into venv at %s", version, venvDir))

	if err := os.MkdirAll(filepath.Dir(venvDir), 0755); err != nil {
		return fmt.Errorf("creating checkov parent dir: %w", err)
	}

	res, err := ac.Exec("python3", "-m", "venv", venvDir)
	if err != nil {
		return fmt.Errorf("creating checkov venv: %w\n%s", err, res.Stderr)
	}

	pip := filepath.Join(venvDir, "bin", "pip")
	spec := "checkov==" + version
	res, err = ac.Exec(pip, "install", "--quiet", spec)
	if err != nil {
		return fmt.Errorf("installing checkov: %w\n%s", err, res.Stderr)
	}

	ac.Info(fmt.Sprintf("Checkov %s installed", version))
	return nil
}

// --- shared helpers ---

func resolveGitHubLatestRelease(repo string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %s for %s", resp.Status, repo)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("decoding GitHub release response: %w", err)
	}
	// Strip leading 'v' if present.
	return strings.TrimPrefix(rel.TagName, "v"), nil
}

// parseChecksumFile extracts the hex hash for filename from a SHA256SUMS-style file.
// Lines are expected to be: "<hash>  <filename>" or "<hash> <filename>".
func parseChecksumFile(content, filename string) string {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == filename {
			return fields[0]
		}
	}
	return ""
}

// extractZipBinary extracts a single named binary from a zip file into destDir.
func extractZipBinary(zipPath, binaryName, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name != binaryName && filepath.Base(f.Name) != binaryName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("opening zip entry %s: %w", f.Name, err)
		}
		dest := filepath.Join(destDir, binaryName)
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			rc.Close()
			return fmt.Errorf("creating binary file: %w", err)
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return fmt.Errorf("writing binary: %w", err)
		}
		out.Close()
		rc.Close()
		ac.Info(fmt.Sprintf("Extracted %s to %s", binaryName, dest))
		return nil
	}
	return fmt.Errorf("%s not found in zip", binaryName)
}
