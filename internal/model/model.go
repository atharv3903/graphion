package model

type Edge struct {
	Src   int64
	Dst   int64
	DistM int
	Speed int
}

type RouteResponse struct {
	Path          []int64 `json:"path"`
	Total         int     `json:"total"`
	ExploredNodes int     `json:"explored_nodes"`
	CacheHit      bool    `json:"cache_hit"`
}
