package git

import (
	"context"
	"github.com/tektoncd/pipeline/pkg/git"
	"github.com/tektoncd/pipeline/pkg/remote"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// Factory creates a git resolver
type Factory struct {
	// Options the default options used when fetching from git
	Options git.FetchSpec
	// Logger for logging output of git operations
	Logger *zap.SugaredLogger
	// Resolver resolver to use for git operations
	// Lets you use a fake provider when testing
	Resolver remote.Resolver
}

// CreateResolver creates the resolver or returns a fake resolver when unit testing
func (f *Factory) CreateResolver(ctx context.Context) remote.Resolver {
	if f.Resolver != nil {
		return f.Resolver
	}
	return NewResolver(f.GetLogger(), f.Options)
}

// GetLogger returns the logger or lazily creates one if required
func (f *Factory) GetLogger() *zap.SugaredLogger {
	if f.Logger == nil {
		observer, _ := observer.New(zap.InfoLevel)
		f.Logger = zap.New(observer).Sugar()
	}
	return f.Logger
}
