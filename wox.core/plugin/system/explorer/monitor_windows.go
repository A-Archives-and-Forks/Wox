package explorer

/*
extern void fileExplorerActivatedCallbackCGO(int pid, int isFileDialog, int x, int y, int w, int h);
extern void fileExplorerDeactivatedCallbackCGO();
extern void fileExplorerLogCallbackCGO(char* msg);
int refreshFileExplorerMonitorState();
int isForegroundExplorerFileListFocused();
void startFileExplorerMonitor();
void stopFileExplorerMonitor();
*/
import "C"

import (
	"fmt"
	"strings"
	"sync"
	"wox/util/keyboard"
)

var (
	explorerActivatedCallback   func(pid int)
	explorerDeactivatedCallback func()
	dialogActivatedCallback     func(pid int)
	dialogDeactivatedCallback   func()
	explorerKeyListener         func(key string)
	dialogKeyListener           func(key string)

	// Internal state to track explorer window
	explorerActive bool
	explorerRectX  int
	explorerRectY  int
	explorerRectW  int
	explorerRectH  int

	// Internal state to track dialog window
	dialogActive bool
	dialogRectX  int
	dialogRectY  int
	dialogRectW  int
	dialogRectH  int

	rawKeySubscription keyboard.RawKeySubscription
)

// stateMu protects Explorer/dialog state shared by WinEvent callbacks and the
// raw-key listener path.
var stateMu sync.RWMutex

type monitorState int

const (
	stateNone monitorState = iota
	stateExplorer
	stateDialog
)

var currentState monitorState = stateNone

//export fileExplorerLogCallbackCGO
func fileExplorerLogCallbackCGO(msg *C.char) {
	if msg == nil {
		return
	}
	logFromMonitor(C.GoString(msg))
}

//export fileExplorerActivatedCallbackCGO
func fileExplorerActivatedCallbackCGO(pid C.int, isFileDialog C.int, x, y, w, h C.int) {
	isDialog := int(isFileDialog) == 1
	rectX, rectY, rectW, rectH := int(x), int(y), int(w), int(h)
	var deactivated func()
	var activated func(pid int)

	stateMu.Lock()
	if isDialog {
		if currentState == stateExplorer {
			explorerActive = false
			deactivated = explorerDeactivatedCallback
		}
		currentState = stateDialog
		dialogActive = true
		dialogRectX = rectX
		dialogRectY = rectY
		dialogRectW = rectW
		dialogRectH = rectH
		activated = dialogActivatedCallback
	} else {
		if currentState == stateDialog {
			dialogActive = false
			deactivated = dialogDeactivatedCallback
		}
		currentState = stateExplorer
		explorerActive = true
		explorerRectX = rectX
		explorerRectY = rectY
		explorerRectW = rectW
		explorerRectH = rectH
		activated = explorerActivatedCallback
	}
	stateMu.Unlock()

	logFromMonitor(fmt.Sprintf("go activate: pid=%d dialog=%t rect=(%d,%d,%d,%d) state=%d", int(pid), isDialog, rectX, rectY, rectW, rectH, currentState))

	if deactivated != nil {
		deactivated()
	}
	if activated != nil {
		activated(int(pid))
	}
}

//export fileExplorerDeactivatedCallbackCGO
func fileExplorerDeactivatedCallbackCGO() {
	var deactivated func()
	var previousState monitorState

	stateMu.Lock()
	previousState = currentState
	if currentState == stateExplorer {
		explorerActive = false
		deactivated = explorerDeactivatedCallback
	}
	if currentState == stateDialog {
		dialogActive = false
		deactivated = dialogDeactivatedCallback
	}
	currentState = stateNone
	stateMu.Unlock()

	logFromMonitor(fmt.Sprintf("go deactivate: previousState=%d", previousState))

	if deactivated != nil {
		deactivated()
	}
}

func checkUpdateMonitorState() {
	stateMu.RLock()
	needMonitor := explorerActivatedCallback != nil || explorerDeactivatedCallback != nil ||
		dialogActivatedCallback != nil || dialogDeactivatedCallback != nil
	needRawListener := explorerKeyListener != nil || dialogKeyListener != nil
	stateMu.RUnlock()

	if needMonitor {
		C.startFileExplorerMonitor()
	} else {
		C.stopFileExplorerMonitor()
		stateMu.Lock()
		currentState = stateNone
		explorerActive = false
		dialogActive = false
		stateMu.Unlock()
	}

	if needRawListener {
		if rawKeySubscription == nil {
			subscription, err := keyboard.AddRawKeyListener(handleExplorerRawKeyEvent)
			if err == nil {
				rawKeySubscription = subscription
				logFromMonitor("go raw listener: subscribed")
			} else {
				logFromMonitor(fmt.Sprintf("go raw listener: subscribe failed err=%v", err))
			}
		}
		return
	}

	if rawKeySubscription != nil {
		_ = rawKeySubscription.Close()
		rawKeySubscription = nil
		logFromMonitor("go raw listener: unsubscribed")
	}
}

