package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// runGDScript invokes a .gd file using the Godot binary in headless mode.
//
// Invocation convention (cross-platform, no stdin piping required):
//
//	godot --headless --script <script> -- <json_args>
//
// The GDScript tool reads OS.get_cmdline_user_args()[0] as its JSON input
// and prints its JSON result to stdout.
//
// Why not stdin? Godot's headless mode stdout/stdin handling on Windows is
// unreliable for piping. CLI user args work identically on all platforms.
func runGDScript(ctx context.Context, godotBin, scriptPath string, args map[string]any, timeoutSecs int) (any, error) {
	if timeoutSecs <= 0 {
		timeoutSecs = 30
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	inputJSON, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("gdscript tool: marshal args: %w", err)
	}

	// Resolve the script path so the error message is unambiguous.
	absScript, err := filepath.Abs(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("gdscript tool: resolve script path: %w", err)
	}
	if _, err := os.Stat(absScript); err != nil {
		return nil, fmt.Errorf("gdscript tool: script not found: %s", absScript)
	}

	cmd := exec.CommandContext(ctx,
		godotBin,
		"--headless",
		"--script", absScript,
		"--", // everything after -- is user args, accessible via OS.get_cmdline_user_args()
		string(inputJSON),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if runErr := cmd.Run(); runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("gdscript tool %q: timed out after %ds", scriptPath, timeoutSecs)
		}
		// Godot writes its own startup noise to stderr; include it for debugging.
		errDetail := stderr.String()
		if errDetail == "" {
			errDetail = runErr.Error()
		}
		return nil, fmt.Errorf("gdscript tool %q: %s", scriptPath, errDetail)
	}

	// Godot prints engine/debug lines to stdout before script output.
	// Find the first line that looks like JSON (starts with '{' or '[').
	output := extractJSONLine(stdout.Bytes())
	if len(output) == 0 {
		// Tool ran fine but returned nothing — treat as success with empty result.
		log.Printf("gdscript tool %q: no JSON output", scriptPath)
		return map[string]any{"status": "ok"}, nil
	}

	var result any
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("gdscript tool %q: invalid JSON output: %w", scriptPath, err)
	}
	return result, nil
}

// extractJSONLine scans output lines for the first one that starts with { or [.
// This skips Godot's engine startup messages which always precede script output.
func extractJSONLine(out []byte) []byte {
	for _, line := range bytes.Split(out, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) > 0 && (line[0] == '{' || line[0] == '[') {
			return line
		}
	}
	return nil
}
