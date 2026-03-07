# Skill: Add a new command to an existing `<family>` action

**Trigger:** User wants to add a new command to an existing action family.

---

## 1. Create `cmd/<name>.go`

File name convention: snake_case matching the command name (e.g. `check_pr_title.go` for `check-pr-title`).

Commands receive all their options via the shared `input` JSON blob (forwarded as `INPUT_INPUT`) parsed into a typed struct. Pre-initialize the struct with defaults so callers can omit the entire input for the common case.

```go
package cmd

import (
    "fmt"
    "os"

    ac "github.com/bshore/gha/internal/actions-core"
)

func init() { Register("<command-name>", run<Command>) }

type <Command>Input struct {
    MyString string `json:"my_string"`
    MyBool   bool   `json:"my_bool"`
}

func run<Command>() error {
    // Pre-initialize with sane defaults — json.Unmarshal only overwrites fields
    // present in the JSON, so absent fields retain these values.
    inp := <Command>Input{
        MyString: "default-value",
        MyBool:   true,
    }
    if err := ac.GetStructuredInput("input", &inp); err != nil {
        return fmt.Errorf("parsing input: %w", err)
    }

    // Auto-detect from environment for any still-zero fields.
    if inp.MyString == "" {
        inp.MyString = detectFromEnv()
        ac.Info(fmt.Sprintf("Auto-detected my_string: %s", inp.MyString))
    }

    // ... implement logic ...

    // Set output
    if err := ac.SetOutput("my_output", result); err != nil {
        ac.Warning(fmt.Sprintf("could not set my_output: %v", err), nil)
    }

    // Write job summary
    ac.JobSummary.
        AddHeading("<Command>", 2).
        AddTable([][]ac.SummaryTableCell{
            {{Data: "Input", Header: true}, {Data: "Result", Header: true}},
            {{Data: inp.MyString}, {Data: "✅ success"}},
        })
    if err := ac.JobSummary.Write(nil); err != nil {
        ac.Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
    }

    return nil
}
```

---

## 2. Input patterns

### Preferred: structured input (JSON or HCL) with pre-initialized defaults

All command-specific options belong on the command's `*Input` struct, not as separate top-level action inputs. This keeps `action.yml` flat and lets callers omit the input entirely for the common case.

`GetStructuredInput` auto-detects the format: a trimmed value starting with `{` is parsed as JSON, anything else as HCL native syntax (`key = value`). Both use the same `json:"..."` struct tags.

```go
type MyInput struct {
    Repo    string `json:"repo"`
    Depth   int    `json:"depth"`
    Enabled bool   `json:"enabled"`
}

inp := MyInput{
    Repo:    os.Getenv("GITHUB_REPOSITORY"), // env-based default
    Depth:   1,
    Enabled: true,
}
if err := ac.GetStructuredInput("input", &inp); err != nil {
    return fmt.Errorf("parsing input: %w", err)
}
```

`json.Unmarshal` only writes fields present in the input — pre-initialized values survive for any omitted fields. Callers can pass JSON or HCL interchangeably:

```yaml
# JSON
input: '{"depth": 2}'

# HCL (tfvars-style) — friendlier for multiline block scalars
input: |
  depth   = 2
  enabled = false
```

### Auto-detecting from the environment

After structured input parse, probe files or git to fill in remaining zero-value fields:

```go
// Auto-detect from git
if inp.Base == "" {
    inp.Base = defaultBase() // git symbolic-ref → fallback candidates → "main"
    ac.Info(fmt.Sprintf("Auto-detected base branch: %s", inp.Base))
}

// Auto-detect from filesystem
if inp.GoVersionFile == "" && inp.GoVersion == "" {
    if _, err := os.Stat("go.mod"); err == nil {
        inp.GoVersionFile = "go.mod"
        ac.Info("Auto-detected go.mod for Go version")
    }
}
```

Always log auto-detected values with `ac.Info` so runners can see what was inferred.

### Boolean input (standalone, non-struct)

`GetBooleanInput` errors on empty string — always use this helper:

```go
func boolInputOrDefault(name string, defaultVal bool) (bool, error) {
    raw, _ := ac.GetInput(name, nil)
    if raw == "" {
        return defaultVal, nil
    }
    return ac.GetBooleanInput(name, nil)
}
```

### Required input (no default possible)

```go
myInput, _ := ac.GetInput("my_input", nil)
if myInput == "" {
    return fmt.Errorf("input 'my_input' is required")
}
```

### Naming rule

**Input names must use underscores, not hyphens.**
`ac.GetInput` reads `INPUT_<NAME>` after uppercasing and replacing spaces→underscores only.
A hyphen in the name produces an invalid env var like `INPUT_MY-INPUT`.

