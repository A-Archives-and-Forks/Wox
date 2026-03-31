package plugin

import (
	"context"
	"wox/common"
)

type ToolbarStatusScope string

const (
	// ToolbarStatusScopePlugin keeps the status visible only while the user stays
	// inside the owning plugin query context.
	ToolbarStatusScopePlugin ToolbarStatusScope = "plugin"

	// ToolbarStatusScopeGlobal allows the status to be shown outside plugin query
	// context and compete with other global status entries.
	ToolbarStatusScopeGlobal ToolbarStatusScope = "global"
)

// ToolbarStatus is the payload pushed to the launcher toolbar.
type ToolbarStatus struct {
	Id            string
	Scope         ToolbarStatusScope
	Title         string
	Icon          common.WoxImage
	Progress      *int // Progress is a 0-100 value when the work has a measurable percentage.
	Indeterminate bool // Indeterminate shows a spinner without percentage when progress cannot be measured yet.
	Actions       []ToolbarStatusAction
}

// ToolbarStatusAction describes one action rendered on the toolbar while the
// status is visible.
type ToolbarStatusAction struct {
	Id                     string
	Name                   string
	Icon                   common.WoxImage
	Hotkey                 string
	IsDefault              bool
	PreventHideAfterAction bool
	ContextData            common.ContextData                                                  // ContextData is round-tripped back to the action callback.
	Action                 func(ctx context.Context, actionContext ToolbarStatusActionContext) `json:"-"`
}

// ToolbarStatusActionContext identifies the toolbar status action invocation.
type ToolbarStatusActionContext struct {
	ToolbarStatusId       string
	ToolbarStatusActionId string
	ContextData           common.ContextData
}

// ToolbarStatusActionUI is the UI-safe action snapshot sent to Flutter.
type ToolbarStatusActionUI struct {
	Id                     string
	Name                   string
	Icon                   common.WoxImage
	Hotkey                 string
	IsDefault              bool
	PreventHideAfterAction bool
	ContextData            common.ContextData
}

// ToolbarStatusUI is the UI-safe status snapshot sent to Flutter.
type ToolbarStatusUI struct {
	Id            string
	Title         string
	Icon          common.WoxImage
	Progress      *int
	Indeterminate bool
	Actions       []ToolbarStatusActionUI
}
