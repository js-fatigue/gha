package ac

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const cacheChunkSize = 32 * 1024 * 1024 // 32 MB

// CacheInput is the JSON payload for cache_input.
type CacheInput struct {
	Action      string   `json:"action"`       // "restore" or "save"
	Path        []string `json:"path"`         // paths/globs to cache
	Key         string   `json:"key"`          // primary cache key
	RestoreKeys []string `json:"restore_keys"` // fallback prefix keys (restore only)
	FailOnMiss  bool     `json:"fail_on_miss"` // error on cache miss (restore only)
}

type cacheEntry struct {
	CacheKey        string `json:"cacheKey"`
	ArchiveLocation string `json:"archiveLocation"`
}

type reserveResponse struct {
	CacheID int `json:"cacheId"`
}

// RunCache reads cache_input JSON and runs restore or save.
func RunCache() error {
	inp := CacheInput{}
	if err := GetJSONInput("cache_input", &inp); err != nil {
		return fmt.Errorf("parsing cache_input: %w", err)
	}
	if len(inp.Path) == 0 {
		return fmt.Errorf("cache: path is required")
	}
	if inp.Key == "" {
		return fmt.Errorf("cache: key is required")
	}
	switch inp.Action {
	case "restore":
		return RestoreCache(inp)
	case "save":
		return SaveCache(inp)
	default:
		return fmt.Errorf("cache: action must be 'restore' or 'save', got %q", inp.Action)
	}
}

// RestoreCache looks up the cache by key and extracts the archive if found.
func RestoreCache(inp CacheInput) error {
	serviceURL, err := cacheServiceURL()
	if err != nil {
		return err
	}
	token, err := cacheToken()
	if err != nil {
		return err
	}

	keys := append([]string{inp.Key}, inp.RestoreKeys...)
	params := url.Values{
		"keys":    []string{strings.Join(keys, ",")},
		"version": []string{cacheVersion(inp.Path)},
	}
	lookupURL := serviceURL + "/_apis/artifactcache/cache?" + params.Encode()

	resp, err := cacheAPIRequest(http.MethodGet, lookupURL, token, nil, "")
	if err != nil {
		return fmt.Errorf("cache lookup: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		Info(fmt.Sprintf("cache: miss for key %q", inp.Key))
		if err := SetOutput("cache_hit", "false"); err != nil {
			Warning(fmt.Sprintf("could not set cache_hit: %v", err), nil)
		}
		if inp.FailOnMiss {
			return fmt.Errorf("cache miss for key %q", inp.Key)
		}
		JobSummary.
			AddHeading("Cache Restore", 2).
			AddTable([][]SummaryTableCell{
				{{Data: "Key", Header: true}, {Data: "Result", Header: true}},
				{{Data: inp.Key}, {Data: "miss"}},
			})
		if err := JobSummary.Write(nil); err != nil {
			Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
		}
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cache lookup returned %d: %s", resp.StatusCode, body)
	}

	var entry cacheEntry
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return fmt.Errorf("decoding cache lookup response: %w", err)
	}
	Info(fmt.Sprintf("cache: hit — matched key %q", entry.CacheKey))

	dlResp, err := http.Get(entry.ArchiveLocation)
	if err != nil {
		return fmt.Errorf("downloading cache archive: %w", err)
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading cache archive returned %d", dlResp.StatusCode)
	}

	if err := extractTarGz(dlResp.Body); err != nil {
		return fmt.Errorf("extracting cache archive: %w", err)
	}

	if err := SetOutput("cache_hit", "true"); err != nil {
		Warning(fmt.Sprintf("could not set cache_hit: %v", err), nil)
	}

	JobSummary.
		AddHeading("Cache Restore", 2).
		AddTable([][]SummaryTableCell{
			{{Data: "Key", Header: true}, {Data: "Matched", Header: true}, {Data: "Result", Header: true}},
			{{Data: inp.Key}, {Data: entry.CacheKey}, {Data: "✅ hit"}},
		})
	if err := JobSummary.Write(nil); err != nil {
		Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
	}

	return nil
}

