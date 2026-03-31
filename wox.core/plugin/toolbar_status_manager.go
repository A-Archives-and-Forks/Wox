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

type toolbarStatusEntry struct {
	PluginId string
	Status   ToolbarStatus
	Sequence uint64 // Sequence preserves last-writer-wins ordering across plugin and global status updates.
}

type toolbarStatusStore struct {
	PluginToolbarStatuses *util.HashMap[string, *toolbarStatusEntry]
	GlobalToolbarStatuses *util.HashMap[string, *toolbarStatusEntry]
}

type sessionPluginQueryState struct {
	PluginId string
	QueryId  string
}

func newToolbarStatusStore() *toolbarStatusStore {
	return &toolbarStatusStore{
		PluginToolbarStatuses: util.NewHashMap[string, *toolbarStatusEntry](),
		GlobalToolbarStatuses: util.NewHashMap[string, *toolbarStatusEntry](),
	}
}

// ShowToolbarStatus stores the status and re-syncs the visible toolbar snapshot.
func (m *Manager) ShowToolbarStatus(ctx context.Context, pluginInstance *Instance, status ToolbarStatus) {
	if pluginInstance == nil || status.Id == "" {
		return
	}

	scope := status.Scope
	if scope == "" {
		scope = ToolbarStatusScopePlugin
	}

	if scope == ToolbarStatusScopePlugin {
		// Plugin-scoped status is only valid while this session is actively querying the plugin.
		sessionId := util.GetContextSessionId(ctx)
		if sessionId == "" || !m.isPluginActiveInSession(sessionId, pluginInstance.Metadata.Id) {
			logger.Warn(ctx, fmt.Sprintf("[%s] ignored toolbar status outside active plugin query", pluginInstance.GetName(ctx)))
			return
		}
	}

	entry := &toolbarStatusEntry{
		PluginId: pluginInstance.Metadata.Id,
		Status:   m.normalizeToolbarStatus(ctx, pluginInstance, status),
		Sequence: m.toolbarStatusSequence.Add(1),
	}

	store := m.getOrCreateToolbarStatusStore(pluginInstance.Metadata.Id)
	if entry.Status.Scope == ToolbarStatusScopeGlobal {
		store.GlobalToolbarStatuses.Store(entry.Status.Id, entry)
	} else {
		store.PluginToolbarStatuses.Store(entry.Status.Id, entry)
	}

	if entry.Status.Scope == ToolbarStatusScopePlugin {
		sessionId := util.GetContextSessionId(ctx)
		if sessionId != "" {
			m.syncVisibleToolbarStatusForSession(ctx, sessionId)
		}
		return
	}

	m.syncVisibleToolbarStatusForAllSessions(ctx)
}

// ClearToolbarStatus removes both plugin and global entries with the same Id for this plugin.
func (m *Manager) ClearToolbarStatus(ctx context.Context, pluginInstance *Instance, toolbarStatusId string) {
	if pluginInstance == nil || toolbarStatusId == "" {
		return
	}

	store, found := m.toolbarStatuses.Load(pluginInstance.Metadata.Id)
	if !found {
		return
	}

	store.PluginToolbarStatuses.Delete(toolbarStatusId)
	store.GlobalToolbarStatuses.Delete(toolbarStatusId)
	m.syncVisibleToolbarStatusForAllSessions(ctx)
}

