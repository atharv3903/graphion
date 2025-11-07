package main

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var httpClient *http.Client

type EdgePair struct {
	Src int64
	Dst int64
}

func fetchEdgePairs(dsn string) ([]EdgePair, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query("SELECT src_node, dst_node FROM edges")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pairs []EdgePair
	for rows.Next() {
		var src, dst int64
		rows.Scan(&src, &dst)
		pairs = append(pairs, EdgePair{Src: src, Dst: dst})
	}
	return pairs, nil
}

func fetchNodeIDs(dsn string) ([]int64, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query("SELECT node_id FROM nodes")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids, nil
}

func drainAndClose(resp *http.Response) {
	if resp == nil {
		return
	}
	// Drain to enable connection reuse even if server uses chunked/non-empty bodies.
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func clearCache(server string) {
	resp, err := httpClient.Get(server + "/debug/clear_cache")
	if err == nil {
		drainAndClose(resp)
	}
}

func printAdjCacheStats(server string) {
	resp, err := httpClient.Get(server + "/debug/adjcache_stats")
	if err != nil {
		fmt.Println("Failed to fetch stats:", err)
		return
	}
	defer drainAndClose(resp)

	var stats map[string]int
	_ = json.NewDecoder(resp.Body).Decode(&stats)

	gets := stats["gets"]
	hits := stats["hits"]
	puts := stats["puts"]

	hitRate := 0.0
	if gets > 0 {
		hitRate = float64(hits) * 100 / float64(gets)
	}

	fmt.Printf("AdjCache: gets=%d hits=%d puts=%d hit-rate=%.2f%%\n",
		gets, hits, puts, hitRate)
}

func runRoute(server string, p EdgePair, out chan<- float64, wg *sync.WaitGroup, sem chan struct{}) {
	defer wg.Done()
	defer func() { <-sem }()

	url := fmt.Sprintf("%s/route?src=%d&dst=%d", server, p.Src, p.Dst)
	start := time.Now()
	resp, err := httpClient.Get(url)
	if err == nil {
		drainAndClose(resp)
	}
	ms := float64(time.Since(start).Microseconds()) / 1000.0
	out <- ms
}

func runUpdate(server string, edgeID int64, out chan<- float64, wg *sync.WaitGroup, sem chan struct{}) {
	defer wg.Done()
	defer func() { <-sem }()

	url := server + "/road/update"
	body := map[string]any{
		"edge_id":    edgeID,
		"speed_kmph": rand.Intn(70) + 20,
	}
	b, _ := json.Marshal(body)

	start := time.Now()
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(b))
	if err == nil {
		drainAndClose(resp)
	}

	ms := float64(time.Since(start).Microseconds()) / 1000.0
	out <- ms
}

func percentile(lat []float64, p float64) float64 {
	if len(lat) == 0 {
		return 0
	}
	idx := int(float64(len(lat)-1)*p + 0.5)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(lat) {
		idx = len(lat) - 1
	}
	return lat[idx]
}

func runWorkload(server string, pairs []EdgePair, mode, csvfile string, ops int, parallel int, cClearEvery int) {
	switch mode {
	case "A1":
		fmt.Println("Clearing cache (A1 Cold CPU)...")
		clearCache(server)
	case "C":
		fmt.Println("🚀 Mode C — CPU OVERLOAD (routes only, no DB writes)")
		fmt.Println("Clearing cache (C Cold CPU at the start)...")
		clearCache(server)
	}

	fmt.Println("Running workload:", mode)

	sem := make(chan struct{}, parallel)
	out := make(chan float64, ops)
	var wg sync.WaitGroup

	f, _ := os.Create(csvfile)
	w := csv.NewWriter(f)
	// Disable CSV disk writes in Mode C for max generator throughput
	if mode == "C" {
		w = csv.NewWriter(io.Discard)
	}
	_ = w.Write([]string{"op_index", "latency_ms"})

	for i := 0; i < ops; i++ {
		if mode == "C" && cClearEvery > 0 && i > 0 && (i%cClearEvery == 0) {
			clearCache(server)
		}

		wg.Add(1)
		sem <- struct{}{}

		if mode == "B" {
			go runUpdate(server, int64(i%len(pairs)), out, &wg, sem)
		} else {
			idx := rand.Intn(len(pairs))
			go runRoute(server, pairs[idx], out, &wg, sem)
		}
	}

	wg.Wait()
	close(out)

	var lat []float64
	j := 0
	for ms := range out {
		lat = append(lat, ms)
		_ = w.Write([]string{fmt.Sprintf("%d", j), fmt.Sprintf("%.3f", ms)})
		j++
	}

	w.Flush()
	f.Close()

	sort.Float64s(lat)
	avg := 0.0
	for _, x := range lat {
		avg += x
	}
	if len(lat) > 0 {
		avg /= float64(len(lat))
	}

	p50 := percentile(lat, 0.50)
	p95 := percentile(lat, 0.95)
	p99 := percentile(lat, 0.99)

	fmt.Printf("[%s] %d ops parallel=%d — avg=%.2fms p50=%.2fms p95=%.2fms p99=%.2fms\n",
		mode, ops, parallel, avg, p50, p95, p99)

	printAdjCacheStats(server)
}

