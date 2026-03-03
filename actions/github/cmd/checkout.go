package cmd

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	ac "github.com/bshore/gha/internal/actions-core"
)

func init() { Register("checkout", runCheckout) }

type CheckoutInput struct {
	Repository           string `json:"repository"`
	Ref                  string `json:"ref"`
	Path                 string `json:"path"`
	FetchDepth           int    `json:"fetch_depth"`
	FetchTags            bool   `json:"fetch_tags"`
	Clean                bool   `json:"clean"`
	Submodules           string `json:"submodules"`
	LFS                  bool   `json:"lfs"`
	PersistCredentials   bool   `json:"persist_credentials"`
	SetSafeDirectory     bool   `json:"set_safe_directory"`
	SparseCheckout       string `json:"sparse_checkout"`
	SparseCheckoutConeMode bool `json:"sparse_checkout_cone_mode"`
}

func runCheckout() error {
	inp := CheckoutInput{
		FetchDepth:             1,
		Clean:                  true,
		PersistCredentials:     true,
		SetSafeDirectory:       true,
		SparseCheckoutConeMode: true,
		Submodules:             "false",
	}
	if err := ac.GetJSONInput("checkout_input", &inp); err != nil {
		return fmt.Errorf("parsing checkout_input: %w", err)
	}

	repository := inp.Repository
	ref := inp.Ref
	checkoutPath := inp.Path
	fetchDepthRaw := fmt.Sprintf("%d", inp.FetchDepth)
	sparseCheckout := inp.SparseCheckout
	fetchTags := inp.FetchTags
	clean := inp.Clean
	lfs := inp.LFS
	persistCredentials := inp.PersistCredentials
	setSafeDirectory := inp.SetSafeDirectory
	sparseConeMode := inp.SparseCheckoutConeMode
	submodules := inp.Submodules

	// --- Resolve env vars ---
	token := os.Getenv("GITHUB_TOKEN")
	serverURL := os.Getenv("GITHUB_SERVER_URL")
	if serverURL == "" {
		serverURL = "https://github.com"
	}
	workspace := os.Getenv("GITHUB_WORKSPACE")

	if repository == "" {
		repository = os.Getenv("GITHUB_REPOSITORY")
	}
	if ref == "" {
		ref = os.Getenv("GITHUB_REF")
	}

	// --- Resolve target directory ---
	targetDir := workspace
	if checkoutPath != "" {
		if filepath.IsAbs(checkoutPath) {
			targetDir = checkoutPath
		} else {
			targetDir = filepath.Join(workspace, checkoutPath)
		}
	}

	ac.Info(fmt.Sprintf("Checking out %s@%s into %s", repository, ref, targetDir))

	// --- Set safe directory ---
	if setSafeDirectory {
		if out, err := exec.Command("git", "config", "--global", "--add", "safe.directory", targetDir).CombinedOutput(); err != nil {
			return fmt.Errorf("git config safe.directory: %w\n%s", err, out)
		}
	}

	// --- Configure credentials ---
	var cleanupCredentials func()
	if token != "" && persistCredentials {
		encoded := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
		headerVal := "AUTHORIZATION: basic " + encoded
		configKey := fmt.Sprintf("http.%s/.extraheader", strings.TrimRight(serverURL, "/"))
		if out, err := exec.Command("git", "config", "--global", configKey, headerVal).CombinedOutput(); err != nil {
			return fmt.Errorf("configuring git credentials: %w\n%s", err, out)
		}
		if !persistCredentials {
			cleanupCredentials = func() {
				exec.Command("git", "config", "--global", "--unset-all", configKey).Run() //nolint
			}
		}
	} else if token != "" && !persistCredentials {
		encoded := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
		headerVal := "AUTHORIZATION: basic " + encoded
		configKey := fmt.Sprintf("http.%s/.extraheader", strings.TrimRight(serverURL, "/"))
		if out, err := exec.Command("git", "config", "--global", configKey, headerVal).CombinedOutput(); err != nil {
			return fmt.Errorf("configuring git credentials: %w\n%s", err, out)
		}
		cleanupCredentials = func() {
			exec.Command("git", "config", "--global", "--unset-all", configKey).Run() //nolint
		}
	}
	if cleanupCredentials != nil {
		defer cleanupCredentials()
	}

	// --- Build clone URL ---
	serverHost := strings.TrimPrefix(serverURL, "https://")
	serverHost = strings.TrimPrefix(serverHost, "http://")
	serverHost = strings.TrimRight(serverHost, "/")
	cloneURL := fmt.Sprintf("https://%s/%s.git", serverHost, repository)

	// --- Clone or fetch ---
	isExistingRepo := isGitRepo(targetDir)

	if !isExistingRepo {
		if err := cloneRepo(cloneURL, targetDir, ref, fetchDepthRaw, fetchTags, sparseCheckout, sparseConeMode); err != nil {
			return err
		}
	} else {
		if err := fetchRepo(targetDir, cloneURL, ref, fetchDepthRaw, fetchTags, clean); err != nil {
			return err
		}
	}

	// --- Sparse checkout (for existing repos or after init) ---
	if sparseCheckout != "" && isExistingRepo {
		if err := configureSparseCheckout(targetDir, sparseCheckout, sparseConeMode); err != nil {
			return err
		}
	}

	// --- Submodules ---
	if err := handleSubmodules(targetDir, submodules, lfs); err != nil {
		return err
	}

	// --- LFS ---
	if lfs {
		if out, err := runGitIn(targetDir, "lfs", "pull"); err != nil {
			return fmt.Errorf("git lfs pull: %w\n%s", err, out)
		}
	}

	// --- Get commit SHA ---
	commitOut, err := runGitIn(targetDir, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("git rev-parse HEAD: %w\n%s", err, commitOut)
	}
	commitSHA := strings.TrimSpace(string(commitOut))

	// --- Resolve effective ref ---
	effectiveRef := ref
	abbrevOut, abbrevErr := runGitIn(targetDir, "rev-parse", "--abbrev-ref", "HEAD")
	if abbrevErr == nil {
		abbrev := strings.TrimSpace(string(abbrevOut))
		if abbrev != "HEAD" {
			effectiveRef = abbrev
		}
	}
	if effectiveRef == "" {
		effectiveRef = commitSHA
	}

	ac.Info(fmt.Sprintf("Checked out commit: %s", commitSHA))
	ac.Info(fmt.Sprintf("Effective ref: %s", effectiveRef))

	// --- Set outputs ---
	if err := ac.SetOutput("commit", commitSHA); err != nil {
		ac.Warning(fmt.Sprintf("could not set commit output: %v", err), nil)
	}
	if err := ac.SetOutput("ref", effectiveRef); err != nil {
		ac.Warning(fmt.Sprintf("could not set ref output: %v", err), nil)
	}

	// --- Job summary ---
	ac.JobSummary.
		AddHeading("Checkout", 2).
		AddTable([][]ac.SummaryTableCell{
			{
				{Data: "Repository", Header: true},
				{Data: "Ref", Header: true},
				{Data: "Commit", Header: true},
				{Data: "Path", Header: true},
			},
			{
				{Data: repository},
				{Data: effectiveRef},
				{Data: commitSHA},
				{Data: targetDir},
			},
		})
	if err := ac.JobSummary.Write(nil); err != nil {
		ac.Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
	}

	return nil
}

