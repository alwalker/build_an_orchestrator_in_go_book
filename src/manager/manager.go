package manager

import (
	"bytes"
	"cube/node"
	"cube/scheduler"
	"cube/store"
	"cube/task"
	"cube/worker"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	nettypes "github.com/containers/common/libnetwork/types"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
)

type Manager struct {
	Pending       queue.Queue
	TaskDb        store.Store
	EventDb       store.Store
	Workers       []string // hostname:port
	WorkerTaskMap map[string][]uuid.UUID
	TaskWorkerMap map[uuid.UUID]string
	LastWorker    int
	WorkerNodes   []*node.Node
	Scheduler     scheduler.Scheduler
}

func New(workers []string, schedulerType string, dbType string) *Manager {
	workerTaskMap := make(map[string][]uuid.UUID)
	taskWorkerMap := make(map[uuid.UUID]string)
	var nodes []*node.Node

	for worker := range workers {
		workerTaskMap[workers[worker]] = []uuid.UUID{}
		nAPI := fmt.Sprintf("http://%v", workers[worker])
		n := node.NewNode(workers[worker], nAPI, "worker")
		nodes = append(nodes, n)
	}

	var s scheduler.Scheduler
	switch schedulerType {
	case "roundrobin":
		s = &scheduler.RoundRobin{Name: "roundrobin"}
	case "epvm":
		s = &scheduler.Epvm{Name: "epvm"}
	default:
		s = &scheduler.RoundRobin{Name: "roundrobin"}
	}

	m := Manager{
		Pending:       *queue.New(),
		Workers:       workers,
		WorkerTaskMap: workerTaskMap,
		TaskWorkerMap: taskWorkerMap,
		WorkerNodes:   nodes,
		Scheduler:     s,
	}

	var ts, es store.Store
	switch dbType {
	case "memory":
		ts = store.NewInMemoryTaskStore()
		es = store.NewInMemoryTaskEventStore()
	case "persistent":
		ts, _ = store.NewTaskStore("tasks.db", 0600, "tasks")
		es, _ = store.NewEventStore("events.db", 0600, "events")
	}
	m.TaskDb = ts
	m.EventDb = es

	return &m
}

func (m *Manager) GetTasks() []*task.Task {
	taskList, err := m.TaskDb.List()
	if err != nil {
		log.Printf("error getting list of tasks: %v\n", err)
		return nil
	}

	return taskList.([]*task.Task)
}

func (m *Manager) AddTask(te task.TaskEvent) {
	m.Pending.Enqueue(te)
}

func (m *Manager) SelectWorker(t task.Task) (*node.Node, error) {
	candidates := m.Scheduler.SelectCandidateNodes(t, m.WorkerNodes)
	if candidates == nil {
		msg := fmt.Sprintf("No available candidates match resource request for task %v", t.ID)
		err := errors.New(msg)
		return nil, err
	}

	scores := m.Scheduler.Score(t, candidates)

	selectedNode := m.Scheduler.Pick(scores, candidates)

	return selectedNode, nil
}

func (m *Manager) UpdateTasks() {
	for {
		log.Println("Checking for task updates from workers")
		m.updateTasks()
		log.Println("Task updates completed")
		log.Println("Sleeping for 15 seconds")
		time.Sleep(15 * time.Second)
	}
}

func (m *Manager) ProcessTasks() {
	for {
		log.Println("Processing any tasks in the queue")
		m.SendWork()
		log.Println("Sleeping for 10 seconds")
		time.Sleep(10 * time.Second)
	}
}

func (m *Manager) SendWork() {
	if m.Pending.Len() > 0 {
		e := m.Pending.Dequeue()
		te := e.(task.TaskEvent)
		err := m.EventDb.Put(te.ID.String(), &te)
		if err != nil {
			log.Printf("error attempting to store task event %s: %s\n", te.ID.String(), err)
			return
		}
		log.Printf("Pulled %v off pending queue", te)

		taskWorker, ok := m.TaskWorkerMap[te.Task.ID]
		if ok {
			result, err := m.TaskDb.Get(te.Task.ID.String())
			if err != nil {
				log.Printf("unable to schedule task: %s\n", err)
				return
			}

			persistedTask, ok := result.(*task.Task)
			if !ok {
				log.Printf("unable to convert task to task.Task type\n")
				return
			}

			if te.State == task.Completed && task.ValidStateTransition(persistedTask.State, te.State) {
				m.stopTask(taskWorker, te.Task.ID.String())
				return
			}

			log.Printf("invalid request: existing task %s is in state %v and cannot transition to the completed state\n", persistedTask.ID.String(), persistedTask.State)
			return
		}

		t := te.Task
		w, err := m.SelectWorker(t)
		if err != nil {
			log.Printf("error selecting worker for task %s: %v\n", t.ID, err)
		}

		t.State = task.Scheduled
		err = m.TaskDb.Put(t.ID.String(), &t)
		if err != nil {
			log.Printf("Error updating task %s in database: %v", t.ID.String(), err)
		}
		data, err := json.Marshal(te)
		if err != nil {
			log.Printf("Unable to marshal task object: %v.\n", t)
		}

		m.WorkerTaskMap[w.Name] = append(m.WorkerTaskMap[w.Name], te.Task.ID)
		m.TaskWorkerMap[t.ID] = w.Name
		url := fmt.Sprintf("http://%s/tasks", w.Name)
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
		if err != nil {
			log.Printf("Error connecting to %v: %v\n", w.Name, err)
			m.Pending.Enqueue(te)
			return
		}

		d := json.NewDecoder(resp.Body)
		if resp.StatusCode != http.StatusCreated {
			e := worker.ErrResponse{}
			err := d.Decode(&e)
			if err != nil {
				fmt.Printf("Error decoding response: %s\n", err.Error())
				return
			}
			log.Printf("Response error (%d): %s", e.HTTPStatusCode, e.Message)
			return
		}

		t = task.Task{}
		err = d.Decode(&t)
		if err != nil {
			fmt.Printf("Error decoding response: %s\n", err.Error())
			return
		}
		log.Printf("%#v\n", t)
	} else {
		log.Println("No work in the queue")
	}
}

