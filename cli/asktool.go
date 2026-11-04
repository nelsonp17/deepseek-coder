package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"deekseep-coder/internal/deepseek"
)

// toolQuestion is the tool the model uses to ask a structured question.
const toolQuestion = "ask_user_question"

// questionOption is one choice offered to the user.
type questionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// questionArgs is the tool payload.
type questionArgs struct {
	Question string           `json:"question"`
	Options  []questionOption `json:"options"`
	Multi    bool             `json:"multi"`
}

// questionTool describes ask_user_question.
func (a *app) questionTool() deepseek.Tool {
	var params map[string]any
	_ = json.Unmarshal([]byte(`{
		"type": "object",
		"properties": {
			"question": {
				"type": "string",
				"description": "The question, in one line."
			},
			"options": {
				"type": "array",
				"description": "Two to six concrete choices. Do not add an 'other' option: the user can always type their own answer.",
				"items": {
					"type": "object",
					"properties": {
						"label": {"type": "string", "description": "Short label."},
						"description": {"type": "string", "description": "One line saying what this choice implies."}
					},
					"required": ["label"]
				}
			},
			"multi": {
				"type": "boolean",
				"description": "Allow more than one choice."
			}
		},
		"required": ["question", "options"]
	}`), &params)

	return deepseek.Tool{
		Type: "function",
		Function: deepseek.ToolFunction{
			Name:        toolQuestion,
			Description: "Ask the user a question with concrete options when a decision blocks you. Use it instead of guessing. Do not use it for confirmation of a tool call: that is already handled.",
			Parameters:  params,
		},
	}
}

// executeQuestionTool hands the question to the caller.
//
// The answer travels through a.askUser, which is set by the TUI. Without one
// (line mode, one-shot ask) the model is told to ask in plain text instead:
// reading stdin while the REPL owns it would steal its input.
func (a *app) executeQuestionTool(tc deepseek.ToolCall) (string, bool, error) {
	if tc.Function.Name != toolQuestion {
		return "", false, nil
	}
	var args questionArgs
	if err := decodeArgs(tc.Function.Arguments, &args); err != nil {
		return "", true, err
	}
	if strings.TrimSpace(args.Question) == "" {
		return "", true, errors.New("the question cannot be empty")
	}
	if len(args.Options) == 0 {
		return "", true, errors.New("at least one option is required")
	}
	if len(args.Options) > 6 {
		return "", true, errors.New("at most 6 options are allowed")
	}
	if a.askUser == nil {
		return "The user cannot answer an interactive question in this mode. Ask your question in plain text and finish the turn.", true, nil
	}
	labels := make([]string, 0, len(args.Options))
	for _, opt := range args.Options {
		labels = append(labels, opt.Label)
	}
	chosen, err := a.askUser(args.Question, labels, args.Multi)
	if err != nil {
		return "", true, err
	}
	if len(chosen) == 0 {
		return "The user dismissed the question without answering.", true, nil
	}
	return fmt.Sprintf("The user answered: %s", strings.Join(chosen, "; ")), true, nil
}
