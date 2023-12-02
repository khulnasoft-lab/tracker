package integration

import (
	"context"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/khulnasoft-lab/libbpfgo/helpers"

	"github.com/khulnasoft-lab/tracker/pkg/cmd/initialize"
	"github.com/khulnasoft-lab/tracker/pkg/config"
	tracker "github.com/khulnasoft-lab/tracker/pkg/ebpf"
	"github.com/khulnasoft-lab/tracker/pkg/proctree"
	uproc "github.com/khulnasoft-lab/tracker/pkg/utils/proc"
	"github.com/khulnasoft-lab/tracker/types/trace"
)

// eventBuffer is a thread-safe buffer for tracker events
type eventBuffer struct {
	mu     sync.RWMutex
	events []trace.Event
}

// addEvent adds an event to the eventBuffer
func (b *eventBuffer) addEvent(evt trace.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.events = append(b.events, evt)
}

// clear clears the eventBuffer
func (b *eventBuffer) clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.events = b.events[:0]
}

// len returns the number of events in the eventBuffer
func (b *eventBuffer) len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return len(b.events)
}

// load tracker into memory with args
func startTracker(ctx context.Context, t *testing.T, cfg config.Config, output *config.OutputConfig, capture *config.CaptureConfig) *tracker.Tracker {
	initialize.SetLibbpfgoCallbacks()

	kernelConfig, err := initialize.KernelConfig()
	require.NoError(t, err)

	cfg.KernelConfig = kernelConfig

	osInfo, err := helpers.GetOSInfo()
	require.NoError(t, err)

	err = initialize.BpfObject(&cfg, kernelConfig, osInfo, "/tmp/tracker", "")
	require.NoError(t, err)

	if capture == nil {
		capture = prepareCapture()
	}

	cfg.Capture = capture

	cfg.PerfBufferSize = 1024
	cfg.BlobPerfBufferSize = 1024

	// No process tree in the integration tests
	cfg.ProcTree = proctree.ProcTreeConfig{
		Source: proctree.SourceNone,
	}

	errChan := make(chan error)

	go func() {
		for {
			select {
			case err, ok := <-errChan:
				if !ok {
					return
				}
				t.Logf("received error while testing: %s\n", err)
			case <-ctx.Done():
				return
			}
		}
	}()

	if output == nil {
		output = &config.OutputConfig{}
	}

	cfg.Output = output
	cfg.NoContainersEnrich = true

	trc, err := tracker.New(cfg)
	require.NoError(t, err)

	err = trc.Init(ctx)
	require.NoError(t, err)

	t.Logf("started tracker...\n")
	go func() {
		err := trc.Run(ctx)
		require.NoError(t, err, "tracker run failed")
	}()

	return trc
}

// prepareCapture prepares a capture config for tracker
func prepareCapture() *config.CaptureConfig {
	// taken from tracker-rule github project, might have to adjust...
	// prepareCapture is called with nil input
	return &config.CaptureConfig{
		FileWrite: config.FileCaptureConfig{
			PathFilter: []string{},
		},
		OutputPath: filepath.Join("/tmp/tracker", "out"),
	}
}

// wait for tracker to start (or timeout)
// in case of timeout, the test will fail
func waitForTrackerStart(t *testing.T, trc *tracker.Tracker) {
	const checkTimeout = 10 * time.Second
	ticker := time.NewTicker(100 * time.Millisecond)

	for {
		select {
		case <-ticker.C:
			if trc.Running() {
				return
			}
		case <-time.After(checkTimeout):
			t.Logf("timed out on running tracker\n")
			t.FailNow()
		}
	}
}

// wait for tracker to stop (or timeout)
// in case of timeout, the test will continue since all tests already passed
func waitForTrackerStop(t *testing.T, trc *tracker.Tracker) {
	const checkTimeout = 10 * time.Second
	ticker := time.NewTicker(100 * time.Millisecond)

	for {
		select {
		case <-ticker.C:
			if !trc.Running() {
				t.Logf("stopped tracker\n")
				return
			}
		case <-time.After(checkTimeout):
			t.Logf("timed out on stopping tracker\n")
			return
		}
	}
}

// wait for tracker buffer to fill up with expected number of events (or timeout)
// in case of timeout, the test will fail
func waitForTrackerOutputEvents(t *testing.T, actual *eventBuffer, now time.Time, expectedEvts int, failOnTimeout bool) {
	const checkTimeout = 5 * time.Second
	ticker := time.NewTicker(100 * time.Millisecond)

	for {
		select {
		case <-ticker.C:
			if actual.len() >= expectedEvts {
				return
			}
		case <-time.After(checkTimeout):
			if failOnTimeout {
				t.Logf("timed out on output\n")
				t.FailNow()
			}
			return
		}
	}
}

// assureIsRoot skips the test if it is not run as root
func assureIsRoot(t *testing.T) {
	if syscall.Geteuid() != 0 {
		t.Skipf("***** %s must be run as ROOT *****", t.Name())
	}
}

func getProcNS(nsName string) string {
	pid := syscall.Getpid()
	nsID, err := uproc.GetProcNS(uint(pid), nsName)
	if err != nil {
		panic(err)
	}

	return strconv.Itoa(nsID)
}
