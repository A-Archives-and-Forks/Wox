package plugin

import (
	"context"
	"fmt"
	"slices"

	"wox/common"
	"wox/util"

	"github.com/google/uuid"
	"github.com/samber/lo"
)

type toolbarMsgEntry struct {
	PluginId string
	Msg      ToolbarMsg
	Sequence uint64 // Sequence preserves last-writer-wins ordering across plugin and global toolbar msg updates.
}

type toolbarMsgStore struct {
	PluginToolbarMsgs *util.HashMap[string, *toolbarMsgEntry]
	GlobalToolbarMsgs *util.HashMap[string, *toolbarMsgEntry]
}

type sessionPluginQueryState struct {
	PluginId string
	QueryId  string
}

func newToolbarMsgStore() *toolbarMsgStore {
	return &toolbarMsgStore{
		PluginToolbarMsgs: util.NewHashMap[string, *toolbarMsgEntry](),
		GlobalToolbarMsgs: util.NewHashMap[string, *toolbarMsgEntry](),
	}
}

// ShowToolbarMsg stores the toolbar msg and re-syncs the visible toolbar snapshot.
func (m *Manager) ShowToolbarMsg(ctx context.Context, pluginInstance *Instance, msg ToolbarMsg) {
	if pluginInstance == nil || msg.Id == "" {
		return
	}

	scope := msg.Scope
	if scope == "" {
		scope = ToolbarMsgScopePlugin
	}

	if scope == ToolbarMsgScopePlugin {
		// Plugin-scoped toolbar msg is only valid while this session is actively querying the plugin.
		sessionId := util.GetContextSessionId(ctx)
		if sessionId == "" || !m.isPluginActiveInSession(sessionId, pluginInstance.Metadata.Id) {
			logger.Warn(ctx, fmt.Sprintf("[%s] ignored toolbar msg outside active plugin query", pluginInstance.GetName(ctx)))
			return
		}
	}

	entry := &toolbarMsgEntry{
		PluginId: pluginInstance.Metadata.Id,
		Msg:      m.normalizeToolbarMsg(ctx, pluginInstance, msg),
		Sequence: m.toolbarMsgSequence.Add(1),
	}

	store := m.getOrCreateToolbarMsgStore(pluginInstance.Metadata.Id)
	if entry.Msg.Scope == ToolbarMsgScopeGlobal {
		store.GlobalToolbarMsgs.Store(entry.Msg.Id, entry)
	} else {
		store.PluginToolbarMsgs.Store(entry.Msg.Id, entry)
	}

	if entry.Msg.Scope == ToolbarMsgScopePlugin {
		sessionId := util.GetContextSessionId(ctx)
		if sessionId != "" {
			m.syncVisibleToolbarMsgForSession(ctx, sessionId)
		}
		return
	}

	m.syncVisibleToolbarMsgForAllSessions(ctx)
}

// ClearToolbarMsg removes both plugin-scoped and global toolbar msgs with the same Id for this plugin.
func (m *Manager) ClearToolbarMsg(ctx context.Context, pluginInstance *Instance, toolbarMsgId string) {
	if pluginInstance == nil || toolbarMsgId == "" {
		return
	}

	store, found := m.toolbarMsgs.Load(pluginInstance.Metadata.Id)
	if !found {
		return
	}

	store.PluginToolbarMsgs.Delete(toolbarMsgId)
	store.GlobalToolbarMsgs.Delete(toolbarMsgId)
	m.syncVisibleToolbarMsgForAllSessions(ctx)
}

// ExecuteToolbarMsgAction resolves the current toolbar msg snapshot and invokes its action callback.
func (m *Manager) ExecuteToolbarMsgAction(ctx context.Context, sessionId string, toolbarMsgId string, actionId string) error {
	entry, found := m.findToolbarMsgEntry(toolbarMsgId)
	if !found {
		return fmt.Errorf("toolbar msg not found: %s", toolbarMsgId)
	}

	actionIndex := slices.IndexFunc(entry.Msg.Actions, func(action ToolbarMsgAction) bool {
		return action.Id == actionId
	})
	if actionIndex < 0 {
		return fmt.Errorf("toolbar msg action not found: %s", actionId)
	}

	action := entry.Msg.Actions[actionIndex]
	if action.Action == nil {
		return fmt.Errorf("toolbar msg action callback missing: %s", actionId)
	}

	actionCtx := ToolbarMsgActionContext{
		ToolbarMsgId:       toolbarMsgId,
		ToolbarMsgActionId: actionId,
		ContextData:        common.ContextData(lo.Assign(map[string]string{}, action.ContextData)),
	}

	callbackCtx := ctx
	if sessionId != "" {
		callbackCtx = util.WithSessionContext(callbackCtx, sessionId)
	}
	action.Action(callbackCtx, actionCtx)
	return nil
}