func (m *Manager) DoHealthChecks() {
	for {
		log.Println("Performing task health check")
		m.doHealthChecks()

		log.Println("Task health checks completed")
		log.Println("Sleeping for 60 seconds")
		time.Sleep(60 * time.Second)
	}
}

func (m *Manager) updateTasks() {
	for _, worker := range m.Workers {
		log.Printf("Checking worker %v for task updates", worker)
		url := fmt.Sprintf("http://%s/tasks", worker)
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("Error connecting to %v: %v\n", worker, err)
		}
		if resp.StatusCode != http.StatusOK {
			log.Printf("Error sending request: %v\n", err)
		}
		d := json.NewDecoder(resp.Body)

		var tasks []*task.Task
		err = d.Decode(&tasks)
		if err != nil {
			log.Printf("Error unmarshalling tasks: %s\n", err.Error())
		}

		for _, t := range tasks {
			log.Printf("Attempting to update task %v\n", t.ID)

			result, err := m.TaskDb.Get(t.ID.String())
			if err != nil {
				log.Printf("[manager] %s\n", err)
				continue
			}
			taskPersisted, ok := result.(*task.Task)
			if !ok {
				log.Printf("cannot convert result %v to task.Task type\n",
					result)
				continue
			}

			if taskPersisted.State != t.State {
				taskPersisted.State = t.State
			}
			taskPersisted.StartTime = t.StartTime
			taskPersisted.FinishTime = t.FinishTime
			taskPersisted.ContainerID = t.ContainerID
			taskPersisted.HostPorts = t.HostPorts

			err = m.TaskDb.Put(taskPersisted.ID.String(), taskPersisted)
			if err != nil {
				log.Printf("Error updating task %s in database: %v", taskPersisted.ID.String(), err)
			}
		}
	}
}

func (m *Manager) checkTaskHealth(t task.Task) error {
	log.Printf("Calling health check for task %s: %s\n", t.ID, t.HealthCheck)

	w := m.TaskWorkerMap[t.ID]
	hostPort := getHostPort(t.ExposedPorts)
	worker := strings.Split(w, ":")
	url := fmt.Sprintf("http://%s:%s%s", worker[0], *hostPort, t.HealthCheck)

	log.Printf("Calling health check for task %s:%s\n", t.ID, url)
	// resp, err := http.Get(url)
	// if err != nil {
	// 	msg := fmt.Sprintf("Error connecting to health check %s", url)
	// 	log.Println(msg)
	// 	return errors.New(msg)
	// }
	// if resp.StatusCode != http.StatusOK {
	// 	msg := fmt.Sprintf("Error health check for task %s did not return 200\n", t.ID)
	// 	log.Println(msg)
	// 	return errors.New(msg)
	// }
	log.Printf("Task %s health check response: %v\n", t.ID, 200)

	return nil
}

func (m *Manager) doHealthChecks() {
	for _, t := range m.GetTasks() {
		if t.State == task.Running && t.RestartCount < 3 {
			err := m.checkTaskHealth(*t)
			if err != nil {
				if t.RestartCount < 3 {
					m.restartTask(t)
				}
			}
		} else if t.State == task.Failed && t.RestartCount < 3 {
			m.restartTask(t)
		}
	}
}

func (m *Manager) stopTask(worker string, taskID string) {
	client := &http.Client{}
	url := fmt.Sprintf("http://%s/tasks/%s", worker, taskID)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		log.Printf("error creating request to delete task %s: %v\n", taskID, err)
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("error connecting to worker at %s: %v\n", url, err)
		return
	}
	if resp.StatusCode != 204 {
		log.Printf("Error sending request: %v\n", err)
		return
	}

	log.Printf("task %s has been scheduled to be stopped", taskID)
}

func (m *Manager) restartTask(t *task.Task) {
	w := m.TaskWorkerMap[t.ID]
	t.State = task.Scheduled
	t.RestartCount++
	err := m.TaskDb.Put(t.ID.String(), t)
	if err != nil {
		log.Printf("Error updating task %s in database: %v", t.ID.String(), err)
	}
	te := task.TaskEvent{
		ID:        uuid.New(),
		State:     task.Running,
		Timestamp: time.Now(),
		Task:      *t,
	}

	data, err := json.Marshal(te)
	if err != nil {
		log.Printf("Unable to marshal task object: %v.", t)
		return
	}
	url := fmt.Sprintf("http://%s/tasks", w)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Error connecting to %v: %v", w, err)
		m.Pending.Enqueue(t)
		return
	}

	d := json.NewDecoder(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		e := worker.ErrResponse{}
		err := d.Decode(&e)
		if err != nil {
			fmt.Printf("Error decoding response: %s\n", err.Error())
			return
		}
		log.Printf("Response error (%d): %s", e.HTTPStatusCode, e.Message)
		return
	}

	newTask := task.Task{}
	err = d.Decode(&newTask)
	if err != nil {
		fmt.Printf("Error decoding response: %s\n", err.Error())
		return
	}
	log.Printf("%#v\n", t)
}

func getHostPort(ports []nettypes.PortMapping) *string {
	for _, k := range ports {
		p := k.HostPort
		sp := fmt.Sprintf("%d", p)
		return &sp
	}

	log.Print("No ports found in portmap")
	noPort := ""
	return &noPort
}
