//go:build darwin

package filesearch

/*
#cgo LDFLAGS: -framework CoreServices -framework CoreFoundation
#include <CoreServices/CoreServices.h>
#include <CoreFoundation/CoreFoundation.h>
#include <dispatch/dispatch.h>
#include <stdlib.h>
#include <stdint.h>

extern void woxFSEventsCallback(uintptr_t handle, size_t numEvents, char **eventPaths, FSEventStreamEventFlags *eventFlags, FSEventStreamEventId *eventIds);
void woxFSEventsBridge(ConstFSEventStreamRef streamRef, void *clientCallBackInfo, size_t numEvents, void *eventPaths, const FSEventStreamEventFlags eventFlags[], const FSEventStreamEventId eventIds[]);
*/
import "C"

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime/cgo"
	"sync"
	"time"
	"unsafe"
)

const fseventsLatency = time.Second

type FSEventsChangeFeed struct {
	mu        sync.RWMutex
	stream    C.FSEventStreamRef
	queue     C.dispatch_queue_t
	roots     []RootRecord
	signals   chan ChangeSignal
	handle    cgo.Handle
	handlePtr *C.uintptr_t
	closed    bool
}

func NewFSEventsChangeFeed() *FSEventsChangeFeed {
	feed := &FSEventsChangeFeed{
		signals: make(chan ChangeSignal, 256),
	}
	feed.handle = cgo.NewHandle(feed)
	feed.handlePtr = (*C.uintptr_t)(C.malloc(C.size_t(unsafe.Sizeof(C.uintptr_t(0)))))
	*feed.handlePtr = C.uintptr_t(feed.handle)
	return feed
}

func (f *FSEventsChangeFeed) Mode() string {
	return "fsevents"
}

func (f *FSEventsChangeFeed) Signals() <-chan ChangeSignal {
	return f.signals
}

func (f *FSEventsChangeFeed) Refresh(ctx context.Context, roots []RootRecord) error {
	prepared := prepareFSEventsRefresh(roots, time.Now(), defaultFeedCursorSafeWindow)
	for _, signal := range prepared.signals {
		f.emit(signal)
	}

	f.stopCurrentStream()

	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.roots = append([]RootRecord(nil), prepared.watchRoots...)
	f.mu.Unlock()

	if len(prepared.watchRoots) == 0 {
		return nil
	}

	stream, err := f.createStream(prepared.watchRoots, prepared.sinceEventID)
	if err != nil {
		for _, root := range prepared.watchRoots {
			f.emit(ChangeSignal{
				Kind:          ChangeSignalKindFeedUnavailable,
				RootID:        root.ID,
				FeedType:      RootFeedTypeFSEvents,
				Path:          root.Path,
				PathIsDir:     true,
				PathTypeKnown: true,
				Reason:        err.Error(),
				At:            time.Now(),
			})
		}
		return err
	}
	queue, err := f.createQueue()
	if err != nil {
		C.FSEventStreamRelease(stream)
		return err
	}

	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		C.FSEventStreamRelease(stream)
		return nil
	}
	f.stream = stream
	f.queue = queue
	f.mu.Unlock()

	C.FSEventStreamSetDispatchQueue(stream, queue)
	if C.FSEventStreamStart(stream) == 0 {
		for _, root := range prepared.watchRoots {
			f.emit(ChangeSignal{
				Kind:          ChangeSignalKindFeedUnavailable,
				RootID:        root.ID,
				FeedType:      RootFeedTypeFSEvents,
				Path:          root.Path,
				PathIsDir:     true,
				PathTypeKnown: true,
				Reason:        "start fsevents stream",
				At:            time.Now(),
			})
		}
		f.stopCurrentStream()
		return fmt.Errorf("start fsevents stream")
	}

	go func() {
		<-ctx.Done()
		f.stopCurrentStream()
	}()

	return nil
}

func (f *FSEventsChangeFeed) Close() error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true
	f.mu.Unlock()

	f.stopCurrentStream()
	f.handle.Delete()
	if f.handlePtr != nil {
		C.free(unsafe.Pointer(f.handlePtr))
		f.handlePtr = nil
	}
	return nil
}

func (f *FSEventsChangeFeed) SnapshotRootFeed(ctx context.Context, root RootRecord) (RootFeedSnapshot, error) {
	_ = ctx
	cursorText, err := encodeFeedCursor(FeedCursor{
		FeedType:  RootFeedTypeFSEvents,
		UpdatedAt: time.Now().UnixMilli(),
		FSEventID: uint64(C.FSEventsGetCurrentEventId()),
	})
	if err != nil {
		return RootFeedSnapshot{}, err
	}

	return RootFeedSnapshot{
		FeedType:   RootFeedTypeFSEvents,
		FeedCursor: cursorText,
		FeedState:  RootFeedStateReady,
	}, nil
}

