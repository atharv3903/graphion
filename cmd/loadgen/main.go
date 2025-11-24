package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type RouteResp struct {
	Path          []int64 `json:"path"`
	Total         int     `json:"total"`
	ExploredNodes int     `json:"explored_nodes"`
	CacheHit      bool    `json:"cache_hit"`
}

type AdjStats struct {
    Gets      int `json:"gets"`
    Hits      int `json:"hits"`
    Puts      int `json:"puts"`
    Evictions int `json:"evictions"`
}


func main() {
	if len(os.Args) < 3 {
		log.Fatalf("usage: loadgen <mysql_dsn> <server_addr>")
	}

	dsn := os.Args[1]
	server := os.Args[2]
	duration := 30 * time.Second

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	nodes, err := loadNodes(db)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Loaded %d nodes", len(nodes))

	// Stats for route cache
	var totalReq int64 = 0
	var totalErr int64 = 0
	var totalHit int64 = 0

	// Latency records
	var latencies []time.Duration

	client := &http.Client{Timeout: 10 * time.Second}

	
	// clear cache before test to avoid cumulative stats
	_, err = client.Get(server + "/debug/clear_cache")
	if err != nil {
		log.Fatalf("failed to clear cache: %v", err)
	}
	log.Println("Cache cleared")

	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

	log.Println("Running loadgen for 30 seconds…")

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			goto FINISH
		default:
		}

		src := nodes[rnd.Intn(len(nodes))]
		dst := nodes[rnd.Intn(len(nodes))]

		start := time.Now()
		resp, err := client.Get(fmt.Sprintf("%s/route?src=%d&dst=%d", server, src, dst))
		lat := time.Since(start)

		totalReq++
		latencies = append(latencies, lat)

		if err != nil {
			totalErr++
			continue
		}

		var rr RouteResp
		json.NewDecoder(resp.Body).Decode(&rr)
		resp.Body.Close()

		if rr.CacheHit {
			totalHit++
		}
	}

FINISH:

	// ---- fetch adjacency cache stats ----
	adj := AdjStats{}
	resp, err := client.Get(server + "/debug/adjcache_stats")
	if err == nil {
		json.NewDecoder(resp.Body).Decode(&adj)
		resp.Body.Close()
	}

	fmt.Println("\n========== LOADGEN SUMMARY ==========")
	fmt.Printf("Total Requests: %d\n", totalReq)
	fmt.Printf("Errors: %d\n", totalErr)

	// route cache % 
	routeHitRate := float64(totalHit) / float64(totalReq) * 100
	fmt.Printf("RouteCache Hit Rate: %.1f%%\n", routeHitRate)

	fmt.Printf("Evictions: %d\n", adj.Evictions)


	// adjacency cache %
	if adj.Gets > 0 {
		adjHitRate := float64(adj.Hits) / float64(adj.Gets) * 100
		fmt.Printf("AdjCache Hit Rate: %.1f%% (gets=%d, hits=%d, puts=%d)\n",
			adjHitRate, adj.Gets, adj.Hits, adj.Puts)
	}

	if len(latencies) > 0 {
		var min, max, sum time.Duration
		min = latencies[0]
		max = latencies[0]
		for _, l := range latencies {
			if l < min {
				min = l
			}
			if l > max {
				max = l
			}
			sum += l
		}
		avg := sum / time.Duration(len(latencies))

		fmt.Printf("Avg Latency: %v\n", avg)
		fmt.Printf("Fastest: %v\n", min)
		fmt.Printf("Slowest: %v\n", max)
	}

	fmt.Println("=====================================")
}

func loadNodes(db *sql.DB) ([]int64, error) {
	rows, err := db.Query(`
		SELECT DISTINCT src_node FROM edges
		UNION
		SELECT DISTINCT dst_node FROM edges
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		nodes = append(nodes, id)
	}
	return nodes, nil
}