// HandleQueryContext owns plugin query enter/leave lifecycle and the visibility of plugin-scoped toolbar messages.
func (m *Manager) HandleQueryContext(ctx context.Context, query Query, pluginInstance *Instance) {
	sessionId := query.SessionId
	if sessionId == "" {
		return
	}

	nextPluginId := ""
	if pluginInstance != nil && query.Type == QueryTypeInput && query.TriggerKeyword != "" {
		nextPluginId = pluginInstance.Metadata.Id
	}

	prevState, hasPrev := m.sessionPluginQueries.Load(sessionId)
	prevPluginId := ""
	prevQueryId := ""
	if hasPrev {
		prevPluginId = prevState.PluginId
		prevQueryId = prevState.QueryId
	}

	if prevPluginId == nextPluginId {
		if nextPluginId == "" {
			m.sessionPluginQueries.Delete(sessionId)
		} else {
			m.sessionPluginQueries.Store(sessionId, &sessionPluginQueryState{PluginId: nextPluginId, QueryId: query.Id})
		}
		m.syncVisibleToolbarMsgForSession(ctx, sessionId)
		return
	}

	if prevPluginId != "" {
		if prevInstance := m.getPluginInstance(prevPluginId); prevInstance != nil {
			leaveCtx := util.WithQueryIdContext(util.WithSessionContext(ctx, sessionId), prevQueryId)
			for _, callback := range prevInstance.LeavePluginQueryCallbacks {
				util.Go(leaveCtx, fmt.Sprintf("[%s] leave plugin query callback", prevInstance.GetName(leaveCtx)), func() {
					callback(leaveCtx)
				})
			}
		}
		m.clearPluginScopeToolbarMsgs(prevPluginId)
	}

	if nextPluginId == "" {
		m.sessionPluginQueries.Delete(sessionId)
	} else {
		m.sessionPluginQueries.Store(sessionId, &sessionPluginQueryState{PluginId: nextPluginId, QueryId: query.Id})
		if nextInstance := m.getPluginInstance(nextPluginId); nextInstance != nil {
			enterCtx := util.WithQueryIdContext(util.WithSessionContext(ctx, sessionId), query.Id)
			for _, callback := range nextInstance.EnterPluginQueryCallbacks {
				util.Go(enterCtx, fmt.Sprintf("[%s] enter plugin query callback", nextInstance.GetName(enterCtx)), func() {
					callback(enterCtx)
				})
			}
		}
	}

	m.syncVisibleToolbarMsgForSession(ctx, sessionId)
}

// HasVisibleToolbarMsg reports whether the current session, or any known session when no
// session is bound to ctx, currently has a visible toolbar msg snapshot.
func (m *Manager) HasVisibleToolbarMsg(ctx context.Context) bool {
	sessionId := util.GetContextSessionId(ctx)
	if sessionId != "" {
		_, found := m.getVisibleToolbarMsgForSession(sessionId)
		return found
	}

	hasVisible := false
	for _, candidateSessionId := range m.listKnownSessions() {
		if _, found := m.getVisibleToolbarMsgForSession(candidateSessionId); found {
			hasVisible = true
			break
		}
	}
	return hasVisible
}

func (m *Manager) listKnownSessions() []string {
	sessionIds := map[string]struct{}{}
	m.sessionPluginQueries.Range(func(sessionId string, _ *sessionPluginQueryState) bool {
		sessionIds[sessionId] = struct{}{}
		return true
	})
	m.sessionQueryResultCache.Range(func(sessionId string, _ *QueryResultSet) bool {
		sessionIds[sessionId] = struct{}{}
		return true
	})
	return lo.Keys(sessionIds)
}

func (m *Manager) syncVisibleToolbarMsgForAllSessions(ctx context.Context) {
	for _, sessionId := range m.listKnownSessions() {
		m.syncVisibleToolbarMsgForSession(ctx, sessionId)
	}
}

// syncVisibleToolbarMsgForSession pushes the single visible toolbar msg snapshot to UI.
func (m *Manager) syncVisibleToolbarMsgForSession(ctx context.Context, sessionId string) {
	if sessionId == "" {
		return
	}

	uiCtx := util.WithSessionContext(ctx, sessionId)
	msg, found := m.getVisibleToolbarMsgForSession(sessionId)
	if !found {
		m.GetUI().ClearToolbarMsg(uiCtx)
		return
	}

	m.GetUI().ShowToolbarMsg(uiCtx, msg.toToolbarMsgUI())
}

// getVisibleToolbarMsgForSession applies the visibility priority:
// active plugin-scoped toolbar msg first, latest global toolbar msg second.
func (m *Manager) getVisibleToolbarMsgForSession(sessionId string) (*toolbarMsgEntry, bool) {
	if state, found := m.sessionPluginQueries.Load(sessionId); found && state.PluginId != "" {
		if entry, ok := m.getLatestPluginScopedToolbarMsg(state.PluginId); ok {
			return entry, true
		}
	}
	return m.getLatestGlobalToolbarMsg()
}

