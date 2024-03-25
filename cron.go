/*
Copyright (c) 2024 Diagrid Inc.
Licensed under the MIT License.
*/

package etcdcron

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/pkg/errors"
	etcdclient "go.etcd.io/etcd/client/v3"
	anypb "google.golang.org/protobuf/types/known/anypb"

	"github.com/diagridio/go-etcd-cron/collector"
	"github.com/diagridio/go-etcd-cron/counting"
	"github.com/diagridio/go-etcd-cron/locking"
	"github.com/diagridio/go-etcd-cron/partitioning"
	"github.com/diagridio/go-etcd-cron/rhythm"
	"github.com/diagridio/go-etcd-cron/storage"
)

const (
	defaultEtcdEndpoint = "127.0.0.1:2379"
	defaultNamespace    = "etcd_cron"
)

// Cron keeps track of any number of entries, invoking the associated func as
// specified by the schedule. It may be started, stopped, and the entries may
// be inspected while running.
type Cron struct {
	namespace              string
	pendingOperations      []func(context.Context) *Entry
	pendingOperationsMutex sync.RWMutex
	liveOperation          chan func(ctx context.Context) *Entry
	entries                map[string]*Entry
	entriesMutex           sync.RWMutex
	snapshot               chan []*Entry
	etcdErrorsHandler      func(context.Context, Job, error)
	errorsHandler          func(context.Context, Job, error)
	funcCtx                func(context.Context, Job) context.Context
	triggerFunc            TriggerFunction
	running                bool
	runWaitingGroup        sync.WaitGroup
	etcdclient             *etcdclient.Client
	jobStore               storage.JobStore
	organizer              partitioning.Organizer
	partitioning           partitioning.Partitioner
	collector              collector.Collector
}

type TriggerFunction func(ctx context.Context, metadata map[string]string, payload *anypb.Any) (TriggerResult, error)

type TriggerResult int

const (
	OK TriggerResult = iota
	Failure
	Delete
)

// Entry consists of a schedule and the func to execute on that schedule.
type Entry struct {
	// The schedule on which this job should be run.
	Schedule rhythm.Schedule

	// The next time the job will run. This is the zero time if Cron has not been
	// started or this entry's schedule is unsatisfiable
	Next time.Time

	// The last time this job was run. This is the zero time if the job has never
	// been run.
	Prev time.Time

	// The Job o run.
	Job Job

	// Prefix for the ticker mutex
	distMutexPrefix string

	// Counter if has limit on number of triggers
	counter counting.Counter
}

func (e *Entry) tick(now time.Time) {
	e.Prev = e.Next
	start := e.Job.StartTime.Truncate(time.Second)

	if start.After(now) {
		e.Next = start
		return
	}

	e.Next = e.Schedule.Next(start, now)
}

// byTime is a wrapper for sorting the entry array by time
// (with zero time at the end).
type byTime []*Entry

func (s byTime) Len() int      { return len(s) }
func (s byTime) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s byTime) Less(i, j int) bool {
	// Two zero times should return false.
	// Otherwise, zero is "greater" than any other time.
	// (To sort it at the end of the list.)
	if s[i].Next.IsZero() {
		return false
	}
	if s[j].Next.IsZero() {
		return true
	}
	return s[i].Next.Before(s[j].Next)
}

type CronOpt func(cron *Cron)

func WithEtcdErrorsHandler(f func(context.Context, Job, error)) CronOpt {
	return CronOpt(func(cron *Cron) {
		cron.etcdErrorsHandler = f
	})
}

func WithErrorsHandler(f func(context.Context, Job, error)) CronOpt {
	return CronOpt(func(cron *Cron) {
		cron.errorsHandler = f
	})
}

func WithEtcdClient(c *etcdclient.Client) CronOpt {
	return CronOpt(func(cron *Cron) {
		cron.etcdclient = c
	})
}

func WithJobStore(s storage.JobStore) CronOpt {
	return CronOpt(func(cron *Cron) {
		cron.jobStore = s
	})
}

