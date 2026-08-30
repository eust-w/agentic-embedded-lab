package fmi

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
)

type ValueType byte

const (
	Real    ValueType = 'r'
	Integer ValueType = 'i'
	Boolean ValueType = 'b'
)

type Variable struct {
	Reference uint32
	Name      string
	Type      ValueType
	Direction string
}
type Instance struct {
	Adapter    ael.Adapter
	Variables  map[uint32]Variable
	mu         sync.Mutex
	lastTimeUS int64
}
type Bridge struct {
	SocketPath string
	Instances  map[string]*Instance
}

func (b *Bridge) Listen(ctx context.Context) error {
	if b.SocketPath == "" || len(b.Instances) == 0 {
		return errors.New("FMI socket and instances are required")
	}
	if err := os.MkdirAll(filepath.Dir(b.SocketPath), 0o700); err != nil {
		return err
	}
	_ = os.Remove(b.SocketPath)
	listener, err := net.Listen("unix", b.SocketPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(b.SocketPath)
	if err := os.Chmod(b.SocketPath, 0o600); err != nil {
		return err
	}
	return b.Serve(ctx, listener)
}

func (b *Bridge) Serve(ctx context.Context, listener net.Listener) error {
	if listener == nil || len(b.Instances) == 0 {
		return errors.New("FMI listener and instances are required")
	}
	go func() { <-ctx.Done(); _ = listener.Close() }()
	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go b.handle(ctx, connection)
	}
}
func (b *Bridge) handle(ctx context.Context, connection net.Conn) {
	defer connection.Close()
	_ = connection.SetReadDeadline(time.Now().Add(10 * time.Second))
	line, err := bufio.NewReader(connection).ReadString('\n')
	if err != nil {
		return
	}
	response, err := b.Exchange(ctx, strings.TrimSpace(line))
	if err != nil {
		_, _ = fmt.Fprintf(connection, "ERROR %s\n", sanitize(err.Error()))
		return
	}
	_, _ = fmt.Fprintf(connection, "%s\n", response)
}
func (b *Bridge) Exchange(ctx context.Context, request string) (string, error) {
	fields := strings.Fields(request)
	if len(fields) < 4 || fields[0] != "STEP" {
		return "", errors.New("invalid FMI step request")
	}
	instance := b.Instances[fields[1]]
	if instance == nil {
		var match *Instance
		for name, candidate := range b.Instances {
			if strings.HasSuffix(fields[1], "."+name) || strings.HasSuffix(fields[1], "/"+name) {
				if match != nil {
					return "", errors.New("ambiguous FMI instance")
				}
				match = candidate
			}
		}
		instance = match
	}
	if instance == nil || instance.Adapter == nil {
		return "", errors.New("unknown FMI instance")
	}
	current, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return "", err
	}
	step, err := strconv.ParseFloat(fields[3], 64)
	if err != nil || step <= 0 {
		return "", errors.New("invalid FMI step")
	}
	currentUS := int64(math.Round(current * 1e6))
	stepUS := int64(math.Round(step * 1e6))
	instance.mu.Lock()
	defer instance.mu.Unlock()
	if currentUS < instance.lastTimeUS {
		return "", errors.New("FMI time moved backwards")
	}
	for _, field := range fields[4:] {
		kind, reference, value, err := parseValue(field)
		if err != nil {
			return "", err
		}
		variable, ok := instance.Variables[reference]
		if !ok || variable.Type != kind {
			return "", fmt.Errorf("invalid FMI input reference %d", reference)
		}
		if variable.Direction != "input" {
			continue
		}
		events, err := instance.Adapter.Inject(ctx, variable.Name, value, currentUS)
		_ = events
		if err != nil {
			return "", err
		}
	}
	result, err := instance.Adapter.Step(ctx, currentUS, stepUS)
	if err != nil {
		return "", err
	}
	instance.lastTimeUS = currentUS + stepUS
	parts := []string{"OK"}
	references := make([]int, 0, len(instance.Variables))
	for reference, variable := range instance.Variables {
		if variable.Direction == "output" {
			references = append(references, int(reference))
		}
	}
	sortInts(references)
	for _, raw := range references {
		reference := uint32(raw)
		variable := instance.Variables[reference]
		value, ok := result.Outputs[variable.Name]
		if !ok {
			value, ok = result.Metrics[variable.Name]
		}
		if !ok {
			continue
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return "", fmt.Errorf("FMI output %s is non-finite", variable.Name)
		}
		switch variable.Type {
		case Real:
			parts = append(parts, fmt.Sprintf("r%d=%.17g", reference, value))
		case Integer:
			parts = append(parts, fmt.Sprintf("i%d=%d", reference, int64(math.Round(value))))
		case Boolean:
			boolean := 0
			if value != 0 {
				boolean = 1
			}
			parts = append(parts, fmt.Sprintf("b%d=%d", reference, boolean))
		}
	}
	return strings.Join(parts, " "), nil
}
func parseValue(field string) (ValueType, uint32, any, error) {
	if len(field) < 4 {
		return 0, 0, nil, errors.New("invalid FMI value")
	}
	kind := ValueType(field[0])
	delimiter := strings.IndexByte(field, '=')
	if delimiter < 2 {
		return 0, 0, nil, errors.New("invalid FMI value")
	}
	reference64, err := strconv.ParseUint(field[1:delimiter], 10, 32)
	if err != nil {
		return 0, 0, nil, err
	}
	raw := field[delimiter+1:]
	switch kind {
	case Real:
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, 0, nil, errors.New("invalid FMI real")
		}
		return kind, uint32(reference64), value, nil
	case Integer:
		value, err := strconv.ParseInt(raw, 10, 64)
		return kind, uint32(reference64), value, err
	case Boolean:
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > 1 {
			return 0, 0, nil, errors.New("invalid FMI boolean")
		}
		return kind, uint32(reference64), value == 1, nil
	default:
		return 0, 0, nil, errors.New("unsupported FMI value type")
	}
}
func sanitize(value string) string { return strings.NewReplacer("\n", " ", "\r", " ").Replace(value) }
func sortInts(values []int) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