---

## 3. Output pattern

```go
if err := ac.SetOutput("output_name", value); err != nil {
    ac.Warning(fmt.Sprintf("could not set output_name: %v", err), nil)
}
```

---

## 4. Job summary pattern

```go
ac.JobSummary.
    AddHeading("Title", 2).
    AddTable([][]ac.SummaryTableCell{
        {
            {Data: "Col1", Header: true},
            {Data: "Col2", Header: true},
        },
        {
            {Data: val1},
            {Data: val2},
        },
    }).
    AddSeparator().
    AddList(items, false)  // false = unordered list
if err := ac.JobSummary.Write(nil); err != nil {
    ac.Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
}
```

Available `JobSummary` methods: `AddHeading`, `AddTable`, `AddList`, `AddCodeBlock`,
`AddDetails`, `AddSeparator`, `AddBreak`, `AddQuote`, `AddLink`, `AddImage`, `AddRaw`.

---

## 5. PR comment pattern

The family-level `main.go` already handles upsert-on-failure and delete-on-success for the
family's marker. No per-command PR comment work is needed unless the command wants to post
a command-specific comment (rare). If so:

```go
// In your command function, the marker is defined in main.go:
// const prCommentMarker = "<!-- <family>-error -->"
// Pass it through if needed, or define a sub-marker for the command.
```

---

## 6. Update `action.yml`

The `action.yml` already has a single `input` field shared across all commands — no new input needs to be added. The `INPUT_INPUT` env var is already forwarded in the `run` step. No `action.yml` changes are needed when adding a new command unless it introduces new action-level outputs.

### Add a new output (if needed)

```yaml
outputs:
  # ... existing outputs ...
  my_new_output:
    description: Description of the new output
    value: ${{ steps.run.outputs.my_new_output }}
```

---

## 7. Update the test workflow

Edit `.github/workflows/test-actions-<family>.yml` and add a step for the new command, following the existing step pattern. Insert it after the `yoink` step and before or after other command steps:

```yaml
      - name: <command> (source build)
        uses: ./actions/<family>
        env:
          ACTIONS_CACHE_URL: ${{ steps.yoink.outputs.ACTIONS_CACHE_URL }}
          ACTIONS_RUNTIME_TOKEN: ${{ steps.yoink.outputs.ACTIONS_RUNTIME_TOKEN }}
          ACTIONS_RESULTS_URL: ${{ steps.yoink.outputs.ACTIONS_RESULTS_URL }}
          ACTIONS_RUNTIME_URL: ${{ steps.yoink.outputs.ACTIONS_RUNTIME_URL }}
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: <command>
          self_cache: "false"
          input: |
            # include representative inputs if useful for smoke-testing
```

---

## 8. Update `README.md`

Edit `actions/<family>/README.md`:

1. Add the command to the **Commands** table
2. Add a **JSON input schema** sub-section titled `` `input` — `<command>` command ``:
   - Include a field table with `field`, `type`, `default`, `description` columns
   - For auto-detected defaults, use `"auto-detected"` in the Default column and explain the detection logic in Description
   - Add a prose line above the table: `"All fields are optional. Omit input entirely for standard usage."`
3. Add any new outputs to the **Outputs** table
4. Add a **Usage** example showing the minimal invocation (no `input`) first, then an override example if useful

---

## 9. Checklist

- [ ] New file `cmd/<name>.go` with `init()` self-registration
- [ ] `*Input` struct pre-initialized with sane defaults before `GetStructuredInput("input", &inp)`
- [ ] Any zero-value fields auto-detected from environment after JSON parse, logged with `ac.Info`
- [ ] Command-specific options are struct fields on `*Input`, NOT separate top-level action inputs
- [ ] Input names use underscores only
- [ ] Boolean inputs in structs use struct pre-init (not `boolInputOrDefault`); standalone boolean inputs use `boolInputOrDefault`
- [ ] `ac.SetOutput` calls wrapped in warning-on-error
- [ ] `ac.JobSummary.Write` calls wrapped in warning-on-error
- [ ] No `action.yml` input changes needed (shared `input` field already exists and `INPUT_INPUT` is already forwarded)
- [ ] New outputs added to `action.yml` outputs block if applicable
- [ ] `go build ./actions/<family>/...` — compiles cleanly
- [ ] `go vet ./actions/<family>/...` — no issues
- [ ] Command added to `README.md` Commands table
- [ ] JSON schema sub-section added to `README.md` under `` `input` — `<command>` command ``
- [ ] New outputs documented in `README.md` Outputs table
- [ ] Usage example added to `README.md`
- [ ] New command step added to `.github/workflows/test-actions-<family>.yml`