func StartExplorerMonitor(activated func(pid int), deactivated func(), keyListener func(string)) {
	stateMu.Lock()
	explorerActivatedCallback = activated
	explorerDeactivatedCallback = deactivated
	explorerKeyListener = keyListener
	stateMu.Unlock()
	checkUpdateMonitorState()
}

func StopExplorerMonitor() {
	stateMu.Lock()
	explorerActivatedCallback = nil
	explorerDeactivatedCallback = nil
	explorerKeyListener = nil
	if currentState == stateExplorer {
		currentState = stateNone
		explorerActive = false
	}
	stateMu.Unlock()
	checkUpdateMonitorState()
}

func GetActiveExplorerRect() (int, int, int, int, bool) {
	stateMu.RLock()
	defer stateMu.RUnlock()
	if explorerActive {
		return explorerRectX, explorerRectY, explorerRectW, explorerRectH, true
	}
	return 0, 0, 0, 0, false
}

func StartExplorerOpenSaveMonitor(activated func(pid int), deactivated func(), keyListener func(string)) {
	stateMu.Lock()
	dialogActivatedCallback = activated
	dialogDeactivatedCallback = deactivated
	dialogKeyListener = keyListener
	stateMu.Unlock()
	checkUpdateMonitorState()
}

func StopExplorerOpenSaveMonitor() {
	stateMu.Lock()
	dialogActivatedCallback = nil
	dialogDeactivatedCallback = nil
	dialogKeyListener = nil

	if currentState == stateDialog {
		currentState = stateNone
		dialogActive = false
	}
	stateMu.Unlock()
	checkUpdateMonitorState()
}

func GetActiveDialogRect() (int, int, int, int, bool) {
	stateMu.RLock()
	defer stateMu.RUnlock()
	if dialogActive {
		return dialogRectX, dialogRectY, dialogRectW, dialogRectH, true
	}
	return 0, 0, 0, 0, false
}

func handleExplorerRawKeyEvent(event keyboard.RawKeyEvent) bool {
	if event.Type != keyboard.EventTypeKeyDown || event.Key == keyboard.KeyUnknown || event.Character == "" {
		logFromMonitor(fmt.Sprintf("go raw key: ignore invalid type=%d key=%d char=%q modifiers=%d", event.Type, event.Key, event.Character, event.Modifiers))
		return false
	}

	if event.Modifiers&(keyboard.ModifierCtrl|keyboard.ModifierAlt|keyboard.ModifierSuper) != 0 {
		logFromMonitor(fmt.Sprintf("go raw key: ignore modifiers key=%d char=%q modifiers=%d", event.Key, event.Character, event.Modifiers))
		return false
	}

	// Refresh native Explorer/dialog state before consulting currentState.
	// On Windows, returning focus to the same Explorer HWND after Wox hides does
	// not always produce a new WinEvent activation callback, so relying on the
	// cached state alone regresses type-to-search after the first successful use.
	if int(C.refreshFileExplorerMonitorState()) == 0 {
		stateMu.RLock()
		state := currentState
		stateMu.RUnlock()
		logFromMonitor(fmt.Sprintf("go raw key: ignore no active explorer/dialog key=%d char=%q state=%d", event.Key, event.Character, state))
		return false
	}

	// Focus filtering must happen after the state refresh so the file-list check
	// is evaluated against the actual foreground Explorer/dialog window.
	if int(C.isForegroundExplorerFileListFocused()) == 0 {
		stateMu.RLock()
		state := currentState
		stateMu.RUnlock()
		logFromMonitor(fmt.Sprintf("go raw key: ignore focus key=%d char=%q state=%d", event.Key, event.Character, state))
		return false
	}

	stateMu.RLock()
	state := currentState
	explorerListener := explorerKeyListener
	dialogListener := dialogKeyListener
	stateMu.RUnlock()

	if state == stateNone {
		logFromMonitor(fmt.Sprintf("go raw key: ignore no active explorer/dialog key=%d char=%q state=%d", event.Key, event.Character, state))
		return false
	}

	key := strings.ToLower(event.Character)
	logFromMonitor(fmt.Sprintf("go raw key: dispatch key=%q state=%d explorerListener=%t dialogListener=%t", key, state, explorerListener != nil, dialogListener != nil))

	if state == stateExplorer && explorerListener != nil {
		explorerListener(key)
		logFromMonitor(fmt.Sprintf("go raw key: consumed explorer key=%q", key))
		return true
	}

	if state == stateDialog && dialogListener != nil {
		dialogListener(key)
		logFromMonitor(fmt.Sprintf("go raw key: consumed dialog key=%q", key))
		return true
	}

	logFromMonitor(fmt.Sprintf("go raw key: no consumer key=%q state=%d", key, state))
	return false
}
