package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/atharv3903/graphion/internal/algo"
	"github.com/atharv3903/graphion/internal/cache"
	"github.com/atharv3903/graphion/internal/db"
	"github.com/atharv3903/graphion/internal/model"
)

type Server struct {
	Mux   *http.ServeMux
	Store db.Store
	GCtx  algo.GraphCtx
	RC    *cache.RouteCache
	AdjCap int

}

func New(conn *sql.DB) *Server {
	s := &Server{
		Mux:   http.NewServeMux(),
		Store: db.Store{DB: conn},
		RC:    cache.NewRouteCache(),
		AdjCap:  128, // 2048, // or from env
	}

	s.GCtx = algo.GraphCtx{
		Store: s.Store,
		Adj:   cache.NewAdjCacheWithCap(s.AdjCap),
	}

	s.routes()
	return s
}

func (s *Server) routes() {
	s.Mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})

	s.Mux.HandleFunc("/route", s.handleRoute)
	s.Mux.HandleFunc("/road/update", s.handleUpdate)

	s.Mux.HandleFunc("/debug/clear_cache", func(w http.ResponseWriter, r *http.Request) {
		// s.GCtx.Adj = cache.NewAdjCache()
		s.GCtx.Adj = cache.NewAdjCacheWithCap(s.AdjCap)

		s.RC = cache.NewRouteCache()
		w.Write([]byte("cleared"))
	})

	s.Mux.HandleFunc("/debug/adjcache_stats", func(w http.ResponseWriter, r *http.Request) {
		gets, hits, puts, evictions := s.GCtx.Adj.Stats()
		stats := map[string]int{
			"gets":      gets,
			"hits":      hits,
			"puts":      puts,
			"evictions": evictions,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	})

	




}

func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	src, _ := strconv.ParseInt(q.Get("src"), 10, 64)
	dst, _ := strconv.ParseInt(q.Get("dst"), 10, 64)

	key := cache.RouteKey{
		Src:   src,
		Dst:   dst,
		Algo:  "dijkstra",
		Epoch: s.RC.Epoch(),
	}

	if v, ok := s.RC.Get(key); ok {
		json.NewEncoder(w).Encode(model.RouteResponse{Path: v, CacheHit: true})
		return
	}

	cost := func(dist, speed int) int { return dist }

	path, total, explored, err := algo.Dijkstra(s.GCtx, src, dst, cost)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if len(path) > 0 {
		s.RC.Put(key, path)
	}

	json.NewEncoder(w).Encode(model.RouteResponse{
		Path:          path,
		Total:         total,
		ExploredNodes: explored,
		CacheHit:      false,
	})
}

// func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	
// 	var req struct {
// 		EdgeID int64 `json:"edge_id"`
// 		Closed *bool `json:"closed,omitempty"`
// 		Speed  *int  `json:"speed_kmph,omitempty"`
// 		Src    *int64 `json:"src_node,omitempty"`
// 	}

// 	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
// 		http.Error(w, err.Error(), 400)
// 		return
// 	}

// 	if req.Closed != nil {
// 		s.Store.UpdateEdgeClosed(req.EdgeID, *req.Closed)
// 	}

// 	if req.Speed != nil {
// 		s.Store.UpdateEdgeSpeed(req.EdgeID, *req.Speed)
// 	}

// 	// Invalidate specific adjacency
// 	if req.Src != nil {
// 		s.GCtx.Adj.Invalidate(*req.Src)
// 	}

// 	s.RC.BumpEpoch()

	

// 	json.NewEncoder(w).Encode(map[string]any{"ok": true})
// }

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EdgeID int64 `json:"edge_id"`
		Closed *bool `json:"closed,omitempty"`
		Speed  *int  `json:"speed_kmph,omitempty"`
		Src    *int64 `json:"src_node,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	// Do the write(s) inside store which now uses SELECT FOR UPDATE
	if req.Closed != nil {
		if err := s.Store.UpdateEdgeClosed(req.EdgeID, *req.Closed); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}

	if req.Speed != nil {
		if err := s.Store.UpdateEdgeSpeed(req.EdgeID, *req.Speed); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}

	// Invalidate specific adjacency
	if req.Src != nil {
		s.GCtx.Adj.Invalidate(*req.Src)
		// FORCE read of that adjacency to cause DB SELECT load (and refill cache)
		// we ignore the returned edges but this will hit DB via Store.Outgoing
		_, _ = s.GCtx.Store.Outgoing(*req.Src)
	}

	// Bump route epoch so cached routes become stale
	s.RC.BumpEpoch()

	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}





