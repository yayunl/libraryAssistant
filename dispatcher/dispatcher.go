package isbnAPI

import (
	"isbnAPI/task"
	"sync"
)

type taskChannel chan *task.Task

type (
	// Dispatcher represents a management workers.
	Dispatcher struct {
		workerChan chan *worker
		taskChan   taskChannel
		workers    []*worker
		wg         sync.WaitGroup
		done       chan struct{}
		resultChan chan *result
	}

	// worker represents the worker that executes the job.
	worker struct {
		dispatcher *Dispatcher
		data       taskChannel
		done       chan struct{}
		id         int
	}

	result struct {
		// WorkerID is the ID of the worker that executed the task
		WorkerID int

		// Value is the result of the task
		Value interface{}
		// Err is the error of the task
		Err error
	}
)

const (
	maxQueues = 10000
)

// NewDispatcher  returns a pointer of Dispatcher.
func NewDispatcher(maxWorkers int) *Dispatcher {
	d := &Dispatcher{
		workerChan: make(chan *worker, maxWorkers),
		taskChan:   make(chan *task.Task, maxQueues),
		done:       make(chan struct{}),
		resultChan: make(chan *result),
	}
	d.workers = make([]*worker, cap(d.workerChan))
	for i := 0; i < cap(d.workerChan); i++ {
		w := worker{
			dispatcher: d,
			data:       make(taskChannel),
			done:       make(chan struct{}),
			id:         i,
		}
		d.workers[i] = &w
	}
	return d
}

// Add adds a given value to the taskChan of the dispatcher.
func (d *Dispatcher) Add(task *task.Task) {
	d.wg.Add(1)
	d.taskChan <- task
}

// Run starts the specified dispatcher but does not wait for it to complete.
func (d *Dispatcher) Run() {

	for _, w := range d.workers {
		w.start()
	}
	go func() {
		//defer fmt.Printf("Dispatcher done\n")
		for {
			select {
			case t := <-d.taskChan:
				(<-d.workerChan).data <- t

			case <-d.done:
				return
			}
		}
	}()
	d.Wait()
	close(d.resultChan)
}

// Wait waits for the dispatcher to exit. It must have been started by Start.
func (d *Dispatcher) Wait() {
	d.wg.Wait()
}

// Stop stops the dispatcher to execute. The dispatcher stops gracefully
// if the given boolean is false.
func (d *Dispatcher) Stop() {
	close(d.done)
	for _, w := range d.workers {
		close(w.done)
	}
}

func (d *Dispatcher) GetResults() <-chan *result {
	return d.resultChan
}

func (w *worker) start() {
	go func() {
		//defer fmt.Printf("Worker %d done\n", w.id)
		for {
			// register the current worker into the dispatch workerChan
			w.dispatcher.workerChan <- w

			select {
			case t := <-w.data:
				// Process the work
				resp, err := t.Execute()
				res := &result{
					Value:    resp,
					Err:      err,
					WorkerID: w.id,
				}
				select {
				case w.dispatcher.resultChan <- res:
				}

				// If the result contains certain chars. This is only for workers to create new tasks
				//if strings.Contains(resp.(string), "worker0") {
				//	// Create a new task
				//	oldUrl := t.Args[1]
				//	newUrl := fmt.Sprintf("%s/newTask%d", oldUrl, w.id)
				//	delayTime := t.ID * 100
				//	newTask := task.Task{
				//		ID:   t.ID * 10,
				//		Fn:   ExecuteFnc,
				//		Args: []interface{}{delayTime, newUrl, w.id},
				//	}
				//	w.dispatcher.Add(&newTask)
				//}
				w.dispatcher.wg.Done()

			case <-w.done:
				return
			}
		}
	}()
}
