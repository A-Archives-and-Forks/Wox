package filesearch

import "time"

type SearchQuery struct {
	Raw      string
	wildcard *wildcardQuery
	plan     *queryPlan
}

type StatusSnapshot struct {
	RootCount             int
	PreparingRootCount    int
	ScanningRootCount     int
	SyncingRootCount      int
	WritingRootCount      int
	FinalizingRootCount   int
	ErrorRootCount        int
	PendingDirtyRootCount int
	PendingDirtyPathCount int
	ProgressCurrent       int64
	ProgressTotal         int64
	ActiveRootStatus      RootStatus
	ActiveProgressCurrent int64
	ActiveProgressTotal   int64
	ActiveRootIndex       int
	ActiveRootTotal       int
	ActiveDiscoveredCount int64
	ActiveDirectoryIndex  int
	ActiveDirectoryTotal  int
	ActiveItemCurrent     int64
	ActiveItemTotal       int64
	// Root-local progress was no longer enough once one logical root could fan
	// out into many execution jobs, so these fields expose the active run state
	// without removing the existing root-centric compatibility data.
	ActiveRootPath     string
	ActiveRunStatus    RunStatus
	ActiveJobKind      JobKind
	ActiveScopePath    string
	ActiveStage        RunStage
	RunProgressCurrent int64
	RunProgressTotal   int64
	ErrorRootPath      string
	IsIndexing         bool
	IsInitialIndexing  bool
	LastError          string
}

type SearchResult struct {
	Path       string
	Name       string
	ParentPath string
	IsDir      bool
	Score      int64
}

type SearchStage string

const (
	SearchStagePartial SearchStage = "partial"
	SearchStageUpdated SearchStage = "updated"
	SearchStageFinal   SearchStage = "final"
)

type SearchUpdate struct {
	QueryID string
	Stage   SearchStage
	Results []SearchResult
	IsFinal bool
}

type DirtySignalKind string

const (
	DirtySignalKindRoot DirtySignalKind = "root"
	DirtySignalKindPath DirtySignalKind = "path"
)

type DirtySignal struct {
	Kind          DirtySignalKind
	RootID        string
	TraceID       string
	Path          string
	PathIsDir     bool
	PathTypeKnown bool
	At            time.Time
}

type ReconcileMode string

const (
	ReconcileModeSubtree ReconcileMode = "subtree"
	ReconcileModeRoot    ReconcileMode = "root"
)

type ReconcileBatch struct {
	RootID         string
	TraceID        string
	Mode           ReconcileMode
	Paths          []string
	DirtyPathCount int
}

type RootFeedType string

const (
	RootFeedTypeFallback RootFeedType = "fallback"
	RootFeedTypeFSEvents RootFeedType = "fsevents"
	RootFeedTypeUSN      RootFeedType = "usn"
)

type RootFeedState string

const (
	RootFeedStateReady       RootFeedState = "ready"
	RootFeedStateDegraded    RootFeedState = "degraded"
	RootFeedStateUnavailable RootFeedState = "unavailable"
)

type RootKind string

const (
	RootKindDefault RootKind = "default"
	RootKindUser    RootKind = "user"
)

type RootStatus string

const (
	RootStatusIdle       RootStatus = "idle"
	RootStatusPreparing  RootStatus = "preparing"
	RootStatusScanning   RootStatus = "scanning"
	RootStatusSyncing    RootStatus = "syncing"
	RootStatusWriting    RootStatus = "writing"
	RootStatusFinalizing RootStatus = "finalizing"
	RootStatusError      RootStatus = "error"
)

type ReplaceEntriesStage string

const (
	ReplaceEntriesStagePreparing  ReplaceEntriesStage = "preparing"
	ReplaceEntriesStageWriting    ReplaceEntriesStage = "writing"
	ReplaceEntriesStageFinalizing ReplaceEntriesStage = "finalizing"
)

type ReplaceEntriesProgress struct {
	Stage   ReplaceEntriesStage
	Current int64
	Total   int64
}

type TransientRootState struct {
	Root            RootRecord
	RootIndex       int
	RootTotal       int
	DiscoveredCount int64
	DirectoryIndex  int
	DirectoryTotal  int
	ItemCurrent     int64
	ItemTotal       int64
}

type TransientSyncState struct {
	Root             RootRecord
	RootIndex        int
	RootTotal        int
	Mode             ReconcileMode
	ScopeCount       int
	DirtyPathCount   int
	PendingRootCount int
	PendingPathCount int
}

type RootRecord struct {
	ID              string
	Path            string
	Kind            RootKind
	Status          RootStatus
	FeedType        RootFeedType
	FeedCursor      string
	FeedState       RootFeedState
	LastReconcileAt int64
	LastFullScanAt  int64
	ProgressCurrent int64
	ProgressTotal   int64
	LastError       *string
	CreatedAt       int64
	UpdatedAt       int64
}

const RootProgressScale int64 = 1000

type EntryRecord struct {
	Path           string
	RootID         string
	ParentPath     string
	Name           string
	NormalizedName string
	NormalizedPath string
	PinyinFull     string
	PinyinInitials string
	IsDir          bool
	Mtime          int64
	Size           int64
	UpdatedAt      int64
}

type EntryUpdate struct {
	Old EntryRecord
	New EntryRecord
}

type EntryDeltaBatch struct {
	RootID        string
	PreviousCount int
	NextCount     int
	Added         []EntryRecord
	Updated       []EntryUpdate
	Removed       []EntryRecord
	ForceRebuild  bool
}

type DirectoryRecord struct {
	Path         string
	RootID       string
	ParentPath   string
	LastScanTime int64
	Exists       bool
}

type SubtreeSnapshotBatch struct {
	RootID      string
	ScopePath   string
	Directories []DirectoryRecord
	Entries     []EntryRecord
}

type ProviderCandidate struct {
	Path       string
	Name       string
	ParentPath string
	IsDir      bool
	Score      int64
}
