package models

// Backlog represents a collection of tasks
type Backlog struct {
	Tasks []Task `yaml:"tasks"`
}

// NewBacklog creates a new empty backlog
func NewBacklog() *Backlog {
	return &Backlog{
		Tasks: []Task{},
	}
}

// FindTaskByID finds a task by ID in the backlog
func (b *Backlog) FindTaskByID(id string) *Task {
	for i := range b.Tasks {
		if b.Tasks[i].ID == id {
			return &b.Tasks[i]
		}
	}
	return nil
}

// AddTask adds a task to the backlog
func (b *Backlog) AddTask(task Task) {
	b.Tasks = append(b.Tasks, task)
}

// UpdateTask updates an existing task in the backlog
func (b *Backlog) UpdateTask(task Task) bool {
	for i := range b.Tasks {
		if b.Tasks[i].ID == task.ID {
			b.Tasks[i] = task
			return true
		}
	}
	return false
}

// RemoveTask removes a task from the backlog by ID
func (b *Backlog) RemoveTask(id string) bool {
	for i := range b.Tasks {
		if b.Tasks[i].ID == id {
			b.Tasks = append(b.Tasks[:i], b.Tasks[i+1:]...)
			return true
		}
	}
	return false
}