func (m *Manager) getLatestPluginScopedToolbarMsg(pluginId string) (*toolbarMsgEntry, bool) {
	store, found := m.toolbarMsgs.Load(pluginId)
	if !found {
		return nil, false
	}

	var latest *toolbarMsgEntry
	store.PluginToolbarMsgs.Range(func(_ string, entry *toolbarMsgEntry) bool {
		if latest == nil || entry.Sequence > latest.Sequence {
			latest = entry
		}
		return true
	})
	return latest, latest != nil
}

func (m *Manager) getLatestGlobalToolbarMsg() (*toolbarMsgEntry, bool) {
	var latest *toolbarMsgEntry
	m.toolbarMsgs.Range(func(_ string, store *toolbarMsgStore) bool {
		store.GlobalToolbarMsgs.Range(func(_ string, entry *toolbarMsgEntry) bool {
			if latest == nil || entry.Sequence > latest.Sequence {
				latest = entry
			}
			return true
		})
		return true
	})
	return latest, latest != nil
}

func (m *Manager) getOrCreateToolbarMsgStore(pluginId string) *toolbarMsgStore {
	if store, found := m.toolbarMsgs.Load(pluginId); found {
		return store
	}

	store := newToolbarMsgStore()
	m.toolbarMsgs.Store(pluginId, store)
	return store
}

func (m *Manager) clearPluginScopeToolbarMsgs(pluginId string) {
	store, found := m.toolbarMsgs.Load(pluginId)
	if !found {
		return
	}
	store.PluginToolbarMsgs = util.NewHashMap[string, *toolbarMsgEntry]()
}

func (m *Manager) isPluginActiveInSession(sessionId string, pluginId string) bool {
	state, found := m.sessionPluginQueries.Load(sessionId)
	return found && state.PluginId == pluginId
}

func (m *Manager) findToolbarMsgEntry(toolbarMsgId string) (*toolbarMsgEntry, bool) {
	var foundEntry *toolbarMsgEntry
	m.toolbarMsgs.Range(func(_ string, store *toolbarMsgStore) bool {
		if entry, found := store.PluginToolbarMsgs.Load(toolbarMsgId); found {
			foundEntry = entry
			return false
		}
		if entry, found := store.GlobalToolbarMsgs.Load(toolbarMsgId); found {
			foundEntry = entry
			return false
		}
		return true
	})
	return foundEntry, foundEntry != nil
}

// normalizeToolbarMsg translates user-facing text, clones context data, and backfills
// host proxies for external plugin action callbacks.
func (m *Manager) normalizeToolbarMsg(ctx context.Context, pluginInstance *Instance, msg ToolbarMsg) ToolbarMsg {
	normalized := ToolbarMsg{
		Id:            msg.Id,
		Scope:         msg.Scope,
		Title:         pluginInstance.translateMetadataText(ctx, common.I18nString(msg.Title)),
		Icon:          msg.Icon,
		Progress:      msg.Progress,
		Indeterminate: msg.Indeterminate,
		Actions:       make([]ToolbarMsgAction, 0, len(msg.Actions)),
	}

	for _, action := range msg.Actions {
		if action.Id == "" {
			action.Id = uuid.NewString()
		}
		action.Name = pluginInstance.translateMetadataText(ctx, common.I18nString(action.Name))
		action.ContextData = common.ContextData(lo.Assign(map[string]string{}, action.ContextData))

		if action.Action == nil {
			if proxyCreator, ok := pluginInstance.Plugin.(ToolbarMsgActionProxyCreator); ok {
				action.Action = proxyCreator.CreateToolbarMsgActionProxy(action.Id)
			}
		}

		normalized.Actions = append(normalized.Actions, action)
	}

	return normalized
}

// toToolbarMsgUI strips callbacks and returns a UI-safe snapshot.
func (s *toolbarMsgEntry) toToolbarMsgUI() ToolbarMsgUI {
	uiMsg := ToolbarMsgUI{
		Id:            s.Msg.Id,
		Title:         s.Msg.Title,
		Icon:          s.Msg.Icon,
		Progress:      s.Msg.Progress,
		Indeterminate: s.Msg.Indeterminate,
		Actions:       make([]ToolbarMsgActionUI, 0, len(s.Msg.Actions)),
	}
	for _, action := range s.Msg.Actions {
		uiMsg.Actions = append(uiMsg.Actions, ToolbarMsgActionUI{
			Id:                     action.Id,
			Name:                   action.Name,
			Icon:                   action.Icon,
			Hotkey:                 action.Hotkey,
			IsDefault:              action.IsDefault,
			PreventHideAfterAction: action.PreventHideAfterAction,
			ContextData:            common.ContextData(lo.Assign(map[string]string{}, action.ContextData)),
		})
	}
	return uiMsg
}