func WithFuncCtx(f func(context.Context, Job) context.Context) CronOpt {
	return CronOpt(func(cron *Cron) {
		cron.funcCtx = f
	})
}

func WithTriggerFunc(f TriggerFunction) CronOpt {
	return CronOpt(func(cron *Cron) {
		cron.triggerFunc = f
	})
}

func WithNamespace(n string) CronOpt {
	return CronOpt(func(cron *Cron) {
		cron.namespace = n
	})
}

func WithPartitioning(p partitioning.Partitioner) CronOpt {
	return CronOpt(func(cron *Cron) {
		cron.partitioning = p
	})
}

// New returns a new Cron job runner.
func New(opts ...CronOpt) (*Cron, error) {
	cron := &Cron{
		pendingOperations: []func(context.Context) *Entry{},
		liveOperation:     make(chan func(context.Context) *Entry),
		entries:           map[string]*Entry{},
		snapshot:          make(chan []*Entry),
		running:           false,
	}
	for _, opt := range opts {
		opt(cron)
	}
	if cron.partitioning == nil {
		cron.partitioning = partitioning.NoPartitioning()
	}
	if cron.etcdclient == nil {
		etcdClient, err := etcdclient.New(etcdclient.Config{
			Endpoints: []string{defaultEtcdEndpoint},
		})
		if err != nil {
			return nil, err
		}
		cron.etcdclient = etcdClient
	}
	if cron.etcdErrorsHandler == nil {
		cron.etcdErrorsHandler = func(ctx context.Context, j Job, err error) {
			log.Printf("[etcd-cron] etcd error when handling '%v' job: %v", j.Name, err)
		}
	}
	if cron.errorsHandler == nil {
		cron.errorsHandler = func(ctx context.Context, j Job, err error) {
			log.Printf("[etcd-cron] error when handling '%v' job: %v", j.Name, err)
		}
	}
	if cron.namespace == "" {
		cron.namespace = defaultNamespace
	}
	cron.organizer = partitioning.NewOrganizer(cron.namespace, cron.partitioning)
	if cron.jobStore == nil {
		cron.jobStore = storage.NewEtcdJobStore(
			cron.etcdclient,
			cron.organizer,
			cron.partitioning,
			func(ctx context.Context, r *storage.JobRecord) error {
				return cron.scheduleJob(jobFromJobRecord(r))
			},
			func(ctx context.Context, s string) error {
				return cron.onJobDeleted(ctx, s)
			})
	}

	cron.collector = collector.New(time.Hour, time.Minute)
	return cron, nil
}

// AddJob adds a Job.
func (c *Cron) AddJob(ctx context.Context, job Job) error {
	if c.jobStore == nil {
		return fmt.Errorf("cannot persist job: no job store configured")
	}
	record, opts := job.toJobRecord()
	return c.jobStore.Put(ctx, record, opts)
}

// DeleteJob removes a job.
func (c *Cron) DeleteJob(ctx context.Context, jobName string) error {
	if c.jobStore == nil {
		return fmt.Errorf("cannot delete job: no job store configured")
	}
	return c.jobStore.Delete(ctx, jobName)
}

func (c *Cron) onJobDeleted(ctx context.Context, jobName string) error {
	c.killJob(jobName)
	// Best effort to delete the counter.
	partitionId := c.partitioning.CalculatePartitionId(jobName)
	counterKey := c.organizer.CounterPath(partitionId, jobName)
	counter := counting.NewEtcdCounter(c.etcdclient, counterKey, 0, time.Duration(0))
	err := counter.Delete(ctx)
	if err != nil {
		c.errorsHandler(ctx, Job{Name: jobName}, err)
	}
	// Ignore error as it is a best effort.
	return nil
}

func (c *Cron) killJob(name string) {
	c.appendOperation(func(ctx context.Context) *Entry {
		_, ok := c.entries[name]
		if !ok {
			return nil
		}

		delete(c.entries, name)
		return nil
	})
}

// GetJob retrieves a job by name.
func (c *Cron) GetJob(jobName string) *Job {
	c.entriesMutex.RLock()
	defer c.entriesMutex.RUnlock()

	entry, ok := c.entries[jobName]
	if !ok || (entry == nil) {
		return nil
	}

	return &entry.Job
}

