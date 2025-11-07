package algo

import "container/heap"

type pqItem struct {
	node int64
	dist int
}

type pq []pqItem

func (p pq) Len() int           { return len(p) }
func (p pq) Less(i, j int) bool { return p[i].dist < p[j].dist }
func (p pq) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func (p *pq) Push(x any) {
	*p = append(*p, x.(pqItem))
}

func (p *pq) Pop() any {
	old := *p
	n := len(old)
	item := old[n-1]
	*p = old[:n-1]
	return item
}

func Dijkstra(ctx GraphCtx, src, dst int64, cost func(int, int) int) ([]int64, int, int, error) {
	dist := map[int64]int{src: 0}
	prev := map[int64]int64{}
	pq := &pq{}
	heap.Push(pq, pqItem{node: src, dist: 0})
	explored := 0

	for pq.Len() > 0 {
		cur := heap.Pop(pq).(pqItem)
		u := cur.node

		if u == dst {
			break
		}

		explored++

		neighbors, err := ctx.Neighbors(u)
		if err != nil {
			return nil, 0, explored, err
		}

		for _, e := range neighbors {
			w := cost(e.DistM, e.Speed)
			nd := dist[u] + w

			old, found := dist[e.Dst]

			if !found || nd < old {
				dist[e.Dst] = nd
				prev[e.Dst] = u
				heap.Push(pq, pqItem{node: e.Dst, dist: nd})
			}
		}
	}

	if _, ok := dist[dst]; !ok {
		return nil, 0, explored, nil
	}

	// reconstruct
	path := []int64{}
	cur := dst

	for cur != src {
		path = append(path, cur)
		cur = prev[cur]
	}
	path = append(path, src)

	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	return path, dist[dst], explored, nil
}
