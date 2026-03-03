package ac

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ExitCode mirrors the TS ExitCode enum.
type ExitCode int

const (
	ExitSuccess ExitCode = 0
	ExitFailure ExitCode = 1
)

// InputOptions configures GetInput behavior.
// SkipTrimWhitespace: when true, whitespace is NOT trimmed (default trims — matches TS).
type InputOptions struct {
	Required           bool
	SkipTrimWhitespace bool
}

// AnnotationProperties configures error/warning/notice annotations.
// Zero-value fields are omitted from the command.
type AnnotationProperties struct {
	Title       string
	File        string
	StartLine   int
	EndLine     int
	StartColumn int
	EndColumn   int
}

var currentExitCode = ExitSuccess

// --- Unexported helpers ---

func issueCommand(command string, props map[string]string, message string) {
	cmd := "::" + command
	if len(props) > 0 {
		parts := make([]string, 0, len(props))
		for k, v := range props {
			parts = append(parts, k+"="+escapeProperty(v))
		}
		cmd += " " + strings.Join(parts, ",")
	}
	cmd += "::" + escapeData(message)
	fmt.Println(cmd)
}

// issueFileCommand appends message (with a trailing newline) to the file
// pointed to by the GITHUB_<envKey> environment variable.
func issueFileCommand(envKey, message string) error {
	filePath := os.Getenv("GITHUB_" + envKey)
	if filePath == "" {
		return fmt.Errorf("GITHUB_%s is not set", envKey)
	}
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("opening GITHUB_%s file: %w", envKey, err)
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, message)
	return err
}

// prepareKeyValueMessage formats a key-value pair using the heredoc delimiter
// format required by GITHUB_ENV, GITHUB_OUTPUT, and GITHUB_STATE.
func prepareKeyValueMessage(key, value string) (string, error) {
	delim := generateDelimiter()
	if strings.Contains(value, delim) {
		return "", fmt.Errorf("generated delimiter appears in value; please re-run the action")
	}
	return fmt.Sprintf("%s<<%s\n%s\n%s", key, delim, value, delim), nil
}

// generateDelimiter returns a random "ghadelimiter_<hex>" string suitable for
// use as a heredoc delimiter in file commands.
func generateDelimiter() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return "ghadelimiter_" + hex.EncodeToString(b)
}

func escapeData(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

func escapeProperty(s string) string {
	s = escapeData(s)
	s = strings.ReplaceAll(s, ":", "%3A")
	s = strings.ReplaceAll(s, ",", "%2C")
	return s
}

// annotationPropsToMap converts an AnnotationProperties struct to a map of
// workflow command property names, omitting any zero-value fields.
func annotationPropsToMap(p *AnnotationProperties) map[string]string {
	if p == nil {
		return nil
	}
	m := make(map[string]string)
	if p.Title != "" {
		m["title"] = p.Title
	}
	if p.File != "" {
		m["file"] = p.File
	}
	if p.StartLine != 0 {
		m["line"] = fmt.Sprintf("%d", p.StartLine)
	}
	if p.EndLine != 0 {
		m["endLine"] = fmt.Sprintf("%d", p.EndLine)
	}
	if p.StartColumn != 0 {
		m["col"] = fmt.Sprintf("%d", p.StartColumn)
	}
	if p.EndColumn != 0 {
		m["endColumn"] = fmt.Sprintf("%d", p.EndColumn)
	}
	return m
}

// --- Variables ---

// ExportVariable sets name in the current process environment and exports it to
// subsequent steps. Uses GITHUB_ENV (file command) when available, otherwise
// falls back to the legacy ::set-env workflow command.
func ExportVariable(name, val string) error {
	if err := os.Setenv(name, val); err != nil {
		return err
	}
	if os.Getenv("GITHUB_ENV") != "" {
		msg, err := prepareKeyValueMessage(name, val)
		if err != nil {
			return err
		}
		return issueFileCommand("ENV", msg)
	}
	issueCommand("set-env", map[string]string{"name": name}, val)
	return nil
}

// SetSecret masks secret in the runner's log output.
func SetSecret(secret string) {
	issueCommand("add-mask", nil, secret)
}

// AddPath prepends inputPath to PATH for the current and subsequent steps.
// Uses GITHUB_PATH (file command) when available, otherwise falls back to the
// legacy ::add-path workflow command. Always updates the current process PATH.
func AddPath(inputPath string) error {
	if os.Getenv("GITHUB_PATH") != "" {
		if err := issueFileCommand("PATH", inputPath); err != nil {
			return err
		}
	} else {
		issueCommand("add-path", nil, inputPath)
	}
	current := os.Getenv("PATH")
	if current != "" {
		return os.Setenv("PATH", inputPath+string(os.PathListSeparator)+current)
	}
	return os.Setenv("PATH", inputPath)
}

// --- Inputs ---

// GetInput reads the named action input from the environment (INPUT_<NAME>).
// Returns an error if Required is set and the value is empty.
func GetInput(name string, opts *InputOptions) (string, error) {
	key := "INPUT_" + strings.ToUpper(strings.ReplaceAll(name, " ", "_"))
	val := os.Getenv(key)
	if opts == nil || !opts.SkipTrimWhitespace {
		val = strings.TrimSpace(val)
	}
	if opts != nil && opts.Required && val == "" {
		return "", fmt.Errorf("input required and not supplied: %s", name)
	}
	return val, nil
}

// GetMultilineInput reads a multiline action input, returning each non-empty line.
func GetMultilineInput(name string, opts *InputOptions) ([]string, error) {
	val, err := GetInput(name, opts)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(val, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if opts == nil || !opts.SkipTrimWhitespace {
			line = strings.TrimSpace(line)
		}
		if line != "" {
			result = append(result, line)
		}
	}
	return result, nil
}

// GetBooleanInputOrDefault reads a boolean action input, returning defaultVal
// when the input is empty. This avoids the error GetBooleanInput returns on an
// empty string, which is the common case for optional boolean inputs.
func GetBooleanInputOrDefault(name string, defaultVal bool, opts *InputOptions) (bool, error) {
	val, err := GetInput(name, opts)
	if err != nil {
		return false, err
	}
	if val == "" {
		return defaultVal, nil
	}
	return GetBooleanInput(name, opts)
}

// GetJSONInput reads the named input as a JSON string and unmarshals it into out.
// If the input is empty, out is left unchanged (caller should pre-initialize with defaults).
func GetJSONInput(name string, out any) error {
	raw, err := GetInput(name, nil)
	if err != nil {
		return err
	}
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), out)
}

