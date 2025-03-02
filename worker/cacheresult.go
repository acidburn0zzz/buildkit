package worker

import (
	"context"
	"strings"
	"time"

	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/solver"
	"github.com/moby/buildkit/util/compression"
	"github.com/pkg/errors"
)

func NewCacheResultStorage(wc *Controller) solver.CacheResultStorage {
	return &cacheResultStorage{
		wc: wc,
	}
}

type cacheResultStorage struct {
	wc *Controller
}

func (s *cacheResultStorage) Save(res solver.Result, createdAt time.Time) (solver.CacheResult, error) {
	ref, ok := res.Sys().(*WorkerRef)
	if !ok {
		return solver.CacheResult{}, errors.Errorf("invalid result: %T", res.Sys())
	}
	if ref.ImmutableRef != nil {
		if !ref.ImmutableRef.HasCachePolicyRetain() {
			if err := ref.ImmutableRef.SetCachePolicyRetain(); err != nil {
				return solver.CacheResult{}, err
			}
		}
	}
	return solver.CacheResult{ID: ref.ID(), CreatedAt: createdAt}, nil
}
func (s *cacheResultStorage) Load(ctx context.Context, res solver.CacheResult) (solver.Result, error) {
	return s.load(ctx, res.ID, false)
}

func (s *cacheResultStorage) getWorkerRef(id string) (Worker, string, error) {
	workerID, refID, err := parseWorkerRef(id)
	if err != nil {
		return nil, "", err
	}
	w, err := s.wc.Get(workerID)
	if err != nil {
		return nil, "", err
	}
	return w, refID, nil
}

func (s *cacheResultStorage) load(ctx context.Context, id string, hidden bool) (solver.Result, error) {
	w, refID, err := s.getWorkerRef(id)
	if err != nil {
		return nil, err
	}
	if refID == "" {
		return NewWorkerRefResult(nil, w), nil
	}
	ref, err := w.LoadRef(ctx, refID, hidden)
	if err != nil {
		return nil, err
	}
	return NewWorkerRefResult(ref, w), nil
}

func (s *cacheResultStorage) LoadRemotes(ctx context.Context, res solver.CacheResult, compressionopt *solver.CompressionOpt, g session.Group) ([]*solver.Remote, error) {
	w, refID, err := s.getWorkerRef(res.ID)
	if err != nil {
		return nil, err
	}
	ref, err := w.LoadRef(ctx, refID, true)
	if err != nil {
		return nil, err
	}
	defer ref.Release(context.TODO())
	wref := WorkerRef{ref, w}
	all := true // load as many compression blobs as possible
	if compressionopt == nil {
		compressionopt = &solver.CompressionOpt{Type: compression.Default}
		all = false
	}
	remotes, err := wref.GetRemotes(ctx, false, *compressionopt, all, g)
	if err != nil {
		return nil, nil // ignore error. loadRemote is best effort
	}
	return remotes, nil
}
func (s *cacheResultStorage) Exists(id string) bool {
	ref, err := s.load(context.TODO(), id, true)
	if err != nil {
		return false
	}
	ref.Release(context.TODO())
	return true
}

func parseWorkerRef(id string) (string, string, error) {
	parts := strings.Split(id, "::")
	if len(parts) != 2 {
		return "", "", errors.Errorf("invalid workerref id: %s", id)
	}
	return parts[0], parts[1], nil
}
