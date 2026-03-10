package ac

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
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

// CacheInput is the JSON payload for the `input` field when running a cache command.
type CacheInput struct {
	Action      string   `json:"action"`       // "restore" or "save"
	Path        []string `json:"path"`         // paths/globs to cache
	Key         string   `json:"key"`          // primary cache key
	RestoreKeys []string `json:"restore_keys"` // fallback prefix keys (restore only)
	FailOnMiss  bool     `json:"fail_on_miss"` // error on cache miss (restore only)
}

// v2 Twirp cache service request/response types.
type v2GetCacheReq struct {
	Key         string   `json:"key"`
	RestoreKeys []string `json:"restoreKeys"`
	Version     string   `json:"version"`
}
type v2GetCacheResp struct {
	Ok                bool   `json:"ok"`
	SignedDownloadURL string `json:"signed_download_url"`
	MatchedKey        string `json:"matched_key"`
}
type v2CreateCacheReq struct {
	Key     string `json:"key"`
	Version string `json:"version"`
}
type v2CreateCacheResp struct {
	Ok              bool   `json:"ok"`
	SignedUploadURL string `json:"signed_upload_url"`
}
type v2FinalizeReq struct {
	Key       string `json:"key"`
	Version   string `json:"version"`
	SizeBytes int64  `json:"size_bytes"`
}
type v2FinalizeResp struct {
	Ok bool `json:"ok"`
}

// RunCache reads input JSON and runs restore or save.
func RunCache() error {
	inp := CacheInput{}
	if err := GetStructuredInput("input", &inp); err != nil {
		return fmt.Errorf("parsing input: %w", err)
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
	baseURL, err := cacheServiceURL()
	if err != nil {
		return err
	}
	token, err := cacheToken()
	if err != nil {
		return err
	}

	reqBody, _ := json.Marshal(v2GetCacheReq{
		Key:         inp.Key,
		RestoreKeys: inp.RestoreKeys,
		Version:     cacheVersion(inp.Path),
	})
	resp, err := cacheAPIRequest(http.MethodPost,
		baseURL+"GetCacheEntryDownloadURL",
		token, reqBody, "application/json")
	if err != nil {
		return fmt.Errorf("cache lookup: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cache lookup returned %d: %s", resp.StatusCode, body)
	}

	var cacheResp v2GetCacheResp
	if err := json.NewDecoder(resp.Body).Decode(&cacheResp); err != nil {
		return fmt.Errorf("decoding cache lookup response: %w", err)
	}

	if !cacheResp.Ok || cacheResp.SignedDownloadURL == "" {
		Info(fmt.Sprintf("cache: miss for key %q", inp.Key))
		if err := SetOutput("cache_hit", "false"); err != nil {
			Warning(fmt.Sprintf("could not set cache_hit: %v", err), nil)
		}
		if inp.FailOnMiss {
			return fmt.Errorf("cache miss for key %q", inp.Key)
		}
		JobSummary.AddHeading("Cache Restore", 2).AddTable([][]SummaryTableCell{
			{{Data: "Key", Header: true}, {Data: "Result", Header: true}},
			{{Data: inp.Key}, {Data: "miss"}},
		})
		if err := JobSummary.Write(nil); err != nil {
			Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
		}
		return nil
	}

	Info(fmt.Sprintf("cache: hit — matched key %q", cacheResp.MatchedKey))

	dlResp, err := http.Get(cacheResp.SignedDownloadURL)
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

	JobSummary.AddHeading("Cache Restore", 2).AddTable([][]SummaryTableCell{
		{{Data: "Key", Header: true}, {Data: "Matched", Header: true}, {Data: "Result", Header: true}},
		{{Data: inp.Key}, {Data: cacheResp.MatchedKey}, {Data: "✅ hit"}},
	})
	if err := JobSummary.Write(nil); err != nil {
		Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
	}

	return nil
}

// SaveCache archives the paths and uploads them under key.
func SaveCache(inp CacheInput) error {
	baseURL, err := cacheServiceURL()
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

	// Create (reserve) a cache entry.
	createBody, _ := json.Marshal(v2CreateCacheReq{
		Key:     inp.Key,
		Version: cacheVersion(inp.Path),
	})
	resp, err := cacheAPIRequest(http.MethodPost,
		baseURL+"CreateCacheEntry",
		token, createBody, "application/json")
	if err != nil {
		return fmt.Errorf("creating cache entry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		resp.Body.Close()
		Info(fmt.Sprintf("cache: key %q already exists, skipping save", inp.Key))
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("creating cache entry returned %d: %s", resp.StatusCode, body)
	}

	var createResp v2CreateCacheResp
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return fmt.Errorf("decoding create cache response: %w", err)
	}
	if !createResp.Ok {
		Info(fmt.Sprintf("cache: key %q already exists, skipping save", inp.Key))
		return nil
	}

	// Upload to Azure Block Blob.
	if err := putAzureBlockBlob(createResp.SignedUploadURL, archivePath); err != nil {
		return fmt.Errorf("uploading cache archive: %w", err)
	}

	// Finalize the cache entry.
	finalizeBody, _ := json.Marshal(v2FinalizeReq{
		Key:       inp.Key,
		Version:   cacheVersion(inp.Path),
		SizeBytes: archiveSize,
	})
	finalizeResp, err := cacheAPIRequest(http.MethodPost,
		baseURL+"FinalizeCacheEntryUpload",
		token, finalizeBody, "application/json")
	if err != nil {
		return fmt.Errorf("finalizing cache: %w", err)
	}
	defer finalizeResp.Body.Close()

	if finalizeResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(finalizeResp.Body)
		return fmt.Errorf("finalizing cache returned %d: %s", finalizeResp.StatusCode, body)
	}

	var finResp v2FinalizeResp
	if err := json.NewDecoder(finalizeResp.Body).Decode(&finResp); err != nil {
		return fmt.Errorf("decoding finalize response: %w", err)
	}
	if !finResp.Ok {
		return fmt.Errorf("cache finalize returned ok=false")
	}

	Info(fmt.Sprintf("cache: saved %q", inp.Key))

	JobSummary.AddHeading("Cache Save", 2).AddTable([][]SummaryTableCell{
		{{Data: "Key", Header: true}, {Data: "Size", Header: true}, {Data: "Result", Header: true}},
		{{Data: inp.Key}, {Data: fmt.Sprintf("%d bytes", archiveSize)}, {Data: "✅ saved"}},
	})
	if err := JobSummary.Write(nil); err != nil {
		Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
	}

	return nil
}