// SaveCache archives the paths and uploads them under key.
func SaveCache(inp CacheInput) error {
	serviceURL, err := cacheServiceURL()
	if err != nil {
		return err
	}
	token, err := cacheToken()
	if err != nil {
		return err
	}

	Info(fmt.Sprintf("cache: archiving %v", inp.Path))
	archivePath, archiveSize, err := createTarGz(inp.Path)
	if err != nil {
		return fmt.Errorf("creating cache archive: %w", err)
	}
	defer os.Remove(archivePath)
	Info(fmt.Sprintf("cache: archive size %d bytes", archiveSize))

	// Reserve a cache entry.
	reserveBody, _ := json.Marshal(map[string]string{
		"key":     inp.Key,
		"version": cacheVersion(inp.Path),
	})
	resp, err := cacheAPIRequest(http.MethodPost,
		serviceURL+"/_apis/artifactcache/caches",
		token, reserveBody, "application/json")
	if err != nil {
		return fmt.Errorf("reserving cache: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		Info(fmt.Sprintf("cache: key %q already exists, skipping save", inp.Key))
		return nil
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("reserving cache returned %d: %s", resp.StatusCode, body)
	}

	var reserveResp reserveResponse
	if err := json.NewDecoder(resp.Body).Decode(&reserveResp); err != nil {
		return fmt.Errorf("decoding reserve response: %w", err)
	}
	cacheID := reserveResp.CacheID
	uploadURL := fmt.Sprintf("%s/_apis/artifactcache/caches/%d", serviceURL, cacheID)

	// Upload in chunks.
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close()

	buf := make([]byte, cacheChunkSize)
	var offset int64
	for {
		n, readErr := io.ReadFull(f, buf)
		if n == 0 {
			break
		}
		chunk := buf[:n]
		end := offset + int64(n) - 1

		req, err := http.NewRequest(http.MethodPatch, uploadURL, bytes.NewReader(chunk))
		if err != nil {
			return fmt.Errorf("building upload request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/*", offset, end))
		req.ContentLength = int64(n)

		upResp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("uploading chunk at offset %d: %w", offset, err)
		}
		upResp.Body.Close()
		if upResp.StatusCode != http.StatusNoContent {
			return fmt.Errorf("upload chunk at offset %d returned %d", offset, upResp.StatusCode)
		}

		offset += int64(n)
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("reading archive: %w", readErr)
		}
	}

	// Commit the cache entry.
	commitBody, _ := json.Marshal(map[string]int64{"size": archiveSize})
	commitResp, err := cacheAPIRequest(http.MethodPost, uploadURL, token, commitBody, "application/json")
	if err != nil {
		return fmt.Errorf("committing cache: %w", err)
	}
	commitResp.Body.Close()
	if commitResp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("committing cache returned %d", commitResp.StatusCode)
	}

	Info(fmt.Sprintf("cache: saved %q", inp.Key))

	JobSummary.
		AddHeading("Cache Save", 2).
		AddTable([][]SummaryTableCell{
			{{Data: "Key", Header: true}, {Data: "Size", Header: true}, {Data: "Result", Header: true}},
			{{Data: inp.Key}, {Data: fmt.Sprintf("%d bytes", archiveSize)}, {Data: "✅ saved"}},
		})
	if err := JobSummary.Write(nil); err != nil {
		Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
	}

	return nil
}

