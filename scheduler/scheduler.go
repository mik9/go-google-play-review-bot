package scheduler

import (
	"sync"
	"time"
)

type job struct {
	closeChan chan int
}

type Scheduler struct {
	jobs     []job
	jobsLock sync.Mutex
}

func NewScheduler() *Scheduler {
	return &Scheduler{}
}

func (s *Scheduler) Schedule(jobFunc func(), every time.Duration) {
	waitChan := make(chan int)
	closeChan := make(chan int)

	job := job{
		closeChan: closeChan,
	}

	s.jobsLock.Lock()
	s.jobs = append(s.jobs, job)
	s.jobsLock.Unlock()

	go func() {
		for {
			select {
			case <-waitChan:
				jobFunc()
				wait(waitChan, every)
			case <-closeChan:
				return
			}
		}
	}()
	waitChan <- 0
}

func wait(waitChan chan int, duration time.Duration) {
	go func() {
		time.Sleep(duration)
		waitChan <- 0
	}()
}

func (s *Scheduler) Clear() {
	s.jobsLock.Lock()
	for _, job := range s.jobs {
		job.closeChan <- 0
	}
	s.jobs = []job{}
	s.jobsLock.Unlock()
}