// SelfCacheBinary saves the running binary's directory to the Actions cache.
// It reads INPUT_SELF_CACHE (default true) and no-ops if caching is disabled or
// ACTIONS_RESULTS_URL is not set. The cache key mirrors bootstrap.sh:
//
//	<binary>-<RUNNER_OS>-<RUNNER_ARCH>-<GHA_<BINARY>_VERSION>
func SelfCacheBinary() {
	enabled, err := GetBooleanInputOrDefault("self_cache", true, nil)
	if err != nil || !enabled {
		return
	}
	if os.Getenv("ACTIONS_RESULTS_URL") == "" || os.Getenv("ACTIONS_RUNTIME_TOKEN") == "" {
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
	u := os.Getenv("ACTIONS_RESULTS_URL")
	if u == "" {
		return "", fmt.Errorf("ACTIONS_RESULTS_URL is not set (not running in GitHub Actions?)")
	}
	base := strings.TrimRight(u, "/")
	return base + "/twirp/github.actions.results.api.v1.CacheService/", nil
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
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return http.DefaultClient.Do(req)
}

const azureBlockSize = 64 * 1024 * 1024 // 64 MB

// putAzureBlockBlob uploads archivePath to an Azure Block Blob SAS URL using
// 64 MB blocks, matching the @actions/cache v4 upload strategy.
func putAzureBlockBlob(uploadURL, archivePath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close()

	buf := make([]byte, azureBlockSize)
	var blockIDs []string

	for i := 0; ; i++ {
		n, readErr := io.ReadFull(f, buf)
		if n == 0 {
			break
		}

		blockID := base64.StdEncoding.EncodeToString(fmt.Appendf([]byte{}, "%05d", i))
		blockIDs = append(blockIDs, blockID)

		blockURL := fmt.Sprintf("%s&comp=block&blockid=%s", uploadURL, url.QueryEscape(blockID))
		req, err := http.NewRequest(http.MethodPut, blockURL, bytes.NewReader(buf[:n]))
		if err != nil {
			return fmt.Errorf("building block %d request: %w", i, err)
		}
		req.Header.Set("x-ms-blob-type", "BlockBlob")
		req.Header.Set("Content-Type", "application/octet-stream")
		req.ContentLength = int64(n)

		blkResp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("uploading block %d: %w", i, err)
		}
		blkResp.Body.Close()
		if blkResp.StatusCode != http.StatusCreated {
			return fmt.Errorf("block %d upload returned %d", i, blkResp.StatusCode)
		}

		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("reading archive: %w", readErr)
		}
	}

	// Commit the block list.
	var xmlBody strings.Builder
	xmlBody.WriteString(`<?xml version="1.0" encoding="utf-8"?><BlockList>`)
	for _, id := range blockIDs {
		xmlBody.WriteString("<Latest>")
		xmlBody.WriteString(id)
		xmlBody.WriteString("</Latest>")
	}
	xmlBody.WriteString("</BlockList>")

	xmlBytes := []byte(xmlBody.String())
	blockListURL := uploadURL + "&comp=blocklist"
	req, err := http.NewRequest(http.MethodPut, blockListURL, bytes.NewReader(xmlBytes))
	if err != nil {
		return fmt.Errorf("building blocklist request: %w", err)
	}
	req.Header.Set("Content-Type", "application/xml")
	req.ContentLength = int64(len(xmlBytes))

	listResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("committing block list: %w", err)
	}
	listResp.Body.Close()
	if listResp.StatusCode != http.StatusCreated {
		return fmt.Errorf("block list commit returned %d", listResp.StatusCode)
	}

	return nil
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
			os.Chmod(target, 0755) // ensure writable for subsequent file extraction
		case tar.TypeSymlink:
			os.Remove(target) // ignore error; may not exist
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return fmt.Errorf("symlink %s: %w", target, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			dir := filepath.Dir(target)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("mkdir parent of %s: %w", target, err)
			}
			os.Chmod(dir, 0755) // ensure writable even if it pre-existed as 0555 (e.g. Go module cache)
			os.Remove(target)   // ignore error; handles read-only files (e.g. Go module cache uses 0444)
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