// isGitRepo returns true if targetDir contains a .git entry.
func isGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info != nil
}

// runGitIn runs a git command inside dir and returns combined output.
func runGitIn(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// cloneRepo performs a fresh git clone.
func cloneRepo(cloneURL, targetDir, ref, fetchDepth string, fetchTags bool, sparseCheckout string, sparseConeMode bool) error {
	args := []string{"clone"}

	if fetchDepth != "0" && fetchDepth != "" {
		args = append(args, "--depth", fetchDepth)
	}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	if !fetchTags {
		args = append(args, "--no-tags")
	}
	if sparseCheckout != "" {
		args = append(args, "--no-checkout", "--filter=blob:none")
	}
	args = append(args, cloneURL, targetDir)

	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone: %w\n%s", err, out)
	}

	if sparseCheckout != "" {
		if err := configureSparseCheckout(targetDir, sparseCheckout, sparseConeMode); err != nil {
			return err
		}
		checkoutRef := ref
		if checkoutRef == "" {
			checkoutRef = "HEAD"
		}
		if out, err := runGitIn(targetDir, "checkout", checkoutRef); err != nil {
			return fmt.Errorf("git checkout after sparse init: %w\n%s", err, out)
		}
	}

	return nil
}

// fetchRepo fetches into an existing git repository.
func fetchRepo(targetDir, cloneURL, ref, fetchDepth string, fetchTags, clean bool) error {
	if clean {
		if out, err := runGitIn(targetDir, "clean", "-ffdx"); err != nil {
			return fmt.Errorf("git clean: %w\n%s", err, out)
		}
		if out, err := runGitIn(targetDir, "reset", "--hard"); err != nil {
			return fmt.Errorf("git reset: %w\n%s", err, out)
		}
	}

	// Ensure origin points to the right URL.
	runGitIn(targetDir, "remote", "set-url", "origin", cloneURL) //nolint

	fetchArgs := []string{"fetch"}
	if fetchDepth != "0" && fetchDepth != "" {
		fetchArgs = append(fetchArgs, "--depth", fetchDepth)
	}
	if fetchTags {
		fetchArgs = append(fetchArgs, "--tags")
	}
	fetchArgs = append(fetchArgs, "origin")
	if ref != "" {
		fetchArgs = append(fetchArgs, ref)
	}

	if out, err := runGitIn(targetDir, fetchArgs...); err != nil {
		return fmt.Errorf("git fetch: %w\n%s", err, out)
	}

	checkoutTarget := "FETCH_HEAD"
	if ref != "" {
		checkoutTarget = ref
	}
	if out, err := runGitIn(targetDir, "checkout", checkoutTarget); err != nil {
		// Try FETCH_HEAD as fallback.
		if out2, err2 := runGitIn(targetDir, "checkout", "FETCH_HEAD"); err2 != nil {
			return fmt.Errorf("git checkout %s: %w\n%s", checkoutTarget, err, out)
		} else {
			_ = out2
		}
	}

	return nil
}

