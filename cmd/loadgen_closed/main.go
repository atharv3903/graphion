package main

import (
	"bufio"  
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type RouteResp struct {
	Path          []int64 `json:"path"`
	Total         int     `json:"total"`
	ExploredNodes int     `json:"explored_nodes"`
	CacheHit      bool    `json:"cache_hit"`
}

type Result struct {
	Clients     int
	AvgLatency  float64
	Throughput  float64
}

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: loadgen_closed <server_addr>")
	}
	server := os.Args[1]

	client := &http.Client{Timeout: 5 * time.Second}

	// warm the server (so cold-start effects don't matter)
	client.Get(server + "/debug/clear_cache")
	client.Get(fmt.Sprintf("%s/route?src=%d&dst=%d", server, 1, 2))

	clientCounts := []int{2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28, 30, 32, 34, 36, 38, 40, 42, 44, 46, 48, 50, 52, 54, 56, 58, 60, 62, 64, 66, 68, 70, 72, 74, 76, 78, 80, 82, 84, 86, 88, 90, 92, 94, 96, 98, 100}

	// {1, 2, 3,  4, 5, 6 , 7 , 8, 12 , 16, 20, 32}
	// {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	// { 2, 6, 10, 14,  18,  22,  26,  30 , 34 , 38 , 42 , 46 , 50}
	// {2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28, 30, 32, 34, 36, 38, 40, 42, 44, 46, 48, 50}


	testDuration := 10 * time.Second

	var results []Result

	for _, n := range clientCounts {
		fmt.Printf("\n== Running test with %d clients ==\n", n)
		avgLat, throughput := runClosedLoop(server, n, testDuration)
		results = append(results, Result{
			Clients:    n,
			AvgLatency: avgLat,
			Throughput: throughput,
		})
	}

	fmt.Println("\n========== CLOSED-LOOP RESULTS (CSV) ==========")
	fmt.Println("clients,avg_latency_ms,throughput_rps")
	for _, r := range results {
		fmt.Printf("%d,%.4f,%.2f\n", r.Clients, r.AvgLatency, r.Throughput)
	}
	fmt.Println("===============================================")

	f, _ := os.Create("results.csv")
	defer f.Close()

	fmt.Fprintf(f, "clients,avg_latency_ms,throughput_rps\n")
	for _, r := range results {
		fmt.Fprintf(f, "%d,%.4f,%.2f\n", r.Clients, r.AvgLatency, r.Throughput)
	}


	fmt.Println("===============================================")
	fmt.Println("\nPress ENTER to exit.")
	
	bufio.NewReader(os.Stdin).ReadBytes('\n')


}

// --------------------------------------------
// CLOSED LOOP TEST FOR A FIXED WORKER COUNT
// --------------------------------------------
func runClosedLoop(server string, clients int, dur time.Duration) (float64, float64) {

	// client := &http.Client{Timeout: 10 * time.Second}

	transport := &http.Transport{
		MaxIdleConns:        500,
		MaxIdleConnsPerHost: 500,
		MaxConnsPerHost:     2000,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}



	ctx, cancel := context.WithTimeout(context.Background(), dur)
	defer cancel()

	var wg sync.WaitGroup

	var mu sync.Mutex
	var totalLatency time.Duration
	var totalReq int64

	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				src := 1 + time.Now().UnixNano()%25000
				dst := 1 + time.Now().UnixNano()%25000

				url := fmt.Sprintf("%s/route?src=%d&dst=%d", server, src, dst)

				start := time.Now()
				resp, err := client.Get(url)
				// 1ms sleep:
				time.Sleep(100 * time.Microsecond)

				lat := time.Since(start)

				if err == nil {
					json.NewDecoder(resp.Body).Decode(&RouteResp{})
					resp.Body.Close()

					mu.Lock()
					totalLatency += lat
					totalReq++
					mu.Unlock()
				}
			}
		}()
	}

	wg.Wait()

	if totalReq == 0 {
		return 0, 0
	}

	avgLat := float64(totalLatency.Milliseconds()) / float64(totalReq)
	throughput := float64(totalReq) / dur.Seconds()

	return avgLat, throughput
}
