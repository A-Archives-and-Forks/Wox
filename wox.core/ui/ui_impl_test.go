package ui

import (
	"context"
	"reflect"
	"testing"
	"unsafe"
	"wox/common"
	"wox/plugin"
	"wox/util"
)

type testUIPlugin struct{}

func (p *testUIPlugin) Init(ctx context.Context, initParams plugin.InitParams) {}
func (p *testUIPlugin) Query(ctx context.Context, query plugin.Query) []plugin.QueryResult {
	return nil
}

type testToolbarStateUI struct {
	clearToolbarMsgCalls int
	showToolbarMsgCalls  int
}

func (u *testToolbarStateUI) ChangeQuery(ctx context.Context, query common.PlainQuery)      {}
func (u *testToolbarStateUI) RefreshQuery(ctx context.Context, preserveSelectedIndex bool)  {}
func (u *testToolbarStateUI) HideApp(ctx context.Context)                                   {}
func (u *testToolbarStateUI) ShowApp(ctx context.Context, showContext common.ShowContext)   {}
func (u *testToolbarStateUI) ToggleApp(ctx context.Context, showContext common.ShowContext) {}
func (u *testToolbarStateUI) OpenSettingWindow(ctx context.Context, windowContext common.SettingWindowContext) {
}
func (u *testToolbarStateUI) PickFiles(ctx context.Context, params common.PickFilesParams) []string {
	return nil
}
func (u *testToolbarStateUI) GetActiveWindowSnapshot(ctx context.Context) common.ActiveWindowSnapshot {
	return common.ActiveWindowSnapshot{}
}
func (u *testToolbarStateUI) GetServerPort(ctx context.Context) int                  { return 0 }
func (u *testToolbarStateUI) GetAllThemes(ctx context.Context) []common.Theme        { return nil }
func (u *testToolbarStateUI) ChangeTheme(ctx context.Context, theme common.Theme)    {}
func (u *testToolbarStateUI) InstallTheme(ctx context.Context, theme common.Theme)   {}
func (u *testToolbarStateUI) UninstallTheme(ctx context.Context, theme common.Theme) {}
func (u *testToolbarStateUI) RestoreTheme(ctx context.Context)                       {}
func (u *testToolbarStateUI) Notify(ctx context.Context, msg common.NotifyMsg)       {}
func (u *testToolbarStateUI) ShowToolbarMsg(ctx context.Context, msg interface{}) {
	u.showToolbarMsgCalls++
}
func (u *testToolbarStateUI) ClearToolbarMsg(ctx context.Context)                       { u.clearToolbarMsgCalls++ }
func (u *testToolbarStateUI) UpdateResult(ctx context.Context, result interface{}) bool { return true }
func (u *testToolbarStateUI) PushResults(ctx context.Context, payload interface{}) bool { return true }
func (u *testToolbarStateUI) IsVisible(ctx context.Context) bool                        { return true }
func (u *testToolbarStateUI) FocusToChatInput(ctx context.Context)                      {}
func (u *testToolbarStateUI) SendChatResponse(ctx context.Context, chatData common.AIChatData) {
}
func (u *testToolbarStateUI) ReloadChatResources(ctx context.Context, resouceName string) {}
func (u *testToolbarStateUI) ReloadSettingPlugins(ctx context.Context)                    {}
func (u *testToolbarStateUI) ReloadSetting(ctx context.Context)                           {}

func setUnexportedField(target any, fieldName string, value any) any {
	targetValue := reflect.ValueOf(target).Elem()
	fieldValue := targetValue.FieldByName(fieldName)
	originalValue := reflect.NewAt(fieldValue.Type(), unsafe.Pointer(fieldValue.UnsafeAddr())).Elem().Interface()
	dest := reflect.NewAt(fieldValue.Type(), unsafe.Pointer(fieldValue.UnsafeAddr())).Elem()
	if value == nil {
		dest.Set(reflect.Zero(fieldValue.Type()))
	} else {
		dest.Set(reflect.ValueOf(value))
	}
	return originalValue
}

func clearUnexportedHashMap(target any, fieldName string) {
	targetValue := reflect.ValueOf(target).Elem()
	fieldValue := targetValue.FieldByName(fieldName)
	reflect.NewAt(fieldValue.Type(), unsafe.Pointer(fieldValue.UnsafeAddr())).Elem().MethodByName("Clear").Call(nil)
}

func storeSessionPluginQueryState(manager any, sessionId string, pluginId string, queryId string) {
	targetValue := reflect.ValueOf(manager).Elem()
	fieldValue := targetValue.FieldByName("sessionPluginQueries")
	storeMethod := reflect.NewAt(fieldValue.Type(), unsafe.Pointer(fieldValue.UnsafeAddr())).Elem().MethodByName("Store")
	stateType := storeMethod.Type().In(1)
	stateValue := reflect.New(stateType.Elem())
	stateValue.Elem().FieldByName("PluginId").SetString(pluginId)
	stateValue.Elem().FieldByName("QueryId").SetString(queryId)
	storeMethod.Call([]reflect.Value{reflect.ValueOf(sessionId), stateValue})
}

func TestHandleWebsocketQueryClearsPluginToolbarMsgWhenQueryBecomesEmpty(t *testing.T) {
	GetUIManager()
	manager := plugin.GetPluginManager()
	fakeUI := &testToolbarStateUI{}
	fakePluginInstance := &plugin.Instance{
		Plugin: &testUIPlugin{},
		Metadata: plugin.Metadata{
			Id:              "test-plugin",
			Name:            "Test Plugin",
			TriggerKeywords: []string{"f"},
		},
	}

	originalUI := setUnexportedField(manager, "ui", common.UI(fakeUI))
	originalInstances := setUnexportedField(manager, "instances", []*plugin.Instance{fakePluginInstance})
	clearUnexportedHashMap(manager, "toolbarMsgs")
	clearUnexportedHashMap(manager, "sessionPluginQueries")
	defer func() {
		setUnexportedField(manager, "ui", originalUI)
		setUnexportedField(manager, "instances", originalInstances)
		clearUnexportedHashMap(manager, "toolbarMsgs")
		clearUnexportedHashMap(manager, "sessionPluginQueries")
	}()

	ctx := util.WithSessionContext(util.NewTraceContext(), "session-1")
	selectionJSON := "{}"
	storeSessionPluginQueryState(manager, "session-1", fakePluginInstance.Metadata.Id, "query-1")

	handleWebsocketQuery(ctx, WebsocketMsg{
		RequestId: "request-2",
		SessionId: "session-1",
		Method:    "Query",
		Data: map[string]any{
			"queryId":        "query-2",
			"queryType":      plugin.QueryTypeInput,
			"queryText":      "",
			"querySelection": selectionJSON,
		},
	})

	if fakeUI.clearToolbarMsgCalls == 0 {
		t.Fatalf("expected empty query to clear plugin-scoped toolbar msg")
	}
}

var _ common.UI = (*testToolbarStateUI)(nil)
var _ plugin.Plugin = (*testUIPlugin)(nil)