// configureSparseCheckout enables sparse checkout and writes patterns.
func configureSparseCheckout(targetDir, patterns string, coneMode bool) error {
	initArgs := []string{"sparse-checkout", "init"}
	if coneMode {
		initArgs = append(initArgs, "--cone")
	} else {
		initArgs = append(initArgs, "--no-cone")
	}
	if out, err := runGitIn(targetDir, initArgs...); err != nil {
		return fmt.Errorf("git sparse-checkout init: %w\n%s", err, out)
	}

	var patternList []string
	for _, p := range strings.Split(patterns, "\n") {
		p = strings.TrimSpace(p)
		if p != "" {
			patternList = append(patternList, p)
		}
	}

	setArgs := append([]string{"sparse-checkout", "set"}, patternList...)
	if out, err := runGitIn(targetDir, setArgs...); err != nil {
		return fmt.Errorf("git sparse-checkout set: %w\n%s", err, out)
	}

	return nil
}

// handleSubmodules handles submodule init/update based on the input value.
func handleSubmodules(targetDir, submodules string, lfs bool) error {
	switch submodules {
	case "false", "":
		return nil
	case "recursive":
		if out, err := runGitIn(targetDir, "submodule", "sync", "--recursive"); err != nil {
			return fmt.Errorf("git submodule sync --recursive: %w\n%s", err, out)
		}
		if out, err := runGitIn(targetDir, "submodule", "update", "--init", "--recursive"); err != nil {
			return fmt.Errorf("git submodule update --init --recursive: %w\n%s", err, out)
		}
	default: // "true" or any non-false value
		if out, err := runGitIn(targetDir, "submodule", "sync"); err != nil {
			return fmt.Errorf("git submodule sync: %w\n%s", err, out)
		}
		if out, err := runGitIn(targetDir, "submodule", "update", "--init"); err != nil {
			return fmt.Errorf("git submodule update --init: %w\n%s", err, out)
		}
	}

	if lfs {
		// Run lfs pull in each submodule.
		if out, err := runGitIn(targetDir, "submodule", "foreach", "--recursive", "git lfs pull"); err != nil {
			return fmt.Errorf("git lfs pull in submodules: %w\n%s", err, out)
		}
	}

	return nil
}