// SelfCacheBinary saves the running binary's directory to the Actions cache.
// It reads INPUT_CACHE (default true) and no-ops if caching is disabled or
// ACTIONS_CACHE_URL is not set. The cache key mirrors bootstrap.sh:
//
//	<binary>-<RUNNER_OS>-<RUNNER_ARCH>-<GHA_<BINARY>_VERSION>
func SelfCacheBinary() {
	enabled, err := GetBooleanInputOrDefault("cache", true, nil)
	if err != nil || !enabled {
		return
	}
	if os.Getenv("ACTIONS_CACHE_URL") == "" || os.Getenv("ACTIONS_RUNTIME_TOKEN") == "" {
		return
	}

	execPath, err := os.Executable()
	if err != nil {
		Warning(fmt.Sprintf("self-cache: could not determine executable path: %v", err), nil)
		return
	}

	binaryName := strings.ToLower(filepath.Base(execPath))
	binaryDir := filepath.Dir(execPath)

	runnerOS := os.Getenv("RUNNER_OS")
	if runnerOS == "" {
		runnerOS = "Linux"
	}
	runnerArch := os.Getenv("RUNNER_ARCH")
	if runnerArch == "" {
		runnerArch = "X64"
	}

	releaseTag := os.Getenv("GHA_" + strings.ToUpper(binaryName) + "_VERSION")
	if releaseTag == "" {
		releaseTag = "latest"
	}

	cacheKey := binaryName + "-" + runnerOS + "-" + runnerArch + "-" + releaseTag
	if err := SaveCache(CacheInput{
		Action: "save",
		Path:   []string{binaryDir},
		Key:    cacheKey,
	}); err != nil {
		Warning(fmt.Sprintf("self-cache: %v", err), nil)
	}
}

// --- helpers ---

func cacheServiceURL() (string, error) {
	u := os.Getenv("ACTIONS_CACHE_URL")
	if u == "" {
		return "", fmt.Errorf("ACTIONS_CACHE_URL is not set (not running in GitHub Actions?)")
	}
	return strings.TrimRight(u, "/"), nil
}

func cacheToken() (string, error) {
	t := os.Getenv("ACTIONS_RUNTIME_TOKEN")
	if t == "" {
		return "", fmt.Errorf("ACTIONS_RUNTIME_TOKEN is not set (not running in GitHub Actions?)")
	}
	return t, nil
}

// cacheVersion mirrors actions/cache: SHA256 of newline-joined paths + RUNNER_OS.
func cacheVersion(paths []string) string {
	runnerOS := os.Getenv("RUNNER_OS")
	if runnerOS == "" {
		runnerOS = "Linux"
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s\n", strings.Join(paths, "\n"), runnerOS)
	return hex.EncodeToString(h.Sum(nil))
}

func expandPath(p string) (string, error) {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, p[2:]), nil
	}
	return filepath.Abs(p)
}

func cacheAPIRequest(method, rawURL, token string, body []byte, contentType string) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, rawURL, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json;api-version=6.0-preview.1")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return http.DefaultClient.Do(req)
}

// createTarGz archives paths into a temp tar.gz file, returns its path and size.
func createTarGz(paths []string) (string, int64, error) {
	tmp, err := os.CreateTemp("", "gha-cache-*.tar.gz")
	if err != nil {
		return "", 0, fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()

	gw := gzip.NewWriter(tmp)
	tw := tar.NewWriter(gw)

	for _, p := range paths {
		expanded, err := expandPath(p)
		if err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return "", 0, fmt.Errorf("expanding path %q: %w", p, err)
		}
		if err := addToTar(tw, expanded); err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return "", 0, fmt.Errorf("archiving %q: %w", expanded, err)
		}
	}

	if err := tw.Close(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", 0, err
	}
	if err := gw.Close(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", 0, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", 0, err
	}

	info, err := os.Stat(tmpName)
	if err != nil {
		os.Remove(tmpName)
		return "", 0, err
	}
	return tmpName, info.Size(), nil
}

// addToTar walks root and writes entries into tw with absolute paths (leading / stripped).
func addToTar(tw *tar.Writer, root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		var linkTarget string
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		hdr, err := tar.FileInfoHeader(info, linkTarget)
		if err != nil {
			return err
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		hdr.Name = strings.TrimPrefix(abs, "/")
		if info.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
}

// extractTarGz extracts a tar.gz stream, restoring files to their absolute paths.
func extractTarGz(r io.Reader) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar entry: %w", err)
		}

		// Reconstruct absolute path; filepath.Clean prevents traversal above /.
		target := filepath.Clean("/" + hdr.Name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("mkdir %s: %w", target, err)
			}
		case tar.TypeSymlink:
			os.Remove(target) // ignore error; may not exist
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return fmt.Errorf("symlink %s: %w", target, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("mkdir parent of %s: %w", target, err)
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return fmt.Errorf("creating %s: %w", target, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("writing %s: %w", target, err)
			}
			f.Close()
		}
	}
	return nil
}
