/*
Copyright (c) 2024 Diagrid Inc.
Licensed under the MIT License.
*/

package etcdcron

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
	"go.etcd.io/etcd/api/v3/mvccpb"
	etcdclient "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/mirror"
	"google.golang.org/protobuf/proto"
)

// The JobStore persists and reads jobs from Etcd.
type JobStore interface {
	Start(ctx context.Context) error
	Put(ctx context.Context, job Job) error
	Delete(ctx context.Context, jobName string) error
	Wait()
}

type etcdStore struct {
	runWaitingGroup sync.WaitGroup
	etcdClient      *etcdclient.Client
	kvStore         etcdclient.KV
	partitioning    Partitioning
	organizer       Organizer
	putCallback     func(context.Context, Job) error
	deleteCallback  func(context.Context, string) error
}

type noStore struct {
	putCallback    func(context.Context, Job) error
	deleteCallback func(context.Context, string) error
}

func NoStore(
	putCallback func(context.Context, Job) error,
	deleteCallback func(context.Context, string) error) JobStore {
	return &noStore{
		putCallback:    putCallback,
		deleteCallback: deleteCallback,
	}
}

func (s *noStore) Start(ctx context.Context) error {
	return nil
}

func (s *noStore) Put(ctx context.Context, job Job) error {
	s.putCallback(ctx, job)
	return nil
}

func (s *noStore) Delete(ctx context.Context, jobName string) error {
	s.deleteCallback(ctx, jobName)
	return nil
}

func (s *noStore) Wait() {}

func NewEtcdJobStore(
	client *etcdclient.Client,
	organizer Organizer,
	partitioning Partitioning,
	putCallback func(context.Context, Job) error,
	deleteCallback func(context.Context, string) error) JobStore {
	return &etcdStore{
		etcdClient:     client,
		kvStore:        etcdclient.NewKV(client),
		partitioning:   partitioning,
		organizer:      organizer,
		putCallback:    putCallback,
		deleteCallback: deleteCallback,
	}
}

func (s *etcdStore) Start(ctx context.Context) error {
	for _, partitionId := range s.partitioning.ListPartitions() {
		// TODO(artursouza): parallelize this per partition.
		partitionPrefix := s.organizer.JobsPath(partitionId) + "/"
		partitionSyncer := mirror.NewSyncer(s.etcdClient, partitionPrefix, 0)
		rc, errc := partitionSyncer.SyncBase(ctx)

		for r := range rc {
			for _, kv := range r.Kvs {
				err := s.notifyPut(ctx, kv, s.putCallback)
				if err != nil {
					return err
				}
			}
		}

		err := <-errc
		if err != nil {
			return err
		}

		s.sync(ctx, partitionPrefix, partitionSyncer)
	}

	return nil
}

func (s *etcdStore) Put(ctx context.Context, job Job) error {
	opts := []etcdclient.OpOption{}
	if job.TTL > 0 {
		// Create a lease
		lease, err := s.etcdClient.Grant(ctx, job.TTL)
		if err != nil {
			return errors.Errorf("failed to create lease to save job %s: %v", job.Name, err)
		}

		opts = append(opts, etcdclient.WithLease(lease.ID))
	}

	record := &JobRecord{
		Name:    job.Name,
		Rhythm:  job.Rhythm,
		Type:    job.Type,
		Payload: job.Payload,
	}
	bytes, err := proto.Marshal(record)
	if err != nil {
		return err
	}
	_, err = s.kvStore.Put(
		ctx,
		s.organizer.JobPath(job.Name),
		string(bytes),
		opts...,
	)
	return err
}

func (s *etcdStore) Delete(ctx context.Context, jobName string) error {
	_, err := s.kvStore.Delete(
		ctx,
		s.organizer.JobPath(jobName))
	return err
}

func (s *etcdStore) Wait() {
	s.runWaitingGroup.Wait()
}

func (s *etcdStore) notifyPut(ctx context.Context, kv *mvccpb.KeyValue, callback func(context.Context, Job) error) error {
	record := JobRecord{}
	err := proto.Unmarshal(kv.Value, &record)
	if err != nil {
		return fmt.Errorf("could not unmarshal job for key %s: %v", string(kv.Key), err)
	}
	if record.GetName() == "" || record.GetRhythm() == "" {
		return fmt.Errorf("could not deserialize job for key %s", string(kv.Key))
	}

	return callback(ctx, Job{
		Name:    record.Name,
		Rhythm:  record.Rhythm,
		Type:    record.Type,
		Payload: record.Payload,
	})
}

func (s *etcdStore) notifyDelete(ctx context.Context, name string, callback func(context.Context, string) error) error {
	return callback(ctx, name)
}

func (s *etcdStore) sync(ctx context.Context, prefix string, syncer mirror.Syncer) {
	s.runWaitingGroup.Add(1)
	go func() {
		log.Printf("Started sync for path: %s\n", prefix)
		wc := syncer.SyncUpdates(ctx)
		for {
			select {
			case <-ctx.Done():
				s.runWaitingGroup.Done()
				return
			case wr := <-wc:
				for _, ev := range wr.Events {
					t := ev.Type
					switch t {
					case mvccpb.PUT:
						s.notifyPut(ctx, ev.Kv, s.putCallback)
					case mvccpb.DELETE:
						_, name := filepath.Split(string(ev.Kv.Key))
						s.notifyDelete(ctx, name, s.deleteCallback)
					default:
						log.Printf("Unknown etcd event type: %v", t.String())
					}
				}
			}
		}
	}()
}
