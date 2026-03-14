package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// runExternal executes an external tool binary, passes args as JSON on stdin,
// and returns the parsed JSON from stdout.
//
// The external tool contract is simple and language-agnostic:
//   - Receive: JSON object on stdin
//   - Return:  JSON object on stdout
//   - Errors:  non-zero exit code + message on stderr
//
// This lets developers write tools in Python, Rust, Node.js, GDScript wrappers,
// or anything else that can read stdin and write stdout.
func runExternal(ctx context.Context, command string, args map[string]any, timeoutSecs int) (any, error) {
	if timeoutSecs <= 0 {
		timeoutSecs = 30
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	inputJSON, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("external tool: marshal args: %w", err)
	}

	cmd := exec.CommandContext(ctx, command)
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("external tool %q: timed out after %ds", command, timeoutSecs)
		}
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return nil, fmt.Errorf("external tool %q: %s", command, errMsg)
	}

	if stdout.Len() == 0 {
		return map[string]any{"status": "ok"}, nil
	}

	var result any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		// Tool returned non-JSON; wrap it as a plain string so the call still succeeds.
		return map[string]any{"output": stdout.String()}, nil
	}
	return result, nil
}
