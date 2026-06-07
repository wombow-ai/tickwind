package store

import (
	"context"
	"time"
)

// Split forwards Pruner calls to the durable Market store (the User store holds
// only cheap, rebuildable per-user data — nothing worth pruning). If the Market
// store doesn't implement Pruner, each call is a harmless no-op.

func (s *Split) PruneNews(ctx context.Context, before, hotBefore time.Time) (int64, error) {
	if p, ok := s.Market.(Pruner); ok {
		return p.PruneNews(ctx, before, hotBefore)
	}
	return 0, nil
}

func (s *Split) PruneSocial(ctx context.Context, before, hotBefore time.Time, protect []string) (int64, error) {
	if p, ok := s.Market.(Pruner); ok {
		return p.PruneSocial(ctx, before, hotBefore, protect)
	}
	return 0, nil
}

func (s *Split) PruneFilings(ctx context.Context, before time.Time) (int64, error) {
	if p, ok := s.Market.(Pruner); ok {
		return p.PruneFilings(ctx, before)
	}
	return 0, nil
}

func (s *Split) PruneInsiderBuys(ctx context.Context, before time.Time) (int64, error) {
	if p, ok := s.Market.(Pruner); ok {
		return p.PruneInsiderBuys(ctx, before)
	}
	return 0, nil
}

func (s *Split) PruneSeenForm4(ctx context.Context, before time.Time) (int64, error) {
	if p, ok := s.Market.(Pruner); ok {
		return p.PruneSeenForm4(ctx, before)
	}
	return 0, nil
}

func (s *Split) CapPerTicker(ctx context.Context, table string, n int) (int64, error) {
	if p, ok := s.Market.(Pruner); ok {
		return p.CapPerTicker(ctx, table, n)
	}
	return 0, nil
}
