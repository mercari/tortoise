package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"time"
)

func main() {
	maxprocs := runtime.NumCPU()
	// Read commandline
	var url = flag.String("url", "", "the URL to send the traffic")
	var parallel = flag.Int("parallel", maxprocs, "the number of goroutine working on sending the traffic. Use it to adjust the amount of traffic. (default: the number of CPU)")
	flag.Parse()
	if url == nil || *url == "" {
		fmt.Println("the --url option is required")
		os.Exit(1)
	}

	fmt.Println(fmt.Sprintf("Start loadtester: url %s parallel %d", *url, *parallel))

	ctx, cancel := context.WithCancel(context.Background())
	m := newManager(*url, ctx.Done(), *parallel)
	go m.run()
	fmt.Println("Start workers")

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	select {
	case <-quit:
		fmt.Println("Received SIGTERM")
	case <-ctx.Done():
	}
	cancel()
}

type manager struct {
	workers []*worker
	factor  time.Duration
	stopCh  <-chan struct{}
}

func newManager(
	url string,
	stopCh <-chan struct{},
	parallel int,
) *manager {
	interval := 500 * time.Millisecond
	workers := make([]*worker, parallel)
	for i := range workers {
		workers[i] = &worker{url: url, stopCh: stopCh, interval: interval}
	}
	return &manager{
		workers: workers,
		factor:  10 * time.Microsecond, // will be multiplied to 1-13
	}
}

func (m *manager) updateIntervals(d time.Duration) {
	for i := range m.workers {
		m.workers[i].interval = d
	}
}

func (m *manager) run() {
	for _, w := range m.workers {
		// start each worker
		w.run()
	}

	for {
		oneHourAfter := time.Now().Add(1 * time.Hour)
		nextHour := time.Date(oneHourAfter.Year(), oneHourAfter.Month(), oneHourAfter.Day(), oneHourAfter.Hour(), 0, 0, 0, oneHourAfter.Location())

		select {
		case <-time.After(time.Until(nextHour)):
			nextInterval := math.Abs(float64(nextHour.Hour()-11)) + 1 // 1 - 13
			m.updateIntervals(time.Duration(nextInterval) * m.factor)
		case <-m.stopCh:
			return
		}
	}
}

type worker struct {
	url      string
	stopCh   <-chan struct{}
	interval time.Duration
}

func (w *worker) run() {
	go func() {
		for {
			send(w.url)
			select {
			case <-time.After(w.interval):
			case <-w.stopCh:
				return
			}
		}
	}()
}

func send(url string) {
	resp, err := http.Get(url)
	defer resp.Body.Close()
	if err != nil {
		fmt.Println(err)
		return
	}
	if resp.StatusCode != 200 {
		fmt.Println(resp.StatusCode)
	}
}
