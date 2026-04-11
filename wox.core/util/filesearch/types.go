package filesearch

type SearchQuery struct {
	Raw string
}

type StatusSnapshot struct {
	RootCount         int
	ScanningRootCount int
	ErrorRootCount    int
	ProgressCurrent   int64
	ProgressTotal     int64
	IsIndexing        bool
	IsInitialIndexing bool
	LastError         string
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

type RootKind string

const (
	RootKindDefault RootKind = "default"
	RootKindUser    RootKind = "user"
)

type RootStatus string

const (
	RootStatusIdle     RootStatus = "idle"
	RootStatusScanning RootStatus = "scanning"
	RootStatusError    RootStatus = "error"
)

type RootRecord struct {
	ID              string
	Path            string
	Kind            RootKind
	Status          RootStatus
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

type ProviderCandidate struct {
	Path       string
	Name       string
	ParentPath string
	IsDir      bool
	Score      int64
}
