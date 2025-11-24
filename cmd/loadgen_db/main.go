package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Payload for /road/update
type UpdateResp struct {
	Ok bool `json:"ok"`
}

type Result struct {
	Clients    int
	AvgLatency float64
	P50        float64
	P95        float64
	P99        float64
	Throughput float64
	Errors     int64
	Total      int64
}

func main() {

	if len(os.Args) < 3 {
		log.Fatalf("usage: loadgen_db <mysql_dsn> <server_addr>")
	}

	dsn := os.Args[1]
	server := os.Args[2]

	// DB connection (for live-edge fetching)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	httpClient := &http.Client{Timeout: 5 * time.Second}

	// Warm server
	httpClient.Get(server + "/debug/clear_cache")
	time.Sleep(300 * time.Millisecond)

	fmt.Println("🔥 Running DB-bound WRITE workload (increasing clients)")

	clientCounts := []int{
		 2,  4,  6,  8,  10,  12,  14, 15,  16,
		18, 19, 20, 
	}

	testDuration := 5 * time.Second
	var results []Result

	for _, clients := range clientCounts {
		fmt.Printf("\n== %d CLIENTS ==\n", clients)
		res := runDBWriteTest(db, server, clients, testDuration)
		results = append(results, res)

		fmt.Printf("RPS: %.2f | Avg %.2fms | P99 %.2fms | Errors=%d/%d\n",
			res.Throughput, res.AvgLatency, res.P99, res.Errors, res.Total)
	}

	// CSV output
	fmt.Println("\nclients,avg_ms,p50,p95,p99,throughput,errors,total")
	for _, r := range results {
		fmt.Printf("%d,%.2f,%.2f,%.2f,%.2f,%.2f,%d,%d\n",
			r.Clients, r.AvgLatency, r.P50, r.P95, r.P99,
			r.Throughput, r.Errors, r.Total)
	}

	f, _ := os.Create("results_db.csv")
	defer f.Close()

	fmt.Fprintf(f, "clients,avg_ms,p50,p95,p99,throughput,errors,total\n")
	for _, r := range results {
		fmt.Fprintf(f, "%d,%.2f,%.2f,%.2f,%.2f,%.2f,%d,%d\n",
			r.Clients, r.AvgLatency, r.P50, r.P95, r.P99,
			r.Throughput, r.Errors, r.Total)
	}

	fmt.Println("\nSaved results_db.csv")
	fmt.Println("Press ENTER to exit.")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
}

//
// MAIN WORKER LOGIC — true DB-bound load
//
func runDBWriteTest(db *sql.DB, server string, clients int, dur time.Duration) Result {

	client := &http.Client{Timeout: 5 * time.Second}

	ctx, cancel := context.WithTimeout(context.Background(), dur)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex

	latencies := []time.Duration{}
	var totalReq int64
	var totalErr int64

	for w := 0; w < clients; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			rng := rand.New(rand.NewSource(time.Now().UnixNano()))

			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				// Fetch random live edge from DB
				var edgeID int64
				var srcNode int64

				row := db.QueryRow(`
					SELECT edge_id, src_node
					FROM edges
					ORDER BY RAND()
					LIMIT 1
				`)

				if err := row.Scan(&edgeID, &srcNode); err != nil {
					mu.Lock()
					totalErr++
					totalReq++
					mu.Unlock()
					continue
				}

				// Build JSON update request
				updateType := rng.Intn(2)
				var body map[string]interface{}

				if updateType == 0 {
					body = map[string]interface{}{
						"edge_id":    edgeID,
						"speed_kmph": 30 + rng.Intn(90),
						"src_node":   srcNode,
					}
				} else {
					body = map[string]interface{}{
						"edge_id":  edgeID,
						"closed":   (rng.Intn(2) == 1),
						"src_node": srcNode,
					}
				}

				jsonBody, _ := json.Marshal(body)

				start := time.Now()
				resp, err := client.Post(server+"/road/update", "application/json", bytes.NewBuffer(jsonBody))
				lat := time.Since(start)

				mu.Lock()
				totalReq++
				mu.Unlock()

				if err != nil {
					mu.Lock()
					totalErr++
					mu.Unlock()
					continue
				}

				io := resp.Body
				_ = json.NewDecoder(io).Decode(&UpdateResp{})
				io.Close()

				mu.Lock()
				latencies = append(latencies, lat)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	avg := computeAvg(latencies)
	p50, p95, p99 := computePercentiles(latencies)
	rps := float64(totalReq) / dur.Seconds()

	return Result{
		Clients:    clients,
		AvgLatency: avg,
		P50:        p50,
		P95:        p95,
		P99:        p99,
		Throughput: rps,
		Errors:     totalErr,
		Total:      totalReq,
	}
}

//
// Helpers
//
func computeAvg(l []time.Duration) float64 {
	var sum time.Duration
	for _, x := range l {
		sum += x
	}
	return float64(sum.Milliseconds()) / float64(len(l))
}

func computePercentiles(l []time.Duration) (p50, p95, p99 float64) {
	if len(l) == 0 {
		return 0, 0, 0
	}
	tmp := make([]time.Duration, len(l))
	copy(tmp, l)
	sort.Slice(tmp, func(i, j int) bool { return tmp[i] < tmp[j] })

	idx := func(p float64) int {
		i := int(float64(len(tmp)) * p)
		if i >= len(tmp) {
			i = len(tmp) - 1
		}
		return i
	}

	return float64(tmp[idx(0.50)].Milliseconds()),
		float64(tmp[idx(0.95)].Milliseconds()),
		float64(tmp[idx(0.99)].Milliseconds())
}
