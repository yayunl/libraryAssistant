package task

type taskFnc func(args []any) (interface{}, error)

// Result is the result of a task
type Result struct {
	// WorkerID is the ID of the worker that executed the task
	WorkerID int
	// TaskID is the ID of the task
	TaskID interface{}
	// Value is the result of the task
	Value interface{}
	// Err is the error of the task
	Err error
}

// Task is a data structure that represents a task
type Task struct {
	ID int
	// Fn is the function to be executed
	Fn taskFnc
	// Args is the arguments of the task
	Args []any
}

// New creates a new task
func New(id int, fn taskFnc, args []any) *Task {
	return &Task{
		ID:   id,
		Fn:   fn,
		Args: args,
	}
}

// Execute lets the task be executed in a worker
func (t *Task) Execute() (interface{}, error) {
	return t.Fn(t.Args)
}
