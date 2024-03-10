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

	etcdcron "github.com/diagridio/go-etcd-cron"
	"google.golang.org/protobuf/types/known/anypb"
)

func main() {
	hostId, err := strconv.Atoi(os.Getenv("HOST_ID"))
	if err != nil {
		hostId = 0
	}
	numHosts, err := strconv.Atoi(os.Getenv("NUM_HOSTS"))
	if err != nil {
		numHosts = 2
	}
	numPartitions, err := strconv.Atoi(os.Getenv("NUM_PARTITIONS"))
	if err != nil {
		numPartitions = 5
	}

	log.Printf("starting hostId=%d for total of %d hosts and %d partitions", hostId, numHosts, numPartitions)

	p, err := etcdcron.NewPartitioning(numPartitions, numHosts, hostId)
	if err != nil {
		log.Fatal("fail to create partitioning", err)
	}
	cron, err := etcdcron.New(
		etcdcron.WithNamespace("example"),
		etcdcron.WithPartitioning(p),
		etcdcron.WithTriggerFunc(func(ctx context.Context, triggerType string, payload *anypb.Any) error {
			fmt.Printf("Trigger from pid %d: %s %s\n", os.Getpid(), triggerType, string(payload.Value))
			return nil
		}),
	)
	if err != nil {
		log.Fatal("fail to create etcd-cron", err)
	}

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	var wg sync.WaitGroup
	// Start a goroutine to listen for signals
	go func() {
		// Wait for a signal
		sig := <-signalChannel
		fmt.Println("\nReceived signal:", sig)

		// Clean up and notify the main goroutine to exit
		cron.Stop()
		wg.Done()
	}()

	if os.Getenv("ADD") == "1" {
		cron.AddJob(etcdcron.Job{
			Name:    "error-every-2s",
			Rhythm:  "*/2 * * * * *",
			Type:    "stdout", // can be anything the client wants
			Payload: &anypb.Any{Value: []byte("even error")},
		})
		cron.AddJob(etcdcron.Job{
			Name:    "echo-every-10s",
			Rhythm:  "*/10 * * * * *",
			Type:    "stdout", // can be anything the client wants
			Payload: &anypb.Any{Value: []byte("every 10 seconds")},
		})
		cron.AddJob(etcdcron.Job{
			Name:    "error-every-3s",
			Rhythm:  "*/3 * * * * *",
			Type:    "stdout", // can be anything the client wants
			Payload: &anypb.Any{Value: []byte("odd error")},
		})
		cron.AddJob(etcdcron.Job{
			Name:    "error-every-4s",
			Rhythm:  "*/4 * * * * *",
			Type:    "stdout", // can be anything the client wants
			Payload: &anypb.Any{Value: []byte("fourth error")},
		})
		cron.AddJob(etcdcron.Job{
			Name:    "error-every-5s",
			Rhythm:  "*/5 * * * * *",
			Type:    "stdout", // can be anything the client wants
			Payload: &anypb.Any{Value: []byte("fifth error")},
		})
		cron.AddJob(etcdcron.Job{
			Name:    "error-every-6s",
			Rhythm:  "*/6 * * * * *",
			Type:    "stdout", // can be anything the client wants
			Payload: &anypb.Any{Value: []byte("sixth error")},
		})
		cron.AddJob(etcdcron.Job{
			Name:    "error-every-7s",
			Rhythm:  "*/7 * * * * *",
			Type:    "stdout", // can be anything the client wants
			Payload: &anypb.Any{Value: []byte("seventh error")},
		})
	}
	cron.Start(context.Background())

	// Wait for graceful shutdown on interrupt signal
	wg.Add(1)
	wg.Wait()

	fmt.Println("Program gracefully terminated.")
}