func main() {
	// ===== FLAGS =====
	var (
		dsn         = flag.String("dsn", "root:admin@tcp(127.0.0.1:3306)/routing", "MySQL DSN")
		server      = flag.String("server", "http://127.0.0.1:8080", "Server base URL")
		ops         = flag.Int("ops", 5000, "Operations per mode")
		parallel    = flag.Int("parallel", 200, "Max in-flight requests")
		pairset     = flag.Int("pairset", 200, "Subset of routes to use")
		cooldown    = flag.Duration("cooldown", 2*time.Second, "Cooldown between workloads")
		pairmode    = flag.String("pairmode", "edge", "pair selection mode: edge | random")
		modeC       = flag.Bool("modeC", false, "CPU overload mode (flatline server core), runs alone")
		cClearEvery = flag.Int("c_clear_every", 0, "Mode C: clear caches every N ops (0 = never)")
	)
	flag.Parse()

	// shared HTTP client with a large keep-alive pool
	httpClient = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        10000,
			MaxIdleConnsPerHost: 10000,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  true,
			// keep-alive is ON by default; so i have not set DisableKeepAlives=true
		},
		Timeout: 0, // no client-side timeout: let the server be the limiter
	}

	// ===== Load Data =====
	fmt.Println("Loading data...")

	var pairs []EdgePair

	if *pairmode == "edge" {
		fmt.Println("Mode: Using EDGE pairs (valid guaranteed src/dst)")
		allPairs, err := fetchEdgePairs(*dsn)
		if err != nil {
			panic(err)
		}
		rand.Shuffle(len(allPairs), func(i, j int) { allPairs[i], allPairs[j] = allPairs[j], allPairs[i] })
		if *pairset > 0 && *pairset < len(allPairs) {
			allPairs = allPairs[:*pairset]
		}
		pairs = allPairs
	} else {
		fmt.Println("Mode: Using RANDOM node pairs")
		nodeIDs, err := fetchNodeIDs(*dsn)
		if err != nil {
			panic(err)
		}
		if *pairset <= 0 {
			*pairset = 200
		}
		for i := 0; i < *pairset; i++ {
			src := nodeIDs[rand.Intn(len(nodeIDs))]
			dst := nodeIDs[rand.Intn(len(nodeIDs))]
			pairs = append(pairs, EdgePair{Src: src, Dst: dst})
		}
	}

	fmt.Printf("Using %d pairs for loadgen\n", len(pairs))

	// ===== Pinning Pause =====
	fmt.Println("✅ Loadgen ready.")
	fmt.Println("👉 PIN CORES NOW:")
	fmt.Println("   - graphion.exe → YOUR target core")
	fmt.Println("   - loadgen.exe  → some other core(s)")
	fmt.Println("👉 Press ENTER to start...")
	fmt.Scanln()

	// ===== Workloads =====
	if *modeC {
		// Mode C runs ALONE
		runWorkload(*server, pairs, "C", "C_cpu_overload.csv", *ops, *parallel, *cClearEvery)
		return
	}

	runWorkload(*server, pairs, "A1", "A1_cold.csv", *ops, *parallel, 0)
	time.Sleep(*cooldown)

	runWorkload(*server, pairs, "A2", "A2_warm.csv", *ops, *parallel, 0)
	time.Sleep(*cooldown)

	runWorkload(*server, pairs, "B", "B_io.csv", *ops, *parallel, 0)
}