// Schedule adds a Job to the Cron to be run on the given schedule.
func (c *Cron) scheduleJob(job *Job) error {
	s, repeats, err := rhythm.Parse(job.Rhythm)
	if err != nil {
		return err
	}

	if (repeats > 0) && (job.Repeats > 0) && (job.Repeats != int32(repeats)) {
		return fmt.Errorf("conflicting number of repeats: %v vs %v", repeats, job.Repeats)
	}

	if repeats > 0 {
		job.Repeats = int32(repeats)
	}

	return c.schedule(s, job)
}

// Schedule adds a Job to the Cron to be run on the given schedule.
func (c *Cron) schedule(schedule rhythm.Schedule, job *Job) error {
	partitionId := c.partitioning.CalculatePartitionId(job.Name)
	if !c.partitioning.CheckPartitionLeader(partitionId) {
		// It means the partitioning changed and persisted jobs are in the wrong partition now.
		return fmt.Errorf("host does not own partition %d", partitionId)
	}

	var counter counting.Counter
	if job.Repeats > 0 {
		counterKey := c.organizer.CounterPath(partitionId, job.Name)
		// Needs to count the number of invocations.
		// Follows the job's TTL (if set).
		counter = counting.NewEtcdCounter(c.etcdclient, counterKey, int(job.Repeats), job.TTL)
	}

	entry := &Entry{
		Schedule:        schedule,
		Job:             *job,
		distMutexPrefix: c.organizer.TicksPath(partitionId) + "/",
		counter:         counter,
	}

	c.appendOperation(func(ctx context.Context) *Entry {
		c.entries[entry.Job.Name] = entry
		return entry
	})
	return nil
}

func (c *Cron) appendOperation(op func(ctx context.Context) *Entry) {
	if !c.running {
		c.pendingOperationsMutex.Lock()
		defer c.pendingOperationsMutex.Unlock()

		c.pendingOperations = append(c.pendingOperations, op)
		return
	}

	c.liveOperation <- op
}

// Entries returns a snapshot of the cron entries.
func (c *Cron) Entries() []*Entry {
	if c.running {
		c.snapshot <- nil
		x := <-c.snapshot
		return x
	}

	c.entriesMutex.RLock()
	defer c.entriesMutex.RUnlock()
	entries := []*Entry{}
	for _, e := range c.entries {
		entries = append(entries, &Entry{
			Schedule: e.Schedule,
			Next:     e.Next,
			Prev:     e.Prev,
			Job:      e.Job,
		})
	}
	return entries
}

// Start the cron scheduler in its own go-routine.
func (c *Cron) Start(ctx context.Context) error {
	err := c.jobStore.Start(ctx)
	if err != nil {
		return err
	}
	c.collector.Start(ctx)
	c.running = true
	c.runWaitingGroup.Add(1)
	go c.run(ctx)
	return nil
}

