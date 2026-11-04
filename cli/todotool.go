package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"deekseep-coder/internal/deepseek"
	"deekseep-coder/internal/todo"
)

// toolTodo is the name the model uses to publish its task list.
const toolTodo = "todo_write"

// todoTool describes the task list tool to the model.
//
// The description does most of the work here: it is what tells the model that
// keeping the list honest is part of the job and not decoration.
func (a *app) todoTool() deepseek.Tool {
	var params map[string]any
	_ = json.Unmarshal([]byte(`{
		"type": "object",
		"properties": {
			"todos": {
				"type": "array",
				"description": "The full list, in order. Replaces the previous one.",
				"items": {
					"type": "object",
					"properties": {
						"content": {"type": "string", "description": "What has to be done, in one line."},
						"status": {
							"type": "string",
							"enum": ["pending", "in_progress", "completed"],
							"description": "pending, in_progress or completed."
						}
					},
					"required": ["content", "status"]
				}
			}
		},
		"required": ["todos"]
	}`), &params)

	return deepseek.Tool{
		Type: "function",
		Function: deepseek.ToolFunction{
			Name:        toolTodo,
			Description: "Publish the task list for this session. Use it whenever the work takes more than one step. Replaces the whole list on every call. Keep exactly one task in_progress, mark a task completed only when it is truly finished, and keep it updated as you go: the user sees it while you work.",
			Parameters:  params,
		},
	}
}

// executeTodoTool stores a new task list.
func (a *app) executeTodoTool(tc deepseek.ToolCall) (string, bool, error) {
	if tc.Function.Name != toolTodo {
		return "", false, nil
	}
	var args struct {
		Todos []todo.Item `json:"todos"`
	}
	if err := decodeArgs(tc.Function.Arguments, &args); err != nil {
		return "", true, err
	}
	// An empty list is a valid way to clear it.
	if args.Todos == nil {
		args.Todos = []todo.Item{}
	}
	if err := todo.List(args.Todos).Validate(); err != nil {
		return "", true, err
	}
	if a.session != nil {
		if err := a.session.AppendTodos(args.Todos); err != nil {
			return "", true, err
		}
	}
	list := todo.List(args.Todos)
	if len(list) == 0 {
		return "Task list cleared.", true, nil
	}
	return fmt.Sprintf("Task list updated (%s):\n%s", list.Progress(), list.Render()), true, nil
}

// todoPreview shows a task list in a permission prompt or a log line.
func todoPreview(rawArgs string) (string, bool) {
	var args struct {
		Todos []todo.Item `json:"todos"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return "", false
	}
	if len(args.Todos) == 0 {
		return "tareas: (lista vacía)", true
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s\n", todo.List(args.Todos).Progress()))
	b.WriteString(todo.List(args.Todos).Render())
	return b.String(), true
}

// todosString renders the current list for the /todos command.
func (a *app) todosString() string {
	if a.session == nil {
		return a.msg.NoTodos
	}
	list := a.session.Todos()
	if len(list) == 0 {
		return a.msg.NoTodos
	}
	return list.Progress() + "\n" + list.Render()
}