// GetBooleanInput reads a boolean action input using YAML 1.2 rules.
// Accepted true values: "true", "True", "TRUE".
// Accepted false values: "false", "False", "FALSE".
// All other values return an error.
func GetBooleanInput(name string, opts *InputOptions) (bool, error) {
	val, err := GetInput(name, opts)
	if err != nil {
		return false, err
	}
	switch val {
	case "true", "True", "TRUE":
		return true, nil
	case "false", "False", "FALSE":
		return false, nil
	default:
		return false, fmt.Errorf("input %q is not a boolean value: %q", name, val)
	}
}

// --- Outputs ---

// SetOutput sets an action output parameter. Uses GITHUB_OUTPUT (file command)
// when available, otherwise falls back to the legacy ::set-output workflow command.
func SetOutput(name, value string) error {
	if os.Getenv("GITHUB_OUTPUT") != "" {
		msg, err := prepareKeyValueMessage(name, value)
		if err != nil {
			return err
		}
		return issueFileCommand("OUTPUT", msg)
	}
	fmt.Println()
	issueCommand("set-output", map[string]string{"name": name}, value)
	return nil
}

// --- Results / Exit ---

// SetFailed marks the action as failed and logs an error message.
// The exit code is applied when Exit() is called.
func SetFailed(message string) {
	currentExitCode = ExitFailure
	Error(message, nil)
}

// Exit calls os.Exit with the current exit code.
// Intended for use as: defer core.Exit()
func Exit() {
	os.Exit(int(currentExitCode))
}

// --- Logging ---

// IsDebug returns true when the runner has debug logging enabled (RUNNER_DEBUG=1).
func IsDebug() bool {
	return os.Getenv("RUNNER_DEBUG") == "1"
}

// Debug emits a ::debug:: workflow command. Output is only visible when RUNNER_DEBUG=1.
func Debug(message string) {
	issueCommand("debug", nil, message)
}

// Info writes message directly to stdout.
func Info(message string) {
	fmt.Println(message)
}

// Error logs an error annotation with optional source location properties.
func Error(message string, props *AnnotationProperties) {
	issueCommand("error", annotationPropsToMap(props), message)
}

// Warning logs a warning annotation with optional source location properties.
func Warning(message string, props *AnnotationProperties) {
	issueCommand("warning", annotationPropsToMap(props), message)
}

// Notice logs a notice annotation with optional source location properties.
func Notice(message string, props *AnnotationProperties) {
	issueCommand("notice", annotationPropsToMap(props), message)
}

// SetCommandEcho enables or disables echoing of workflow commands to the log.
func SetCommandEcho(enabled bool) {
	val := "off"
	if enabled {
		val = "on"
	}
	issueCommand("echo", nil, val)
}

// --- Groups ---

// StartGroup begins a collapsible log group with the given name.
func StartGroup(name string) {
	issueCommand("group", nil, name)
}

// EndGroup ends the current collapsible log group.
func EndGroup() {
	issueCommand("endgroup", nil, "")
}

// Group runs fn inside a named collapsible log group, always closing the group
// even if fn returns an error.
func Group(name string, fn func() error) error {
	StartGroup(name)
	defer EndGroup()
	return fn()
}

// --- State ---

// SaveState persists state for use by the action's post step. Uses GITHUB_STATE
// (file command) when available, otherwise falls back to the legacy
// ::save-state workflow command.
func SaveState(name, value string) error {
	if os.Getenv("GITHUB_STATE") != "" {
		msg, err := prepareKeyValueMessage(name, value)
		if err != nil {
			return err
		}
		return issueFileCommand("STATE", msg)
	}
	issueCommand("save-state", map[string]string{"name": name}, value)
	return nil
}

// GetState retrieves state saved by a previous step of this action (STATE_<name>).
func GetState(name string) string {
	return os.Getenv("STATE_" + name)
}