// Run the scheduler.. this is private just due to the need to synchronize
// access to the 'running' state variable.
func (c *Cron) run(ctx context.Context) {
	localMutexer := locking.NewMutexer(c.collector)
	mutexStore := locking.NewMutexStore(locking.NewDistributedMutexBuilderFunc(c.etcdclient), c.collector)
	// Figure out the next activation times for each entry.
	now := time.Now().Local()

	entries := []*Entry{}

	// Pending operations only matter before running, ignored afterwards.
	c.pendingOperationsMutex.Lock()
	c.entriesMutex.Lock()
	for _, op := range c.pendingOperations {
		newEntry := op(ctx)
		if newEntry != nil {
			newEntry.tick(now)
		}
	}
	for _, e := range c.entries {
		entries = append(entries, e)
	}
	c.entriesMutex.Unlock()
	c.pendingOperations = []func(context.Context) *Entry{}
	c.pendingOperationsMutex.Unlock()

	for {
		sort.Sort(byTime(entries))

		var effective time.Time
		if len(entries) == 0 || entries[0].Next.IsZero() {
			// If there are no entries yet, just sleep - it still handles new entries
			// and stop requests.
			effective = now.AddDate(10, 0, 0)
		} else {
			effective = entries[0].Next
		}

		select {
		case op := <-c.liveOperation:
			c.entriesMutex.Lock()
			newEntry := op(ctx)
			if newEntry != nil {
				newEntry.tick(now)
			}
			entries = []*Entry{}
			for _, e := range c.entries {
				entries = append(entries, e)
			}
			c.entriesMutex.Unlock()

		case now = <-time.After(effective.Sub(now)):
			// Run every entry whose next time was this effective time.
			for _, e := range entries {
				if e.Next != effective {
					break
				}
				e.Prev = e.Next
				e.tick(effective)

				go func(ctx context.Context, e *Entry, next time.Time) {
					if c.funcCtx != nil {
						ctx = c.funcCtx(ctx, e.Job)
					}

					tickLock := e.distMutexPrefix + fmt.Sprintf("%v", effective.Unix())
					m, err := mutexStore.Get(tickLock)
					if err != nil {
						c.etcdErrorsHandler(ctx, e.Job, errors.Wrapf(err, "fail to create etcd mutex for job '%v'", e.Job.Name))
						return
					}

					lockCtx, cancel := context.WithTimeout(ctx, time.Second)
					defer cancel()

					// Local mutex is needed to avoid race condition on reusing the etcd mutex object.
					localMutex := localMutexer.Get(tickLock)
					localMutex.Lock()
					err = m.Lock(lockCtx)
					localMutex.Unlock()
					if err == context.DeadlineExceeded {
						return
					} else if err != nil {
						c.etcdErrorsHandler(ctx, e.Job, errors.Wrapf(err, "fail to lock mutex '%v'", m.Key()))
						return
					}

					result, err := c.triggerFunc(ctx, e.Job.Metadata, e.Job.Payload)
					if err != nil {
						c.errorsHandler(ctx, e.Job, err)
						return
					}

					if result == Delete {
						// Job must be deleted.
						// This is handy if client wants to have a custom logic to decide if job is over.
						// One example, is having a more efficient way to count number of invocations.
						err = c.DeleteJob(ctx, e.Job.Name)
						if err != nil {
							c.errorsHandler(ctx, e.Job, err)
						}

						// No need to check (and delete) a counter since every counter has a TTL.
						return
					}

					if result == OK && e.counter != nil {
						// Needs to check number of triggers
						remaining, updated, err := e.counter.Increment(ctx, -1)
						if err != nil {
							c.errorsHandler(ctx, e.Job, err)
							// No need to abort if updating the count failed.
							// The count solution is not transactional anyway.
						}

						if updated {
							if remaining <= 0 {
								err = c.DeleteJob(ctx, e.Job.Name)
								if err != nil {
									c.errorsHandler(ctx, e.Job, err)
								}
								// Tries to delete the counter here
								err = e.counter.Delete(ctx)
								if err != nil {
									c.errorsHandler(ctx, e.Job, err)
								}
							}
						}
					}
					// Cannot unlock because it can open a chance for double trigger since two instances
					// can have a clock skew and compete for the lock at slight different windows.
					// So, we keep the lock during its ttl
				}(ctx, e, e.Next)
			}
			continue

		case <-c.snapshot:
			c.snapshot <- c.entrySnapshot(entries)

		case <-ctx.Done():
			c.runWaitingGroup.Done()
			return
		}
	}
}

// Wait the cron to stop after context is cancelled.
func (c *Cron) Wait() {
	c.runWaitingGroup.Wait()
	c.jobStore.Wait()
	c.running = false
}

// entrySnapshot returns a copy of the current cron entry list.
func (c *Cron) entrySnapshot(input []*Entry) []*Entry {
	entries := []*Entry{}
	for _, e := range input {
		entries = append(entries, &Entry{
			Schedule: e.Schedule,
			Next:     e.Next,
			Prev:     e.Prev,
			Job:      e.Job,
		})
	}
	return entries
}
