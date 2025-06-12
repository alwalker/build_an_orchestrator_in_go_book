package task

import (
	"log"
	"strings"
	"time"

	"context"

	nettypes "github.com/containers/common/libnetwork/types"
	"github.com/containers/podman/v5/libpod/define"
	"github.com/containers/podman/v5/pkg/bindings"
	"github.com/containers/podman/v5/pkg/bindings/containers"
	"github.com/containers/podman/v5/pkg/bindings/images"
	"github.com/containers/podman/v5/pkg/specgen"
	"github.com/google/uuid"
	// "github.com/opencontainers/runtime-spec/specs-go"
)

type Task struct {
	ID            uuid.UUID
	ContainerID   string
	Name          string
	State         State
	Image         string
	Cpu           uint64
	Memory        int64
	Disk          int64
	ExposedPorts  []nettypes.PortMapping
	HostPorts     map[string][]define.InspectHostPort
	PortBindings  map[string]string
	RestartPolicy string
	StartTime     time.Time
	FinishTime    time.Time
	HealthCheck   string
	RestartCount  int
}

type TaskEvent struct {
	ID        uuid.UUID
	State     State
	Timestamp time.Time
	Task      Task
}

// Config struct to hold Podman container config
type Config struct {
	// Name of the task, also used as the container name
	Name string
	// AttachStdin boolean which determines if stdin should be attached
	AttachStdin bool
	// AttachStdout boolean which determines if stdout should be attached
	AttachStdout bool
	// AttachStderr boolean which determines if stderr should be attached
	AttachStderr bool
	// ExposedPorts list of ports exposed
	ExposedPorts []nettypes.PortMapping
	// Cmd to be run inside container (optional)
	Cmd []string
	// Image used to run the container
	Image string
	// Cpu in shares (request)
	Cpu uint64
	// Memory in MiB
	Memory int64
	// Disk in GiB
	Disk int64
	// Env variables
	Env map[string]string
	// RestartPolicy for the container ["", "always", "unless-stopped", "on-failure"]
	RestartPolicy string
}

func NewConfig(t *Task) *Config {
	return &Config{
		Name:          t.Name,
		ExposedPorts:  t.ExposedPorts,
		Image:         t.Image,
		Cpu:           t.Cpu,
		Memory:        t.Memory,
		Disk:          t.Disk,
		RestartPolicy: t.RestartPolicy,
	}
}


type Podman struct {
	Conn   context.Context
	Config Config
}

type PodmanInspectResponse struct {
	Error     error
	Container *define.InspectContainerData
}

func (p *Podman) Inspect(containerID string) PodmanInspectResponse {

	resp, err := containers.Inspect(p.Conn, p.Config.Name, nil)
	if err != nil {
		log.Printf("Error inspecting container: %s\n", err)
		return PodmanInspectResponse{Error: err}
	}

	return PodmanInspectResponse{Container: resp}
}

func NewPodman(c *Config) (*Podman, error) {
	conn, err := bindings.NewConnection(context.Background(), "unix:///run/user/1000/podman/podman.sock")
	if err != nil {
		log.Printf("Error creating Podman connection: %s\n", err)
		return nil, err
	}

	return &Podman{Conn: conn, Config: *c}, nil
}

type ContainerResult struct {
	Error       error
	Action      string
	ContainerId string
	Result      string
}

func (p *Podman) Run() ContainerResult {
	irp, err := images.Pull(p.Conn, p.Config.Image, nil)
	if err != nil {
		log.Printf("Error pulling image %s: %v\n", p.Config.Image, err)
		return ContainerResult{Error: err}
	}
	log.Printf("%s", strings.Join(irp, "\n"))

	// mib := p.Config.Memory * 1024 * 1024

	s := specgen.NewSpecGenerator(p.Config.Image, false)
	s.RestartPolicy = p.Config.RestartPolicy
	// s.ResourceLimits = &specs.LinuxResources{Memory: &specs.LinuxMemory{Limit: &mib}, CPU: &specs.LinuxCPU{Shares: &p.Config.Cpu}}
	s.Name = p.Config.Name
	s.Env = p.Config.Env
	s.PortMappings = p.Config.ExposedPorts

	createResponse, err := containers.CreateWithSpec(p.Conn, s, nil)
	if err != nil {
		log.Printf("Error creating container %s: %v", p.Config.Name, err)
		return ContainerResult{Error: err}
	}
	log.Printf("Container %s:%s created", p.Config.Name, createResponse.ID)

	if err := containers.Start(p.Conn, createResponse.ID, nil); err != nil {
		log.Printf("Error starting container %s:%s -> %v", p.Config.Name, createResponse.ID, err)
		return ContainerResult{Error: err}
	}
	log.Printf("Container %s:%s started", p.Config.Name, createResponse.ID)

	return ContainerResult{ContainerId: createResponse.ID, Action: "start", Result: "success"}
}

func (p *Podman) Stop(id string) ContainerResult {
	log.Printf("Attempting to stop container %v", id)
	err := containers.Stop(p.Conn, id, nil)
	if err != nil {
		log.Printf("Error stopping container %s: %v\n", id, err)
		return ContainerResult{Error: err}
	}

	_, err = containers.Remove(p.Conn, id, nil)
	if err != nil {
		log.Printf("Error removing container %s: %v\n", id, err)
		return ContainerResult{Error: err}
	}

	return ContainerResult{Action: "stop", Result: "success", Error: nil}
}
