package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"deepseek-coder/internal/deepseek"
)

// toolPlan is the tool the model calls to leave plan mode.
const toolPlan = "exit_plan_mode"

// planHint is added to the system prompt while plan mode is on.
const planHint = `PLAN MODE is active. You may read anything (read_file, glob, grep) but you cannot write files or run commands: they are disabled until the plan is approved. Investigate what you need, then write a concrete plan — the files you will touch, the change in each, the order, and how you will verify it — and call exit_plan_mode with it. If the user rejects the plan, revise it and call exit_plan_mode again.`

// planTool describes exit_plan_mode.
func (a *app) planTool() deepseek.Tool {
	var params map[string]any
	_ = json.Unmarshal([]byte(`{
		"type": "object",
		"properties": {
			"plan": {
				"type": "string",
				"description": "The plan, as Markdown: what changes, in which files, in what order, and how it will be verified."
			}
		},
		"required": ["plan"]
	}`), &params)

	return deepseek.Tool{
		Type: "function",
		Function: deepseek.ToolFunction{
			Name:        toolPlan,
			Description: "Present the plan for approval and leave plan mode. Writes and commands stay disabled until the user approves.",
			Parameters:  params,
		},
	}
}

// executePlanTool handles exit_plan_mode.
//
// The tool executes only after the approval prompt said yes, so reaching this
// point means the plan was accepted: it just turns plan mode off.
func (a *app) executePlanTool(tc deepseek.ToolCall) (string, bool, error) {
	if tc.Function.Name != toolPlan {
		return "", false, nil
	}
	var args struct {
		Plan string `json:"plan"`
	}
	if err := decodeArgs(tc.Function.Arguments, &args); err != nil {
		return "", true, err
	}
	if strings.TrimSpace(args.Plan) == "" {
		return "", true, fmt.Errorf("the plan cannot be empty")
	}
	a.planMode = false
	return "Plan approved. Plan mode is off: you may now write files and run commands. Follow the plan, and tell the user when you deviate from it.", true, nil
}

// planPreview renders the plan in the permission prompt, where the user decides.
func planPreview(rawArgs string) (string, bool) {
	var args struct {
		Plan string `json:"plan"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return "", false
	}
	if strings.TrimSpace(args.Plan) == "" {
		return "", false
	}
	return "PLAN PROPUESTO\n\n" + args.Plan, true
}

// planString reports whether plan mode is on.
func (a *app) planString() string {
	if a.planMode {
		return a.msg.PlanOn
	}
	return a.msg.PlanOff
}
