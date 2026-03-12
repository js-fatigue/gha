package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	ac "github.com/js-fatigue/gha/internal/actions-core"
)

func init() { Register("build", runBuild) }

type BuildInput struct {
	Context    string   `json:"context"`
	Dockerfile string   `json:"dockerfile"`
	Tags       []string `json:"tags"`
	BuildArgs  []string `json:"build_args"`
	Platform   string   `json:"platform"`
	Push       bool     `json:"push"`
	Load       bool     `json:"load"`
	Target     string   `json:"target"`
	NoCache    bool     `json:"no_cache"`
	Labels     []string `json:"labels"`
	CacheFrom  string   `json:"cache_from"`
	CacheTo    string   `json:"cache_to"`
}

func runBuild() error {
	inp := BuildInput{
		Context: ".",
	}
	if err := ac.GetStructuredInput("input", &inp); err != nil {
		return fmt.Errorf("parsing input: %w", err)
	}

	args := []string{"buildx", "build"}

	if inp.Dockerfile != "" {
		args = append(args, "-f", inp.Dockerfile)
	}
	for _, tag := range inp.Tags {
		args = append(args, "-t", tag)
	}
	for _, arg := range inp.BuildArgs {
		args = append(args, "--build-arg", arg)
	}
	for _, label := range inp.Labels {
		args = append(args, "--label", label)
	}
	if inp.Platform != "" {
		args = append(args, "--platform", inp.Platform)
	}
	if inp.Target != "" {
		args = append(args, "--target", inp.Target)
	}
	if inp.NoCache {
		args = append(args, "--no-cache")
	}
	if inp.CacheFrom != "" {
		args = append(args, "--cache-from", inp.CacheFrom)
	}
	if inp.CacheTo != "" {
		args = append(args, "--cache-to", inp.CacheTo)
	}
	if inp.Push {
		args = append(args, "--push")
	} else if inp.Load {
		args = append(args, "--load")
	}

	// Capture image ID and digest via metadata file
	metadataPath := ""
	if mf, err := os.CreateTemp("", "docker-build-metadata-*.json"); err == nil {
		metadataPath = mf.Name()
		mf.Close()
		defer os.Remove(metadataPath)
		args = append(args, "--metadata-file", metadataPath)
	}

	args = append(args, inp.Context)

	ac.Info(fmt.Sprintf("Running: docker %s", strings.Join(args, " ")))

	c := exec.Command("docker", args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}

	ac.Info("Build succeeded")

	if metadataPath != "" {
		data, err := os.ReadFile(metadataPath)
		if err == nil && len(data) > 0 {
			var meta map[string]interface{}
			if err := json.Unmarshal(data, &meta); err == nil {
				if id, ok := meta["containerimage.config.digest"].(string); ok && id != "" {
					if err := ac.SetOutput("image_id", id); err != nil {
						ac.Warning(fmt.Sprintf("could not set image_id output: %v", err), nil)
					}
				}
				if digest, ok := meta["containerimage.digest"].(string); ok && digest != "" {
					if err := ac.SetOutput("digest", digest); err != nil {
						ac.Warning(fmt.Sprintf("could not set digest output: %v", err), nil)
					}
				}
			}
		}
	}

	tagsDisplay := strings.Join(inp.Tags, ", ")
	if tagsDisplay == "" {
		tagsDisplay = "(none)"
	}
	platformDisplay := inp.Platform
	if platformDisplay == "" {
		platformDisplay = "(native)"
	}
	ac.JobSummary.AddHeading("Docker Build", 2).AddTable([][]ac.SummaryTableCell{
		{
			{Data: "Context", Header: true},
			{Data: "Tags", Header: true},
			{Data: "Platform", Header: true},
			{Data: "Push", Header: true},
			{Data: "Result", Header: true},
		},
		{
			{Data: inp.Context},
			{Data: tagsDisplay},
			{Data: platformDisplay},
			{Data: fmt.Sprintf("%v", inp.Push)},
			{Data: "✅ success"},
		},
	})
	if err := ac.JobSummary.Write(nil); err != nil {
		ac.Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
	}

	return nil
}
