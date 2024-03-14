/*
Copyright (c) 2024 Diagrid Inc.
Licensed under the MIT License.
*/

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	etcdcron "github.com/diagridio/go-etcd-cron"
	partitioning "github.com/diagridio/go-etcd-cron/partitioning"
	"google.golang.org/protobuf/types/known/anypb"
)

func main() {
	hostId, err := strconv.Atoi(os.Getenv("HOST_ID"))
	if err != nil {
		hostId = 0
	}
	numHosts, err := strconv.Atoi(os.Getenv("NUM_HOSTS"))
	if err != nil {
		numHosts = 1
	}
	numPartitions, err := strconv.Atoi(os.Getenv("NUM_PARTITIONS"))
	if err != nil {
		numPartitions = 1
	}
	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		namespace = "example"
	}

	log.Printf("starting hostId=%d for total of %d hosts and %d partitions", hostId, numHosts, numPartitions)

	p, err := partitioning.NewPartitioning(numPartitions, numHosts, hostId)
	if err != nil {
		log.Fatal("fail to create partitioning", err)
	}
	cron, err := etcdcron.New(
		etcdcron.WithNamespace(namespace),
		etcdcron.WithPartitioning(p),
		etcdcron.WithTriggerFunc(func(ctx context.Context, metadata map[string]string, payload *anypb.Any) error {
			log.Printf("Trigger from pid %d: %s\n", os.Getpid(), string(payload.Value))
			return nil
		}),
	)
	if err != nil {
		log.Fatal("fail to create etcd-cron", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	var wg sync.WaitGroup
	// Start a goroutine to listen for signals
	go func() {
		// Wait for a signal
		sig := <-signalChannel
		fmt.Println("\nReceived signal:", sig)

		// Clean up and notify the main goroutine to exit
		cancel()
		wg.Done()
	}()

	if os.Getenv("ADD") == "1" {
		cron.AddJob(ctx, etcdcron.Job{
			Name:      "every-2s-dFG3F3DSGSGds",
			Rhythm:    "*/2 * * * * *",
			StartTime: time.Time{}, // even seconds
			Payload:   &anypb.Any{Value: []byte("ev 2s even")},
		})
		cron.AddJob(ctx, etcdcron.Job{
			Name:      "every-2s-b34w5y5hbwthjs",
			Rhythm:    "*/2 * * * * *",
			StartTime: time.Time{}.Add(time.Second), // odd seconds
			Payload:   &anypb.Any{Value: []byte("ev 2s odd")},
		})
		cron.AddJob(ctx, etcdcron.Job{
			Name:    "every-10s-bnsf45354wbdsnd",
			Rhythm:  "*/10 * * * * *",
			Payload: &anypb.Any{Value: []byte("ev 10s")},
		})
		cron.AddJob(ctx, etcdcron.Job{
			Name:    "every-3s-mdhgm764324rqdg",
			Rhythm:  "*/3 * * * * *",
			Payload: &anypb.Any{Value: []byte("ev 3s")},
		})
		cron.AddJob(ctx, etcdcron.Job{
			Name:    "every-4s-vdafbrtjnysh245",
			Rhythm:  "*/4 * * * * *",
			Payload: &anypb.Any{Value: []byte("ev 4s")},
		})
		cron.AddJob(ctx, etcdcron.Job{
			Name:    "every-5s-adjbg43q5rbafbr44",
			Rhythm:  "*/5 * * * * *",
			Payload: &anypb.Any{Value: []byte("ev 5s")},
		})
		cron.AddJob(ctx, etcdcron.Job{
			Name:    "every-6s-abadfh52jgdyj467",
			Rhythm:  "*/6 * * * * *",
			Payload: &anypb.Any{Value: []byte("ev 6s")},
		})
		cron.AddJob(ctx, etcdcron.Job{
			Name:    "every-7s-bndasfbn4q55fgn",
			Rhythm:  "*/7 * * * * *",
			Payload: &anypb.Any{Value: []byte("ev 7s")},
		})
		cron.AddJob(ctx, etcdcron.Job{
			Name:    "every-1s-then-expire-hadfh452erhh",
			Rhythm:  "*/1 * * * * *",
			TTL:     10,
			Payload: &anypb.Any{Value: []byte("ev 1s then expires after 10s")},
		})
	}
	cron.Start(ctx)

	// Wait for graceful shutdown on interrupt signal
	wg.Add(1)
	wg.Wait()

	fmt.Println("Program gracefully terminated.")
}