func (f *FSEventsChangeFeed) createQueue() (C.dispatch_queue_t, error) {
	label := C.CString("wox.filesearch.fsevents")
	defer C.free(unsafe.Pointer(label))

	queue := C.dispatch_queue_create(label, nil)
	if queue == nil {
		return nil, fmt.Errorf("create fsevents dispatch queue")
	}
	return queue, nil
}

func (f *FSEventsChangeFeed) createStream(roots []RootRecord, sinceEventID uint64) (C.FSEventStreamRef, error) {
	pathArray := C.CFArrayCreateMutable(C.CFAllocatorRef(0), 0, &C.kCFTypeArrayCallBacks)
	if pathArray == 0 {
		return nil, fmt.Errorf("create fsevents path array")
	}
	defer C.CFRelease(C.CFTypeRef(pathArray))

	for _, root := range roots {
		cPath := C.CString(root.Path)
		cfPath := C.CFStringCreateWithCString(C.CFAllocatorRef(0), cPath, C.kCFStringEncodingUTF8)
		C.free(unsafe.Pointer(cPath))
		if cfPath == 0 {
			return nil, fmt.Errorf("create fsevents path string for %q", root.Path)
		}
		C.CFArrayAppendValue(pathArray, unsafe.Pointer(cfPath))
		C.CFRelease(C.CFTypeRef(cfPath))
	}

	context := C.FSEventStreamContext{}
	context.info = unsafe.Pointer(f.handlePtr)
	flags := C.FSEventStreamCreateFlags(
		C.kFSEventStreamCreateFlagFileEvents |
			C.kFSEventStreamCreateFlagWatchRoot |
			C.kFSEventStreamCreateFlagNoDefer,
	)

	stream := C.FSEventStreamCreate(
		C.CFAllocatorRef(0),
		(C.FSEventStreamCallback)(C.woxFSEventsBridge),
		&context,
		C.CFArrayRef(pathArray),
		C.FSEventStreamEventId(sinceEventID),
		C.CFTimeInterval(fseventsLatency.Seconds()),
		flags,
	)
	if stream == nil {
		return nil, fmt.Errorf("create fsevents stream")
	}

	return stream, nil
}

func (f *FSEventsChangeFeed) stopCurrentStream() {
	f.mu.Lock()
	stream := f.stream
	f.stream = nil
	f.queue = nil
	f.mu.Unlock()

	if stream != nil {
		C.FSEventStreamStop(stream)
		C.FSEventStreamInvalidate(stream)
		C.FSEventStreamRelease(stream)
	}
}

func (f *FSEventsChangeFeed) copyRoots() []RootRecord {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return append([]RootRecord(nil), f.roots...)
}

func (f *FSEventsChangeFeed) emit(signal ChangeSignal) {
	if signal.RootID == "" {
		return
	}
	if signal.At.IsZero() {
		signal.At = time.Now()
	}

	f.mu.RLock()
	closed := f.closed
	f.mu.RUnlock()
	if closed {
		return
	}

	select {
	case f.signals <- signal:
	default:
	}
}

func (f *FSEventsChangeFeed) onEvents(paths []string, flags []uint64, ids []uint64) {
	roots := f.copyRoots()
	if len(roots) == 0 {
		return
	}

	now := time.Now()
	for index := range paths {
		eventPath := filepath.Clean(paths[index])
		matchedRoots := make([]RootRecord, 0, len(roots))
		for _, root := range roots {
			if pathWithinScope(root.Path, eventPath) {
				matchedRoots = append(matchedRoots, root)
			}
		}
		if len(matchedRoots) == 0 && fseventRequiresRootReconcile(flags[index]) {
			matchedRoots = roots
		}

		for _, root := range matchedRoots {
			for _, signal := range translateFSEvent(root, eventPath, flags[index], ids[index], now) {
				f.emit(signal)
			}
		}
	}
}

//export woxFSEventsCallback
func woxFSEventsCallback(handle C.uintptr_t, numEvents C.size_t, eventPaths **C.char, eventFlags *C.FSEventStreamEventFlags, eventIds *C.FSEventStreamEventId) {
	feed, ok := cgo.Handle(handle).Value().(*FSEventsChangeFeed)
	if !ok || feed == nil {
		return
	}

	count := int(numEvents)
	pathSlice := unsafe.Slice(eventPaths, count)
	flagSlice := unsafe.Slice(eventFlags, count)
	idSlice := unsafe.Slice(eventIds, count)

	paths := make([]string, 0, count)
	flags := make([]uint64, 0, count)
	ids := make([]uint64, 0, count)
	for index := 0; index < count; index++ {
		paths = append(paths, C.GoString(pathSlice[index]))
		flags = append(flags, uint64(flagSlice[index]))
		ids = append(ids, uint64(idSlice[index]))
	}

	feed.onEvents(paths, flags, ids)
}
