package algo

import (
	"github.com/atharv3903/graphion/internal/cache"
	"github.com/atharv3903/graphion/internal/db"
	"github.com/atharv3903/graphion/internal/model"
)

type GraphCtx struct {
	Store db.Store
	Adj   *cache.AdjCache
}

func (g GraphCtx) Neighbors(n int64) ([]model.Edge, error) {
	if v, ok := g.Adj.Get(n); ok {
		return v, nil
	}

	edges, err := g.Store.Outgoing(n)
	if err != nil {
		return nil, err
	}

	g.Adj.Put(n, edges)
	return edges, nil
}
