package plugin

import "context"

var AllSystemPlugin []SystemPlugin

type Plugin interface {
	Init(ctx context.Context, initParams InitParams)
	Query(ctx context.Context, query Query) []QueryResult
}

type SystemPlugin interface {
	Plugin
	GetMetadata() Metadata
}

type FallbackPlugin interface {
	QueryFallback(ctx context.Context, query Query) []QueryResult
}

type InitParams struct {
	API             API
	PluginDirectory string
}