// ExecuteToolbarStatusAction resolves the current status snapshot and invokes its action callback.
func (m *Manager) ExecuteToolbarStatusAction(ctx context.Context, sessionId string, toolbarStatusId string, actionId string) error {
	entry, found := m.findToolbarStatusEntry(toolbarStatusId)
	if !found {
		return fmt.Errorf("toolbar status not found: %s", toolbarStatusId)
	}

	actionIndex := slices.IndexFunc(entry.Status.Actions, func(action ToolbarStatusAction) bool {
		return action.Id == actionId
	})
	if actionIndex < 0 {
		return fmt.Errorf("toolbar status action not found: %s", actionId)
	}

	action := entry.Status.Actions[actionIndex]
	if action.Action == nil {
		return fmt.Errorf("toolbar status action callback missing: %s", actionId)
	}

	actionCtx := ToolbarStatusActionContext{
		ToolbarStatusId:       toolbarStatusId,
		ToolbarStatusActionId: actionId,
		ContextData:           common.ContextData(lo.Assign(map[string]string{}, action.ContextData)),
	}

	callbackCtx := ctx
	if sessionId != "" {
		callbackCtx = util.WithSessionContext(callbackCtx, sessionId)
	}
	action.Action(callbackCtx, actionCtx)
	return nil
}

// HandleQueryContext owns plugin query enter/leave lifecycle and the visibility of plugin-scoped status.
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
		m.syncVisibleToolbarStatusForSession(ctx, sessionId)
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
		m.clearPluginScopeToolbarStatuses(prevPluginId)
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

	m.syncVisibleToolbarStatusForSession(ctx, sessionId)
}

