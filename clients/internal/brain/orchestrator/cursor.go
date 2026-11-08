package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
)

/*
 * CursorExecutor runs Cursor's command-line agent headless (cursor-agent
 * --print), signed in as its owner signed it in.
 *
 * Its own modes do the limiting: "ask" only reads and answers; without
 * --force a command it wants to run is not run, so files are all it changes;
 * --force with its sandbox off is everything. What it wrote is read from its
 * stream-json events (https://cursor.com/docs/cli/reference/output-format,
 * read 2026-11-08): tool_call events whose tool writes, edits or deletes
 * carry the path, a shell call its command, and the result event the answer.
 */
type CursorExecutor struct{ Path string }

func (CursorExecutor) ID() string { return "cursor-agent" }

func (e CursorExecutor) Run(ctx context.Context, job Job) Outcome {
	return run(ctx, job, e.Argv(job), parseCursor)
}

// Argv is exactly what Cursor's agent is started with for this job.
func (e CursorExecutor) Argv(job Job) []string {
	argv := []string{e.Path, "--print", "--output-format", "stream-json", "--workspace", job.Dir}

	switch {
	case job.readOnly():
		argv = append(argv, "--mode", "ask")
	case job.full():
		argv = append(argv, "--force", "--sandbox", "disabled", "--approve-mcps")
	default:
		argv = append(argv, "--sandbox", "enabled")
	}

	if job.Model != "" {
		argv = append(argv, "--model", job.Model)
	}

	return append(argv, "--", job.Prompt)
}

// The tools of Cursor's agent that change a file, by the name of their call.
var cursorWrites = map[string]bool{
	"writeToolCall": true, "editToolCall": true, "deleteToolCall": true,
	"searchReplaceToolCall": true, "multiEditToolCall": true,
}

func parseCursor(line []byte, o *Outcome, dir string) {
	var event map[string]any

	if json.Unmarshal(line, &event) != nil {
		return
	}

	switch event["type"] {
	case "system":
		if m, ok := event["model"].(string); ok && o.Model == "" {
			o.Model = m
		}
	case "assistant":
		o.noted(textOf(event))
	case "tool_call":
		if event["subtype"] != "completed" {
			return
		}

		call, _ := event["tool_call"].(map[string]any)

		for name, raw := range call {
			body, _ := raw.(map[string]any)
			args, _ := body["args"].(map[string]any)

			switch {
			case cursorWrites[name]:
				path, _ := args["path"].(string)

				if result, ok := body["result"].(map[string]any); ok {
					if success, ok := result["success"].(map[string]any); ok {
						if p, ok := success["path"].(string); ok && p != "" {
							path = p
						}
					}
				}

				written(o, dir, path)
				o.noted("writes " + path)
			case strings.HasPrefix(name, "shell"):
				if command, _ := args["command"].(string); command != "" {
					o.Commands = append(o.Commands, command)
					o.noted("runs " + command)
				}
			}
		}
	case "result":
		o.Said, _ = event["result"].(string)
		isError, _ := event["is_error"].(bool)
		o.OK = !isError && o.Failure == ""

		if isError && o.Failure == "" {
			o.Failure = classify(o.Said)
			o.Detail = o.Said
		}
	}
}