// HasVisibleToolbarStatus reports whether the current session, or any known session when no
// session is bound to ctx, currently has a visible toolbar status snapshot.
func (m *Manager) HasVisibleToolbarStatus(ctx context.Context) bool {
	sessionId := util.GetContextSessionId(ctx)
	if sessionId != "" {
		_, found := m.getVisibleToolbarStatusForSession(sessionId)
		return found
	}

	hasVisible := false
	for _, candidateSessionId := range m.listKnownSessions() {
		if _, found := m.getVisibleToolbarStatusForSession(candidateSessionId); found {
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

func (m *Manager) syncVisibleToolbarStatusForAllSessions(ctx context.Context) {
	for _, sessionId := range m.listKnownSessions() {
		m.syncVisibleToolbarStatusForSession(ctx, sessionId)
	}
}

// syncVisibleToolbarStatusForSession pushes the single visible toolbar status snapshot to UI.
func (m *Manager) syncVisibleToolbarStatusForSession(ctx context.Context, sessionId string) {
	if sessionId == "" {
		return
	}

	uiCtx := util.WithSessionContext(ctx, sessionId)
	status, found := m.getVisibleToolbarStatusForSession(sessionId)
	if !found {
		m.GetUI().ClearToolbarStatus(uiCtx)
		return
	}

	m.GetUI().ShowToolbarStatus(uiCtx, status.toToolbarStatusUI())
}

// getVisibleToolbarStatusForSession applies the visibility priority:
// active plugin-scoped status first, latest global status second.
func (m *Manager) getVisibleToolbarStatusForSession(sessionId string) (*toolbarStatusEntry, bool) {
	if state, found := m.sessionPluginQueries.Load(sessionId); found && state.PluginId != "" {
		if entry, ok := m.getLatestPluginScopedToolbarStatus(state.PluginId); ok {
			return entry, true
		}
	}
	return m.getLatestGlobalToolbarStatus()
}

func (m *Manager) getLatestPluginScopedToolbarStatus(pluginId string) (*toolbarStatusEntry, bool) {
	store, found := m.toolbarStatuses.Load(pluginId)
	if !found {
		return nil, false
	}

	var latest *toolbarStatusEntry
	store.PluginToolbarStatuses.Range(func(_ string, entry *toolbarStatusEntry) bool {
		if latest == nil || entry.Sequence > latest.Sequence {
			latest = entry
		}
		return true
	})
	return latest, latest != nil
}

func (m *Manager) getLatestGlobalToolbarStatus() (*toolbarStatusEntry, bool) {
	var latest *toolbarStatusEntry
	m.toolbarStatuses.Range(func(_ string, store *toolbarStatusStore) bool {
		store.GlobalToolbarStatuses.Range(func(_ string, entry *toolbarStatusEntry) bool {
			if latest == nil || entry.Sequence > latest.Sequence {
				latest = entry
			}
			return true
		})
		return true
	})
	return latest, latest != nil
}

func (m *Manager) getOrCreateToolbarStatusStore(pluginId string) *toolbarStatusStore {
	if store, found := m.toolbarStatuses.Load(pluginId); found {
		return store
	}

	store := newToolbarStatusStore()
	m.toolbarStatuses.Store(pluginId, store)
	return store
}

func (m *Manager) clearPluginScopeToolbarStatuses(pluginId string) {
	store, found := m.toolbarStatuses.Load(pluginId)
	if !found {
		return
	}
	store.PluginToolbarStatuses = util.NewHashMap[string, *toolbarStatusEntry]()
}

func (m *Manager) isPluginActiveInSession(sessionId string, pluginId string) bool {
	state, found := m.sessionPluginQueries.Load(sessionId)
	return found && state.PluginId == pluginId
}

func (m *Manager) findToolbarStatusEntry(toolbarStatusId string) (*toolbarStatusEntry, bool) {
	var foundEntry *toolbarStatusEntry
	m.toolbarStatuses.Range(func(_ string, store *toolbarStatusStore) bool {
		if entry, found := store.PluginToolbarStatuses.Load(toolbarStatusId); found {
			foundEntry = entry
			return false
		}
		if entry, found := store.GlobalToolbarStatuses.Load(toolbarStatusId); found {
			foundEntry = entry
			return false
		}
		return true
	})
	return foundEntry, foundEntry != nil
}

// normalizeToolbarStatus translates user-facing text, clones context data, and backfills
// host proxies for external plugin action callbacks.
func (m *Manager) normalizeToolbarStatus(ctx context.Context, pluginInstance *Instance, status ToolbarStatus) ToolbarStatus {
	normalized := ToolbarStatus{
		Id:            status.Id,
		Scope:         status.Scope,
		Title:         pluginInstance.translateMetadataText(ctx, common.I18nString(status.Title)),
		Icon:          status.Icon,
		Progress:      status.Progress,
		Indeterminate: status.Indeterminate,
		Actions:       make([]ToolbarStatusAction, 0, len(status.Actions)),
	}

	for _, action := range status.Actions {
		if action.Id == "" {
			action.Id = uuid.NewString()
		}
		action.Name = pluginInstance.translateMetadataText(ctx, common.I18nString(action.Name))
		action.ContextData = common.ContextData(lo.Assign(map[string]string{}, action.ContextData))

		if action.Action == nil {
			if proxyCreator, ok := pluginInstance.Plugin.(ToolbarStatusActionProxyCreator); ok {
				action.Action = proxyCreator.CreateToolbarStatusActionProxy(action.Id)
			}
		}

		normalized.Actions = append(normalized.Actions, action)
	}

	return normalized
}

// toToolbarStatusUI strips callbacks and returns a UI-safe snapshot.
func (s *toolbarStatusEntry) toToolbarStatusUI() ToolbarStatusUI {
	uiStatus := ToolbarStatusUI{
		Id:            s.Status.Id,
		Title:         s.Status.Title,
		Icon:          s.Status.Icon,
		Progress:      s.Status.Progress,
		Indeterminate: s.Status.Indeterminate,
		Actions:       make([]ToolbarStatusActionUI, 0, len(s.Status.Actions)),
	}
	for _, action := range s.Status.Actions {
		uiStatus.Actions = append(uiStatus.Actions, ToolbarStatusActionUI{
			Id:                     action.Id,
			Name:                   action.Name,
			Icon:                   action.Icon,
			Hotkey:                 action.Hotkey,
			IsDefault:              action.IsDefault,
			PreventHideAfterAction: action.PreventHideAfterAction,
			ContextData:            common.ContextData(lo.Assign(map[string]string{}, action.ContextData)),
		})
	}
	return uiStatus
}
