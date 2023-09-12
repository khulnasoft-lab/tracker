package ebpf

import (
	gocontext "context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/sys/unix"
	"kernel.org/pub/linux/libs/security/libcap/cap"

	bpf "github.com/khulnasoft-lab/libbpfgo"
	"github.com/khulnasoft-lab/libbpfgo/helpers"

	"github.com/khulnasoft-lab/tracker/pkg/bucketscache"
	"github.com/khulnasoft-lab/tracker/pkg/bufferdecoder"
	"github.com/khulnasoft-lab/tracker/pkg/capabilities"
	"github.com/khulnasoft-lab/tracker/pkg/cgroup"
	"github.com/khulnasoft-lab/tracker/pkg/config"
	"github.com/khulnasoft-lab/tracker/pkg/containers"
	"github.com/khulnasoft-lab/tracker/pkg/ebpf/controlplane"
	"github.com/khulnasoft-lab/tracker/pkg/ebpf/initialization"
	"github.com/khulnasoft-lab/tracker/pkg/ebpf/probes"
	"github.com/khulnasoft-lab/tracker/pkg/errfmt"
	"github.com/khulnasoft-lab/tracker/pkg/events"
	"github.com/khulnasoft-lab/tracker/pkg/events/derive"
	"github.com/khulnasoft-lab/tracker/pkg/events/sorting"
	"github.com/khulnasoft-lab/tracker/pkg/events/trigger"
	"github.com/khulnasoft-lab/tracker/pkg/filters"
	"github.com/khulnasoft-lab/tracker/pkg/logger"
	"github.com/khulnasoft-lab/tracker/pkg/metrics"
	"github.com/khulnasoft-lab/tracker/pkg/pcaps"
	"github.com/khulnasoft-lab/tracker/pkg/policy"
	"github.com/khulnasoft-lab/tracker/pkg/signatures/engine"
	"github.com/khulnasoft-lab/tracker/pkg/streams"
	"github.com/khulnasoft-lab/tracker/pkg/utils"
	"github.com/khulnasoft-lab/tracker/pkg/utils/proc"
	"github.com/khulnasoft-lab/tracker/pkg/utils/sharedobjs"
	"github.com/khulnasoft-lab/tracker/types/trace"
)

const (
	pkgName          = "tracker"
	maxMemDumpLength = 127
)

type fileExecInfo struct {
	LastCtime int64
	Hash      string
}

// Tracker traces system calls and system events using eBPF
type Tracker struct {
	config    config.Config
	bootTime  uint64
	startTime uint64
	running   atomic.Bool
	done      chan struct{} // signal to safely stop end-stage processing
	OutDir    *os.File      // use utils.XXX functions to create or write to this file
	stats     metrics.Stats
	sigEngine *engine.Engine
	// Events States
	eventsState map[events.ID]events.EventState
	// Events
	eventsSorter     *sorting.EventsChronologicalSorter
	eventsPool       *sync.Pool
	eventProcessor   map[events.ID][]func(evt *trace.Event) error
	eventDerivations derive.Table
	eventSignatures  map[events.ID]bool
	// Artifacts
	fileHashes     *lru.Cache[string, fileExecInfo]
	capturedFiles  map[string]int64
	writtenFiles   map[string]string
	netCapturePcap *pcaps.Pcaps
	// Internal Data
	readFiles     map[string]string
	pidsInMntns   bucketscache.BucketsCache // first n PIDs in each mountns
	kernelSymbols helpers.KernelSymbolTable
	// eBPF
	bpfModule *bpf.Module
	probes    *probes.ProbeGroup
	// BPF Maps
	StackAddressesMap *bpf.BPFMap
	FDArgPathMap      *bpf.BPFMap
	// Perf Buffers
	eventsPerfMap  *bpf.PerfBuffer // perf buffer for events
	fileWrPerfMap  *bpf.PerfBuffer // perf buffer for file writes
	netCapPerfMap  *bpf.PerfBuffer // perf buffer for network captures
	bpfLogsPerfMap *bpf.PerfBuffer // perf buffer for bpf logs
	// Events Channels
	eventsChannel       chan []byte // channel for events
	fileCapturesChannel chan []byte // channel for file writes
	netCapChannel       chan []byte // channel for network captures
	bpfLogsChannel      chan []byte // channel for bpf logs
	// Lost Events Channels
	lostEvChannel       chan uint64 // channel for lost events
	lostCapturesChannel chan uint64 // channel for lost file writes
	lostNetCapChannel   chan uint64 // channel for lost network captures
	lostBPFLogChannel   chan uint64 // channel for lost bpf logs
	// Containers
	cgroups           *cgroup.Cgroups
	containers        *containers.Containers
	contPathResolver  *containers.ContainerPathResolver
	contSymbolsLoader *sharedobjs.ContainersSymbolsLoader
	// Control Plane
	controlPlane *controlplane.Controller
	// Specific Events Needs
	triggerContexts trigger.Context
	readyCallback   func(gocontext.Context)
	// Streams
	streamsManager *streams.StreamsManager
}

func (t *Tracker) Stats() *metrics.Stats {
	return &t.stats
}

// GetEssentialEventsList sets the default events used by tracker
func GetEssentialEventsList() map[events.ID]events.EventState {
	// Set essential events
	return map[events.ID]events.EventState{
		events.SchedProcessExec: {},
		events.SchedProcessExit: {},
		events.SchedProcessFork: {},
	}
}

// GetCaptureEventsList sets events used to capture data
func GetCaptureEventsList(cfg config.Config) map[events.ID]events.EventState {
	captureEvents := make(map[events.ID]events.EventState)

	// All capture events should be placed, at least for now, to
	// all matched policies, or else the event won't be set to
	// matched policy in eBPF and should_submit() won't submit
	// the capture event to userland.

	if cfg.Capture.Exec {
		captureEvents[events.CaptureExec] = events.EventState{
			Submit: 0xFFFFFFFFFFFFFFFF,
		}
	}
	if cfg.Capture.FileWrite.Capture {
		captureEvents[events.CaptureFileWrite] = events.EventState{
			Submit: 0xFFFFFFFFFFFFFFFF,
		}
	}
	if cfg.Capture.FileRead.Capture {
		captureEvents[events.CaptureFileRead] = events.EventState{}
	}
	if cfg.Capture.Module {
		captureEvents[events.CaptureModule] = events.EventState{
			Submit: 0xFFFFFFFFFFFFFFFF,
		}
	}
	if cfg.Capture.Mem {
		captureEvents[events.CaptureMem] = events.EventState{
			Submit: 0xFFFFFFFFFFFFFFFF,
		}
	}
	if cfg.Capture.Bpf {
		captureEvents[events.CaptureBpf] = events.EventState{
			Submit: 0xFFFFFFFFFFFFFFFF,
		}
	}
	if pcaps.PcapsEnabled(cfg.Capture.Net) {
		captureEvents[events.CaptureNetPacket] = events.EventState{
			Submit: 0xFFFFFFFFFFFFFFFF,
		}
	}

	return captureEvents
}

func (t *Tracker) handleEventsDependencies(givenEventId events.ID, submitMap uint64) {
	givenEventDefinition := events.Core.GetDefinitionByID(givenEventId)
	for _, depEventId := range givenEventDefinition.GetDependencies().GetIDs() {
		depEventState, ok := t.eventsState[depEventId]
		if !ok {
			depEventState = events.EventState{}
			t.handleEventsDependencies(depEventId, submitMap)
		}

		depEventState.Submit |= submitMap
		t.eventsState[depEventId] = depEventState

		if events.Core.GetDefinitionByID(givenEventId).IsSignature() {
			t.eventSignatures[depEventId] = true
		}
	}
}

// New creates a new Tracker instance based on a given valid Config. It is
// expected that it won't cause external system side effects (reads, writes,
// etc.)
func New(cfg config.Config) (*Tracker, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, errfmt.Errorf("validation error: %v", err)
	}

	// Create Tracker

	t := &Tracker{
		config:          cfg,
		done:            make(chan struct{}),
		writtenFiles:    make(map[string]string),
		readFiles:       make(map[string]string),
		capturedFiles:   make(map[string]int64),
		eventsState:     GetEssentialEventsList(),
		eventSignatures: make(map[events.ID]bool),
		streamsManager:  streams.NewStreamsManager(),
	}

	// Initialize capabilities rings soon

	err = capabilities.Initialize(t.config.Capabilities.BypassCaps)
	if err != nil {
		return t, errfmt.WrapError(err)
	}
	caps := capabilities.GetInstance()

	// Pseudo events added by capture

	for eventID, eCfg := range GetCaptureEventsList(cfg) {
		t.eventsState[eventID] = eCfg
	}

	// Events chosen by the user

	for p := range t.config.Policies.Map() {
		for e := range p.EventsToTrace {
			var submit, emit uint64
			if _, ok := t.eventsState[e]; ok {
				submit = t.eventsState[e].Submit
				emit = t.eventsState[e].Emit
			}
			utils.SetBit(&submit, uint(p.ID))
			utils.SetBit(&emit, uint(p.ID))
			t.eventsState[e] = events.EventState{Submit: submit, Emit: emit}
		}
	}

	// Handle all essential events dependencies

	for id, evt := range t.eventsState {
		t.handleEventsDependencies(id, evt.Submit)
	}

	// Update capabilities rings with all events dependencies

	for id := range t.eventsState {
		if !events.Core.IsDefined(id) {
			return t, errfmt.Errorf("event %d is not defined", id)
		}
		evtCaps := events.Core.GetDefinitionByID(id).GetDependencies().GetCapabilities()
		err = caps.BaseRingAdd(evtCaps.GetBase()...)
		if err != nil {
			return t, errfmt.WrapError(err)
		}
		err = caps.BaseRingAdd(evtCaps.GetEBPF()...)
		if err != nil {
			return t, errfmt.WrapError(err)
		}
	}

	// Add/Drop capabilities to/from the Base ring (always effective)

	capsToAdd, err := capabilities.ReqByString(t.config.Capabilities.AddCaps...)
	if err != nil {
		return t, errfmt.WrapError(err)
	}
	err = caps.BaseRingAdd(capsToAdd...)
	if err != nil {
		return t, errfmt.WrapError(err)
	}

	capsToDrop, err := capabilities.ReqByString(t.config.Capabilities.DropCaps...)
	if err != nil {
		return t, errfmt.WrapError(err)
	}
	err = caps.BaseRingRemove(capsToDrop...)
	if err != nil {
		return t, errfmt.WrapError(err)
	}

	// Register default event processors

	t.registerEventProcessors()

	// Start event triggering logic context

	t.triggerContexts = trigger.NewContext()

	return t, nil
}

// Init initialize tracker instance and it's various subsystems, potentially
// performing external system operations to initialize them. NOTE: any
// initialization logic, especially one that causes side effects, should go
// here and not New().
func (t *Tracker) Init() error {
	// Initialize needed values

	initReq, err := t.generateInitValues()
	if err != nil {
		return errfmt.Errorf("failed to generate required init values: %s", err)
	}

	// Init kernel symbols map

	if initReq.Kallsyms {
		err = capabilities.GetInstance().Specific(
			func() error {
				return t.NewKernelSymbols()
			},
			cap.SYSLOG,
		)
		if err != nil {
			return errfmt.WrapError(err)
		}
	}

	t.validateKallsymsDependencies() // Canceling events missing kernel symbols

	// Initialize buckets cache

	var mntNSProcs map[int]int

	if t.config.MaxPidsCache == 0 {
		t.config.MaxPidsCache = 5 // TODO: configure this ? never set, default = 5
	}

	t.pidsInMntns.Init(t.config.MaxPidsCache)

	err = capabilities.GetInstance().Specific(
		func() error {
			mntNSProcs, err = proc.GetMountNSFirstProcesses()
			return err
		},
		cap.DAC_OVERRIDE,
		cap.SYS_PTRACE,
	)
	if err == nil {
		for mountNS, pid := range mntNSProcs {
			t.pidsInMntns.AddBucketItem(uint32(mountNS), uint32(pid))
		}
	} else {
		logger.Debugw("Initializing buckets cache", "error", errfmt.WrapError(err))
	}

	// Initialize cgroups filesystems

	t.cgroups, err = cgroup.NewCgroups()
	if err != nil {
		return errfmt.WrapError(err)
	}

	// Initialize containers enrichment logic

	t.containers, err = containers.New(
		t.cgroups,
		t.config.Sockets,
		"containers_map",
	)
	if err != nil {
		return errfmt.Errorf("error initializing containers: %v", err)
	}

	if err := t.containers.Populate(); err != nil {
		return errfmt.Errorf("error initializing containers: %v", err)
	}

	t.contPathResolver = containers.InitContainerPathResolver(&t.pidsInMntns)
	t.contSymbolsLoader = sharedobjs.InitContainersSymbolsLoader(t.contPathResolver, 1024)

	// Initialize event derivation logic

	err = t.initDerivationTable()
	if err != nil {
		return errfmt.Errorf("error initializing event derivation map: %v", err)
	}

	// Initialize eBPF programs and maps

	err = capabilities.GetInstance().EBPF(
		func() error {
			return t.initBPF()
		},
	)
	if err != nil {
		t.Close()
		return errfmt.WrapError(err)
	}

	// Initialize hashes for files

	t.fileHashes, err = lru.New[string, fileExecInfo](1024)
	if err != nil {
		t.Close()
		return errfmt.WrapError(err)
	}

	// Initialize capture directory

	if err := os.MkdirAll(t.config.Capture.OutputPath, 0755); err != nil {
		t.Close()
		return errfmt.Errorf("error creating output path: %v", err)
	}

	t.OutDir, err = utils.OpenExistingDir(t.config.Capture.OutputPath)
	if err != nil {
		t.Close()
		return errfmt.Errorf("error opening out directory: %v", err)
	}

	// Initialize network capture (all needed pcap files)

	t.netCapturePcap, err = pcaps.New(t.config.Capture.Net, t.OutDir)
	if err != nil {
		t.Close()
		return errfmt.Errorf("error initializing network capture: %v", err)
	}

	// Get reference to stack trace addresses map

	stackAddressesMap, err := t.bpfModule.GetMap("stack_addresses")
	if err != nil {
		t.Close()
		return errfmt.Errorf("error getting access to 'stack_addresses' eBPF Map %v", err)
	}
	t.StackAddressesMap = stackAddressesMap

	// Get reference to fd arg path map

	fdArgPathMap, err := t.bpfModule.GetMap("fd_arg_path_map")
	if err != nil {
		t.Close()
		return errfmt.Errorf("error getting access to 'fd_arg_path_map' eBPF Map %v", err)
	}
	t.FDArgPathMap = fdArgPathMap

	// Initialize events sorting (pipeline step)

	if t.config.Output.EventsSorting {
		t.eventsSorter, err = sorting.InitEventSorter()
		if err != nil {
			return errfmt.WrapError(err)
		}
	}

	// Initialize events pool

	t.eventsPool = &sync.Pool{
		New: func() interface{} {
			return &trace.Event{}
		},
	}

	// Tracker bpf code uses monotonic clock as event timestamp. Get current
	// monotonic clock so tracker can calculate event timestamps relative to it.

	var ts unix.Timespec
	err = unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts)
	if err != nil {
		return errfmt.Errorf("getting clock time %v", err)
	}
	startTime := ts.Nano()

	// Calculate the boot time using the monotonic time (since this is the clock
	// we're using as a timestamp) Note: this is NOT the real boot time, as the
	// monotonic clock doesn't take into account system sleeps.

	bootTime := time.Now().UnixNano() - startTime

	// Initialize times

	t.startTime = uint64(startTime)
	t.bootTime = uint64(bootTime)

	return nil
}

// InitValues determines if to initialize values that might be needed by eBPF programs
type InitValues struct {
	Kallsyms bool
}

func (t *Tracker) generateInitValues() (InitValues, error) {
	initVals := InitValues{}
	for evt := range t.eventsState {
		if !events.Core.IsDefined(evt) {
			return initVals, errfmt.Errorf("event %d is undefined", evt)
		}
		for range events.Core.GetDefinitionByID(evt).GetDependencies().GetKSymbols() {
			initVals.Kallsyms = true // only if length > 0
		}
	}

	return initVals, nil
}

// initTailCall initializes a given tailcall.
func (t *Tracker) initTailCall(tailCall events.TailCall) error {
	tailCallMapName := tailCall.GetMapName()
	tailCallProgName := tailCall.GetProgName()
	tailCallIndexes := tailCall.GetIndexes()

	// Pick eBPF map by name.
	bpfMap, err := t.bpfModule.GetMap(tailCallMapName)
	if err != nil {
		return errfmt.WrapError(err)
	}
	// Pick eBPF program by name.
	bpfProg, err := t.bpfModule.GetProgram(tailCallProgName)
	if err != nil {
		return errfmt.Errorf("could not get BPF program %s: %v", tailCallProgName, err)
	}
	// Pick eBPF program file descriptor.
	bpfProgFD := bpfProg.FileDescriptor()
	if bpfProgFD < 0 {
		return errfmt.Errorf("could not get BPF program FD for %s: %v", tailCallProgName, err)
	}

	once := &sync.Once{}

	// Pick all indexes (event, or syscall, IDs) the BPF program should be related to.
	for _, index := range tailCallIndexes {
		// Special treatment for indexes of syscall events.
		if events.Core.GetDefinitionByID(events.ID(index)).IsSyscall() {
			// Optimization: enable enter/exit probes only if at least one syscall is enabled.
			once.Do(func() {
				err := t.probes.Attach(probes.SyscallEnter__Internal)
				if err != nil {
					logger.Errorw("error attaching to syscall enter", "error", err)
				}
				err = t.probes.Attach(probes.SyscallExit__Internal)
				if err != nil {
					logger.Errorw("error attaching to syscall enter", "error", err)
				}
			})
			// Workaround: Do not map eBPF program to unsupported syscalls (arm64, e.g.)
			if index >= uint32(events.Unsupported) {
				continue
			}
		}
		// Update given eBPF map with the eBPF program file descriptor at given index.
		err := bpfMap.Update(unsafe.Pointer(&index), unsafe.Pointer(&bpfProgFD))
		if err != nil {
			return errfmt.WrapError(err)
		}
	}

	return nil
}

// initDerivationTable initializes tracker's events.DerivationTable. For each
// event, represented through its ID, we declare to which other events it can be
// derived and the corresponding function to derive into that Event.
func (t *Tracker) initDerivationTable() error {
	shouldSubmit := func(id events.ID) func() bool {
		return func() bool { return t.eventsState[id].Submit > 0 }
	}

	t.eventDerivations = derive.Table{
		events.CgroupMkdir: {
			events.ContainerCreate: {
				Enabled:        shouldSubmit(events.ContainerCreate),
				DeriveFunction: derive.ContainerCreate(t.containers),
			},
		},
		events.CgroupRmdir: {
			events.ContainerRemove: {
				Enabled:        shouldSubmit(events.ContainerRemove),
				DeriveFunction: derive.ContainerRemove(t.containers),
			},
		},
		events.PrintSyscallTable: {
			events.HookedSyscalls: {
				Enabled:        shouldSubmit(events.PrintSyscallTable),
				DeriveFunction: derive.DetectHookedSyscall(t.kernelSymbols),
			},
		},
		events.PrintNetSeqOps: {
			events.HookedSeqOps: {
				Enabled:        shouldSubmit(events.HookedSeqOps),
				DeriveFunction: derive.HookedSeqOps(t.kernelSymbols),
			},
		},
		events.HiddenKernelModuleSeeker: {
			events.HiddenKernelModule: {
				Enabled:        shouldSubmit(events.HiddenKernelModuleSeeker),
				DeriveFunction: derive.HiddenKernelModule(),
			},
		},
		events.SharedObjectLoaded: {
			events.SymbolsLoaded: {
				Enabled: shouldSubmit(events.SymbolsLoaded),
				DeriveFunction: derive.SymbolsLoaded(
					t.contSymbolsLoader,
					t.config.Policies,
				),
			},
			events.SymbolsCollision: {
				Enabled: shouldSubmit(events.SymbolsCollision),
				DeriveFunction: derive.SymbolsCollision(
					t.contSymbolsLoader,
					t.config.Policies,
				),
			},
		},
		events.SchedProcessExec: {
			events.SymbolsCollision: {
				Enabled: shouldSubmit(events.SymbolsCollision),
				DeriveFunction: derive.SymbolsCollision(
					t.contSymbolsLoader,
					t.config.Policies,
				),
			},
		},
		//
		// Network Derivations
		//
		events.NetPacketIPBase: {
			events.NetPacketIPv4: {
				Enabled:        shouldSubmit(events.NetPacketIPv4),
				DeriveFunction: derive.NetPacketIPv4(),
			},
			events.NetPacketIPv6: {
				Enabled:        shouldSubmit(events.NetPacketIPv6),
				DeriveFunction: derive.NetPacketIPv6(),
			},
		},
		events.NetPacketTCPBase: {
			events.NetPacketTCP: {
				Enabled:        shouldSubmit(events.NetPacketTCP),
				DeriveFunction: derive.NetPacketTCP(),
			},
		},
		events.NetPacketUDPBase: {
			events.NetPacketUDP: {
				Enabled:        shouldSubmit(events.NetPacketUDP),
				DeriveFunction: derive.NetPacketUDP(),
			},
		},
		events.NetPacketICMPBase: {
			events.NetPacketICMP: {
				Enabled:        shouldSubmit(events.NetPacketICMP),
				DeriveFunction: derive.NetPacketICMP(),
			},
		},
		events.NetPacketICMPv6Base: {
			events.NetPacketICMPv6: {
				Enabled:        shouldSubmit(events.NetPacketICMPv6),
				DeriveFunction: derive.NetPacketICMPv6(),
			},
		},
		events.NetPacketDNSBase: {
			events.NetPacketDNS: {
				Enabled:        shouldSubmit(events.NetPacketDNS),
				DeriveFunction: derive.NetPacketDNS(),
			},
			events.NetPacketDNSRequest: {
				Enabled:        shouldSubmit(events.NetPacketDNSRequest),
				DeriveFunction: derive.NetPacketDNSRequest(),
			},
			events.NetPacketDNSResponse: {
				Enabled:        shouldSubmit(events.NetPacketDNSResponse),
				DeriveFunction: derive.NetPacketDNSResponse(),
			},
		},
		events.NetPacketHTTPBase: {
			events.NetPacketHTTP: {
				Enabled:        shouldSubmit(events.NetPacketHTTP),
				DeriveFunction: derive.NetPacketHTTP(),
			},
			events.NetPacketHTTPRequest: {
				Enabled:        shouldSubmit(events.NetPacketHTTPRequest),
				DeriveFunction: derive.NetPacketHTTPRequest(),
			},
			events.NetPacketHTTPResponse: {
				Enabled:        shouldSubmit(events.NetPacketHTTPResponse),
				DeriveFunction: derive.NetPacketHTTPResponse(),
			},
		},
	}

	return nil
}

// RegisterEventDerivation registers an event derivation handler for tracker to use in the event pipeline
func (t *Tracker) RegisterEventDerivation(deriveFrom events.ID, deriveTo events.ID, deriveCondition func() bool, deriveLogic derive.DeriveFunction) error {
	if t.eventDerivations == nil {
		return errfmt.Errorf("tracker not initialized yet")
	}

	return t.eventDerivations.Register(deriveFrom, deriveTo, deriveCondition, deriveLogic)
}

// options config should match defined values in ebpf code
const (
	optExecEnv uint32 = 1 << iota
	optCaptureFilesWrite
	optExtractDynCode
	optStackAddresses
	optCaptureModules
	optCgroupV1
	optProcessInfo
	optTranslateFDFilePath
	optCaptureBpf
	optCaptureFileRead
)

func (t *Tracker) getOptionsConfig() uint32 {
	var cOptVal uint32

	if t.config.Output.ExecEnv {
		cOptVal = cOptVal | optExecEnv
	}
	if t.config.Output.StackAddresses {
		cOptVal = cOptVal | optStackAddresses
	}
	if t.config.Capture.FileWrite.Capture {
		cOptVal = cOptVal | optCaptureFilesWrite
	}
	if t.config.Capture.FileRead.Capture {
		cOptVal = cOptVal | optCaptureFileRead
	}
	if t.config.Capture.Module {
		cOptVal = cOptVal | optCaptureModules
	}
	if t.config.Capture.Bpf {
		cOptVal = cOptVal | optCaptureBpf
	}
	if t.config.Capture.Mem {
		cOptVal = cOptVal | optExtractDynCode
	}
	switch t.cgroups.GetDefaultCgroup().(type) {
	case *cgroup.CgroupV1:
		cOptVal = cOptVal | optCgroupV1
	}
	if t.config.Output.ParseArgumentsFDs {
		cOptVal = cOptVal | optTranslateFDFilePath
	}

	return cOptVal
}

func (t *Tracker) computeConfigValues() []byte {
	// config_entry
	configVal := make([]byte, 256)

	// tracker_pid
	binary.LittleEndian.PutUint32(configVal[0:4], uint32(os.Getpid()))
	// options
	binary.LittleEndian.PutUint32(configVal[4:8], t.getOptionsConfig())
	// cgroup_v1_hid
	binary.LittleEndian.PutUint32(configVal[8:12], uint32(t.containers.GetDefaultCgroupHierarchyID()))
	// padding
	binary.LittleEndian.PutUint32(configVal[12:16], 0)

	for p := range t.config.Policies.Map() {
		byteIndex := p.ID / 8
		bitOffset := p.ID % 8

		// filter enabled policies bitmap
		if p.UIDFilter.Enabled() {
			// uid_filter_enabled_scopes
			configVal[16+byteIndex] |= 1 << bitOffset
		}
		if p.PIDFilter.Enabled() {
			// pid_filter_enabled_scopes
			configVal[24+byteIndex] |= 1 << bitOffset
		}
		if p.MntNSFilter.Enabled() {
			// mnt_ns_filter_enabled_scopes
			configVal[32+byteIndex] |= 1 << bitOffset
		}
		if p.PidNSFilter.Enabled() {
			// pid_ns_filter_enabled_scopes
			configVal[40+byteIndex] |= 1 << bitOffset
		}
		if p.UTSFilter.Enabled() {
			// uts_ns_filter_enabled_scopes
			configVal[48+byteIndex] |= 1 << bitOffset
		}
		if p.CommFilter.Enabled() {
			// comm_filter_enabled_scopes
			configVal[56+byteIndex] |= 1 << bitOffset
		}
		if p.ContIDFilter.Enabled() {
			// cgroup_id_filter_enabled_scopes
			configVal[64+byteIndex] |= 1 << bitOffset
		}
		if p.ContFilter.Enabled() {
			// cont_filter_enabled_scopes
			configVal[72+byteIndex] |= 1 << bitOffset
		}
		if p.NewContFilter.Enabled() {
			// new_cont_filter_enabled_scopes
			configVal[80+byteIndex] |= 1 << bitOffset
		}
		if p.NewPidFilter.Enabled() {
			// new_pid_filter_enabled_scopes
			configVal[88+byteIndex] |= 1 << bitOffset
		}
		if p.ProcessTreeFilter.Enabled() {
			// proc_tree_filter_enabled_scopes
			configVal[96+byteIndex] |= 1 << bitOffset
		}
		if p.BinaryFilter.Enabled() {
			// bin_path_filter_enabled_scopes
			configVal[104+byteIndex] |= 1 << bitOffset
		}
		if p.Follow {
			// follow_filter_enabled_scopes
			configVal[112+byteIndex] |= 1 << bitOffset
		}

		// filter out scopes bitmap
		if p.UIDFilter.FilterOut() {
			// uid_filter_out_scopes
			configVal[120+byteIndex] |= 1 << bitOffset
		}
		if p.PIDFilter.FilterOut() {
			// pid_filter_out_scopes
			configVal[128+byteIndex] |= 1 << bitOffset
		}
		if p.MntNSFilter.FilterOut() {
			// mnt_ns_filter_out_scopes
			configVal[136+byteIndex] |= 1 << bitOffset
		}
		if p.PidNSFilter.FilterOut() {
			// pid_ns_filter_out_scopes
			configVal[144+byteIndex] |= 1 << bitOffset
		}
		if p.UTSFilter.FilterOut() {
			// uts_ns_filter_out_scopes
			configVal[152+byteIndex] |= 1 << bitOffset
		}
		if p.CommFilter.FilterOut() {
			// comm_filter_out_scopes
			configVal[160+byteIndex] |= 1 << bitOffset
		}
		if p.ContIDFilter.FilterOut() {
			// cgroup_id_filter_out_scopes
			configVal[168+byteIndex] |= 1 << bitOffset
		}
		if p.ContFilter.FilterOut() {
			// cont_filter_out_scopes
			configVal[176+byteIndex] |= 1 << bitOffset
		}
		if p.NewContFilter.FilterOut() {
			// new_cont_filter_out_scopes
			configVal[184+byteIndex] |= 1 << bitOffset
		}
		if p.NewPidFilter.FilterOut() {
			// new_pid_filter_out_scopes
			configVal[192+byteIndex] |= 1 << bitOffset
		}
		if p.ProcessTreeFilter.FilterOut() {
			// proc_tree_filter_out_scopes
			configVal[200+byteIndex] |= 1 << bitOffset
		}
		if p.BinaryFilter.FilterOut() {
			// bin_path_filter_out_scopes
			configVal[208+byteIndex] |= 1 << bitOffset
		}

		// enabled_scopes
		configVal[216+byteIndex] |= 1 << bitOffset
	}

	// compute all policies internals
	t.config.Policies.Compute()

	// uid_max
	binary.LittleEndian.PutUint64(configVal[224:232], t.config.Policies.UIDFilterMax())
	// uid_min
	binary.LittleEndian.PutUint64(configVal[232:240], t.config.Policies.UIDFilterMin())
	// pid_max
	binary.LittleEndian.PutUint64(configVal[240:248], t.config.Policies.PIDFilterMax())
	// pid_min
	binary.LittleEndian.PutUint64(configVal[248:256], t.config.Policies.PIDFilterMin())

	return configVal
}

// validateKallsymsDependencies load all symbols required by events dependencies
// from the kallsyms file to check for missing symbols. If some symbols are
// missing, it will cancel their event with informative error message.
func (t *Tracker) validateKallsymsDependencies() {
	var reqKsyms []string
	symsToDependentEvents := make(map[string][]events.ID)
	for id := range t.eventsState {
		for _, symDep := range events.Core.GetDefinitionByID(id).GetDependencies().GetKSymbols() {
			reqKsyms = append(reqKsyms, symDep.GetSymbol())
			if symDep.IsRequired() {
				symEvents, ok := symsToDependentEvents[symDep.GetSymbol()]
				if ok {
					symEvents = append(symEvents, id)
				} else {
					symEvents = []events.ID{id}
				}
				symsToDependentEvents[symDep.GetSymbol()] = symEvents
			}
		}
	}

	kallsymsValues := LoadKallsymsValues(t.kernelSymbols, reqKsyms)

	// Figuring out for each event if it has missing required symbols and which
	missingSymsPerEvent := make(map[events.ID][]string)
	for sym, depEventsIDs := range symsToDependentEvents {
		_, ok := kallsymsValues[sym]
		if ok {
			continue
		}
		for _, depEventID := range depEventsIDs {
			eventMissingSyms, ok := missingSymsPerEvent[depEventID]
			if ok {
				eventMissingSyms = append(eventMissingSyms, sym)
			} else {
				eventMissingSyms = []string{sym}
			}
			missingSymsPerEvent[depEventID] = eventMissingSyms
		}
	}

	// Cancel events with missing symbols dependencies
	for eventToCancel, missingDepSyms := range missingSymsPerEvent {
		eventNameToCancel := events.Core.GetDefinitionByID(eventToCancel).GetName()
		logger.Errorw(
			"Event canceled because of missing kernel symbol dependency",
			"missing symbols", missingDepSyms, "event", eventNameToCancel,
		)
		delete(t.eventsState, eventToCancel)
	}
}

func (t *Tracker) populateBPFMaps() error {
	// Initialize events parameter types map
	eventsParams := make(map[events.ID][]bufferdecoder.ArgType)
	for _, eventDefinition := range events.Core.GetDefinitions() {
		id := eventDefinition.GetID()
		params := eventDefinition.GetParams()
		for _, param := range params {
			eventsParams[id] = append(eventsParams[id], bufferdecoder.GetParamType(param.Type))
		}
	}

	// Prepare events map
	eventsMap, err := t.bpfModule.GetMap("events_map")
	if err != nil {
		return errfmt.WrapError(err)
	}
	for id, ecfg := range t.eventsState {
		eventConfigVal := make([]byte, 16)

		// bitmap of policies that require this event to be submitted
		binary.LittleEndian.PutUint64(eventConfigVal[0:8], ecfg.Submit)

		// encoded event's parameter types
		var paramTypes uint64
		params := eventsParams[id]
		for n, paramType := range params {
			paramTypes = paramTypes | (uint64(paramType) << (8 * n))
		}
		binary.LittleEndian.PutUint64(eventConfigVal[8:16], paramTypes)

		err := eventsMap.Update(unsafe.Pointer(&id), unsafe.Pointer(&eventConfigVal[0]))
		if err != nil {
			return errfmt.WrapError(err)
		}
	}

	// Prepare 32bit to 64bit syscall number mapping
	sys32to64BPFMap, err := t.bpfModule.GetMap("sys_32_to_64_map") // u32, u32
	if err != nil {
		return errfmt.WrapError(err)
	}
	for eventDefID, eventDefinition := range events.Core.GetDefinitions() {
		id32BitU32 := uint32(eventDefinition.GetID32Bit()) // ID32Bit is int32
		idU32 := uint32(eventDefID)                        // ID is int32
		err := sys32to64BPFMap.Update(unsafe.Pointer(&id32BitU32), unsafe.Pointer(&idU32))
		if err != nil {
			return errfmt.WrapError(err)
		}
	}

	if t.kernelSymbols != nil {
		err = t.UpdateBPFKsymbolsMap()
		if err != nil {
			return errfmt.WrapError(err)
		}
	}

	// Initialize kconfig variables (map used instead of relying in libbpf's .kconfig automated maps)
	// Note: this allows libbpf not to rely on the system kconfig file, tracker does the kconfig var identification job

	bpfKConfigMap, err := t.bpfModule.GetMap("kconfig_map") // u32, u32
	if err != nil {
		return errfmt.WrapError(err)
	}

	kconfigValues, err := initialization.LoadKconfigValues(t.config.KernelConfig)
	if err != nil {
		return errfmt.WrapError(err)
	}

	for key, value := range kconfigValues {
		keyU32 := uint32(key)
		valueU32 := uint32(value)
		err = bpfKConfigMap.Update(unsafe.Pointer(&keyU32), unsafe.Pointer(&valueU32))
		if err != nil {
			return errfmt.WrapError(err)
		}
	}

	cZero := uint32(0)

	// net_packet configuration map
	if pcaps.PcapsEnabled(t.config.Capture.Net) {
		bpfNetConfigMap, err := t.bpfModule.GetMap("netconfig_map")
		if err != nil {
			return errfmt.WrapError(err)
		}

		netConfigVal := make([]byte, 8) // u32 capture_options, u32 capture_length

		options := pcaps.GetPcapOptions(t.config.Capture.Net)

		binary.LittleEndian.PutUint32(netConfigVal[0:4], uint32(options))
		binary.LittleEndian.PutUint32(netConfigVal[4:8], t.config.Capture.Net.CaptureLength)

		if err = bpfNetConfigMap.Update(
			unsafe.Pointer(&cZero),
			unsafe.Pointer(&netConfigVal[0]),
		); err != nil {
			return errfmt.Errorf("error updating net config eBPF map: %v", err)
		}
	}

	// Initialize config map
	bpfConfigMap, err := t.bpfModule.GetMap("config_map")
	if err != nil {
		return errfmt.WrapError(err)
	}

	configVal := t.computeConfigValues()
	if err = bpfConfigMap.Update(unsafe.Pointer(&cZero), unsafe.Pointer(&configVal[0])); err != nil {
		return errfmt.WrapError(err)
	}

	for p := range t.config.Policies.Map() {
		policyID := uint(p.ID)
		errMap := make(map[string]error, 0)

		errMap[policy.UIDFilterMap] = p.UIDFilter.UpdateBPF(t.bpfModule, policyID)
		errMap[policy.PIDFilterMap] = p.PIDFilter.UpdateBPF(t.bpfModule, policyID)
		errMap[policy.MntNSFilterMap] = p.MntNSFilter.UpdateBPF(t.bpfModule, policyID)
		errMap[policy.PidNSFilterMap] = p.PidNSFilter.UpdateBPF(t.bpfModule, policyID)
		errMap[policy.UTSFilterMap] = p.UTSFilter.UpdateBPF(t.bpfModule, policyID)
		errMap[policy.CommFilterMap] = p.CommFilter.UpdateBPF(t.bpfModule, policyID)
		errMap[policy.ContIdFilter] = p.ContIDFilter.UpdateBPF(t.bpfModule, t.containers, policyID)
		errMap[policy.BinaryFilterMap] = p.BinaryFilter.UpdateBPF(t.bpfModule, policyID)

		for k, v := range errMap {
			if v != nil {
				return errfmt.Errorf("error setting %v filter: %v", k, v)
			}
		}
	}

	// Populate containers map with existing containers
	err = t.containers.PopulateBpfMap(t.bpfModule)
	if err != nil {
		return errfmt.WrapError(err)
	}

	// Set filters given by the user to filter file write events
	fileWritePathFilterMap, err := t.bpfModule.GetMap("file_write_path_filter") // u32, u32
	if err != nil {
		return err
	}

	for i := uint32(0); i < uint32(len(t.config.Capture.FileWrite.PathFilter)); i++ {
		filterFilePathWriteBytes := []byte(t.config.Capture.FileWrite.PathFilter[i])
		if err = fileWritePathFilterMap.Update(unsafe.Pointer(&i), unsafe.Pointer(&filterFilePathWriteBytes[0])); err != nil {
			return err
		}
	}

	// Set filters given by the user to filter file read events
	fileReadPathFilterMap, err := t.bpfModule.GetMap("file_read_path_filter") // u32, u32
	if err != nil {
		return err
	}

	for i := uint32(0); i < uint32(len(t.config.Capture.FileRead.PathFilter)); i++ {
		filterFilePathReadBytes := []byte(t.config.Capture.FileRead.PathFilter[i])
		if err = fileReadPathFilterMap.Update(unsafe.Pointer(&i), unsafe.Pointer(&filterFilePathReadBytes[0])); err != nil {
			return err
		}
	}

	// Set filters given by the user to filter file read and write type and fds
	fileTypeFilterMap, err := t.bpfModule.GetMap("file_type_filter") // u32, u32
	if err != nil {
		return errfmt.WrapError(err)
	}

	// Should match the value of CAPTURE_READ_TYPE_FILTER_IDX in eBPF code
	captureReadTypeFilterIndex := uint32(0)
	captureReadTypeFilterVal := uint32(t.config.Capture.FileRead.TypeFilter)
	if err = fileTypeFilterMap.Update(unsafe.Pointer(&captureReadTypeFilterIndex),
		unsafe.Pointer(&captureReadTypeFilterVal)); err != nil {
		return errfmt.WrapError(err)
	}

	// Should match the value of CAPTURE_WRITE_TYPE_FILTER_IDX in eBPF code
	captureWriteTypeFilterIndex := uint32(1)
	captureWriteTypeFilterVal := uint32(t.config.Capture.FileWrite.TypeFilter)
	if err = fileTypeFilterMap.Update(unsafe.Pointer(&captureWriteTypeFilterIndex),
		unsafe.Pointer(&captureWriteTypeFilterVal)); err != nil {
		return errfmt.WrapError(err)
	}

	// Initialize tail call dependencies
	tailCalls := events.Core.GetTailCalls(t.eventsState)
	for _, tailCall := range tailCalls {
		err := t.initTailCall(tailCall)
		if err != nil {
			return errfmt.Errorf("failed to initialize tail call: %v", err)
		}
	}

	return nil
}

// attachProbes attaches selected events probes to their respective eBPF progs
func (t *Tracker) attachProbes() error {
	var err error

	// attach control plane probes first
	err = t.controlPlane.Attach()
	if err != nil {
		return errfmt.WrapError(err)
	}

	// attach selected tracing events probes

	for tr := range t.eventsState {
		if !events.Core.IsDefined(tr) {
			continue
		}

		eventDefinition := events.Core.GetDefinitionByID(tr)

		// attach internal syscall probes for selected syscall events, if any
		if eventDefinition.IsSyscall() {
			err := t.probes.Attach(probes.SyscallEnter__Internal)
			if err != nil {
				return errfmt.WrapError(err)
			}
			err = t.probes.Attach(probes.SyscallExit__Internal)
			if err != nil {
				return errfmt.WrapError(err)
			}
		}

		// attach probes for selected events
		for _, dep := range eventDefinition.GetDependencies().GetProbes() {
			err = t.probes.Attach(dep.GetHandle(), t.cgroups)
			if err != nil && dep.IsRequired() {
				return errfmt.Errorf("failed to attach required probe: %v", err)
			}
		}
	}

	return nil
}

func (t *Tracker) initBPF() error {
	var err error

	// Execute code with higher privileges: ring1 (required)

	newModuleArgs := bpf.NewModuleArgs{
		KConfigFilePath: t.config.KernelConfig.GetKernelConfigFilePath(),
		BTFObjPath:      t.config.BTFObjPath,
		BPFObjBuff:      t.config.BPFObjBytes,
	}

	// Open the eBPF object file (create a new module)

	t.bpfModule, err = bpf.NewModuleFromBufferArgs(newModuleArgs)
	if err != nil {
		return errfmt.WrapError(err)
	}

	// Initialize probes

	t.probes, err = probes.NewDefaultProbeGroup(t.bpfModule, t.netEnabled())
	if err != nil {
		return errfmt.WrapError(err)
	}

	// Load the eBPF object into kernel

	err = t.bpfModule.BPFLoadObject()
	if err != nil {
		return errfmt.WrapError(err)
	}

	// Populate eBPF maps with initial data

	err = t.populateBPFMaps()
	if err != nil {
		return errfmt.WrapError(err)
	}

	// Initialize control plane
	t.controlPlane, err = controlplane.NewController(t.bpfModule, t.containers, t.config.ContainersEnrich)
	if err != nil {
		return errfmt.WrapError(err)
	}

	// Attach eBPF programs to selected event's probes

	err = t.attachProbes()
	if err != nil {
		return errfmt.WrapError(err)
	}

	// Update all ProcessTreeFilters after probes are attached: reduce the
	// possible race window between the bpf programs updating the maps and
	// userland reading procfs and also dealing with same maps.

	for p := range t.config.Policies.Map() {
		err = p.ProcessTreeFilter.UpdateBPF(t.bpfModule, uint(p.ID))
		if err != nil {
			return errfmt.Errorf("error building process tree: %v", err)
		}
	}

	// Initialize perf buffers and needed channels

	t.eventsChannel = make(chan []byte, 1000)
	t.lostEvChannel = make(chan uint64)
	if t.config.PerfBufferSize < 1 {
		return errfmt.Errorf("invalid perf buffer size: %d", t.config.PerfBufferSize)
	}
	t.eventsPerfMap, err = t.bpfModule.InitPerfBuf(
		"events",
		t.eventsChannel,
		t.lostEvChannel,
		t.config.PerfBufferSize,
	)
	if err != nil {
		return errfmt.Errorf("error initializing events perf map: %v", err)
	}

	if t.config.BlobPerfBufferSize > 0 {
		t.fileCapturesChannel = make(chan []byte, 1000)
		t.lostCapturesChannel = make(chan uint64)
		t.fileWrPerfMap, err = t.bpfModule.InitPerfBuf(
			"file_writes",
			t.fileCapturesChannel,
			t.lostCapturesChannel,
			t.config.BlobPerfBufferSize,
		)
		if err != nil {
			return errfmt.Errorf("error initializing file_writes perf map: %v", err)
		}
	}

	if pcaps.PcapsEnabled(t.config.Capture.Net) {
		t.netCapChannel = make(chan []byte, 1000)
		t.lostNetCapChannel = make(chan uint64)
		t.netCapPerfMap, err = t.bpfModule.InitPerfBuf(
			"net_cap_events",
			t.netCapChannel,
			t.lostNetCapChannel,
			t.config.PerfBufferSize,
		)
		if err != nil {
			return errfmt.Errorf("error initializing net capture perf map: %v", err)
		}
	}

	t.bpfLogsChannel = make(chan []byte, 1000)
	t.lostBPFLogChannel = make(chan uint64)
	t.bpfLogsPerfMap, err = t.bpfModule.InitPerfBuf(
		"logs",
		t.bpfLogsChannel,
		t.lostBPFLogChannel,
		t.config.PerfBufferSize,
	)
	if err != nil {
		return errfmt.Errorf("error initializing logs perf map: %v", err)
	}

	return errfmt.WrapError(err)
}

const pollTimeout int = 300

// Run starts the trace. it will run until ctx is cancelled
func (t *Tracker) Run(ctx gocontext.Context) error {
	// Some events need initialization before the perf buffers are polled

	err := t.triggerSyscallsIntegrityCheck(trace.Event{})
	if err != nil {
		logger.Warnw("hooked_syscalls returned an error", "error", err)
	}
	t.triggerSeqOpsIntegrityCheck(trace.Event{})
	err = t.triggerMemDump(trace.Event{})
	if err != nil {
		logger.Warnw("print_mem_dump returned an error", "error", err)
	}

	go t.lkmSeekerRoutine(ctx)

	// Start control plane
	err = t.controlPlane.Start()
	if err != nil {
		return err
	}
	go t.controlPlane.Run(ctx)

	// Main event loop (polling events perf buffer)

	t.eventsPerfMap.Poll(pollTimeout)

	go t.processLostEvents() // termination signaled by closing t.done
	go t.handleEvents(ctx)

	// Parallel perf buffer with file writes events

	if t.config.BlobPerfBufferSize > 0 {
		t.fileWrPerfMap.Poll(pollTimeout)
		go t.processFileCaptures(ctx)
	}

	// Network capture perf buffer (similar to regular pipeline)

	if pcaps.PcapsEnabled(t.config.Capture.Net) {
		t.netCapPerfMap.Poll(pollTimeout)
		go t.processNetCaptureEvents(ctx)
	}

	// Logging perf buffer

	t.bpfLogsPerfMap.Poll(pollTimeout)
	go t.processBPFLogs(ctx)

	// Management

	t.running.Store(true) // set running state after writing pid file
	t.ready(ctx)          // executes ready callback, non blocking
	<-ctx.Done()          // block until ctx is cancelled elsewhere

	// Close perf buffers

	t.eventsPerfMap.Close()
	if t.config.BlobPerfBufferSize > 0 {
		t.fileWrPerfMap.Close()
	}
	if pcaps.PcapsEnabled(t.config.Capture.Net) {
		t.netCapPerfMap.Close()
	}
	t.bpfLogsPerfMap.Close()

	// TODO: move logic below somewhere else (related to file writes)

	// record index of written files
	if t.config.Capture.FileWrite.Capture {
		err := updateCaptureMapFile(t.OutDir, "written_files", t.writtenFiles, t.config.Capture.FileWrite)
		if err != nil {
			return err
		}
	}

	// record index of read files
	if t.config.Capture.FileRead.Capture {
		err := updateCaptureMapFile(t.OutDir, "read_files", t.readFiles, t.config.Capture.FileRead)
		if err != nil {
			return err
		}
	}

	t.Close() // close Tracker

	return nil
}

func updateCaptureMapFile(fileDir *os.File, filePath string, capturedFiles map[string]string, cfg config.FileCaptureConfig) error {
	f, err := utils.OpenAt(fileDir, filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return errfmt.Errorf("error logging captured files")
	}
	defer func() {
		if err := f.Close(); err != nil {
			logger.Errorw("Closing file", "error", err)
		}
	}()
	for fileName, filePath := range capturedFiles {
		captureFiltered := false
		// TODO: We need a method to decide if the capture was filtered by FD or type.
		for _, filterPrefix := range cfg.PathFilter {
			if !strings.HasPrefix(filePath, filterPrefix) {
				captureFiltered = true
				break
			}
		}
		if captureFiltered {
			// Don't write mapping of files that were not actually captured
			continue
		}
		if _, err := f.WriteString(fmt.Sprintf("%s %s\n", fileName, filePath)); err != nil {
			return errfmt.Errorf("error logging captured files")
		}
	}
	return nil
}

// Close cleans up created resources
func (t *Tracker) Close() {
	// clean up (unsubscribe) all streams connected if tracker is done
	if t.streamsManager != nil {
		t.streamsManager.Close()
	}

	if t.probes != nil {
		err := t.probes.DetachAll()
		if err != nil {
			logger.Errorw("failed to detach probes when closing tracker", "err", err)
		}
	}
	if t.controlPlane != nil {
		err := t.controlPlane.Stop()
		if err != nil {
			logger.Errorw("failed to stop control plane when closing tracker", "err", err)
		}
	}
	if t.bpfModule != nil {
		t.bpfModule.Close()
	}
	if t.containers != nil {
		err := t.containers.Close()
		if err != nil {
			logger.Errorw("failed to clean containers module when closing tracker", "err", err)
		}
	}
	if err := t.cgroups.Destroy(); err != nil {
		logger.Errorw("Cgroups destroy", "error", err)
	}

	// set 'running' to false and close 'done' channel only after attempting to close all resources
	t.running.Store(false)
	close(t.done)
}

// Running returns true if the tracker is running
func (t *Tracker) Running() bool {
	return t.running.Load()
}

func (t *Tracker) computeOutFileHash(fileName string) (string, error) {
	f, err := utils.OpenAt(t.OutDir, fileName, os.O_RDONLY, 0)
	if err != nil {
		return "", errfmt.WrapError(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			logger.Errorw("Closing file", "error", err)
		}
	}()
	return computeFileHash(f)
}

func computeFileHashAtPath(fileName string) (string, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return "", errfmt.WrapError(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			logger.Errorw("Closing file", "error", err)
		}
	}()
	return computeFileHash(f)
}

func computeFileHash(file *os.File) (string, error) {
	h := sha256.New()
	_, err := io.Copy(h, file)
	if err != nil {
		return "", errfmt.WrapError(err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// invokeInitEvents emits Tracker events, called Initialization Events, that are generated from the
// userland process itself, and not from the kernel. These events usually serve as informational
// events for the signatures engine/logic.
func (t *Tracker) invokeInitEvents(out chan *trace.Event) {
	var emit uint64

	setMatchedPolicies := func(event *trace.Event, matchedPolicies uint64) {
		event.MatchedPoliciesKernel = matchedPolicies
		event.MatchedPoliciesUser = matchedPolicies
		event.MatchedPolicies = t.config.Policies.MatchedNames(matchedPolicies)
	}

	emit = t.eventsState[events.InitNamespaces].Emit
	if emit > 0 {
		systemInfoEvent := events.InitNamespacesEvent()
		setMatchedPolicies(&systemInfoEvent, emit)
		out <- &systemInfoEvent
		_ = t.stats.EventCount.Increment()
	}

	emit = t.eventsState[events.ExistingContainer].Emit
	if emit > 0 {
		for _, e := range events.ExistingContainersEvents(t.containers, t.config.ContainersEnrich) {
			setMatchedPolicies(&e, emit)
			out <- &e
			_ = t.stats.EventCount.Increment()
		}
	}
}

// netEnabled returns true if any base network event is to be traced
func (t *Tracker) netEnabled() bool {
	for k := range t.eventsState {
		if k >= events.NetPacketBase && k <= events.MaxNetID {
			return true
		}
	}

	// if called before capture meta-events are set to be traced:
	return pcaps.PcapsEnabled(t.config.Capture.Net)
}

//
// TODO: move to triggerEvents package
//

// triggerSyscallsIntegrityCheck is used by a Uprobe to trigger an eBPF program
// that prints the syscall table
func (t *Tracker) triggerSyscallsIntegrityCheck(event trace.Event) error {
	_, ok := t.eventsState[events.HookedSyscalls]
	if !ok {
		return nil
	}

	errArgFilter := make(map[int]error, 0)

	for p := range t.config.Policies.Map() {
		hookedSyscallsFilters := p.ArgFilter.GetEventFilters(events.HookedSyscalls)
		if len(hookedSyscallsFilters) == 0 {
			logger.Debugw("policy %d: no syscalls were provided to hooked_syscall event. "+
				"using default configuration. please provide it via -e hooked_syscalls.args.check_syscalls=<syscall>,<syscall>", p.ID)
			derive.SyscallsToCheck = events.DefaultSyscallsToCheck()
		}

		if len(derive.SyscallsToCheck) == 0 {
			syscallFilter, ok := hookedSyscallsFilters["check_syscalls"].(*filters.StringFilter)
			if syscallFilter != nil && ok {
				eventNamesToID := events.Core.NamesToIDs()
				for _, syscall := range syscallFilter.Equal() {
					_, ok := eventNamesToID[syscall]
					if !ok {
						errArgFilter[p.ID] = fmt.Errorf("policy %d: %s - no such syscall", p.ID, syscall)
						break
					}
					derive.SyscallsToCheck = append(derive.SyscallsToCheck, syscall)
				}
			}
		}
	}

	for k, v := range errArgFilter {
		if v != nil {
			return errfmt.Errorf("error invalid policy %v filter: %v", k, v)
		}
	}

	eventHandle := t.triggerContexts.Store(event)
	t.triggerSyscallsIntegrityCheckCall(uint64(eventHandle), uint64(derive.MaxSupportedSyscallID))
	return nil
}

//go:noinline
func (t *Tracker) triggerSyscallsIntegrityCheckCall(eventHandle uint64, table_size uint64) {
}

// triggerSeqOpsIntegrityCheck is used by a Uprobe to trigger an eBPF program
// that prints the seq ops pointers
func (t *Tracker) triggerSeqOpsIntegrityCheck(event trace.Event) {
	_, ok := t.eventsState[events.HookedSeqOps]
	if !ok {
		return
	}
	var seqOpsPointers [len(derive.NetSeqOps)]uint64
	for i, seqName := range derive.NetSeqOps {
		seqOpsStruct, err := t.kernelSymbols.GetSymbolByName("system", seqName)
		if err != nil {
			continue
		}
		seqOpsPointers[i] = seqOpsStruct.Address
	}
	eventHandle := t.triggerContexts.Store(event)
	_ = t.triggerSeqOpsIntegrityCheckCall(
		uint64(eventHandle),
		seqOpsPointers,
	)
}

//go:noinline
func (t *Tracker) triggerSeqOpsIntegrityCheckCall(
	eventHandle uint64,
	seqOpsStruct [len(derive.NetSeqOps)]uint64) error {
	return nil
}

// triggerMemDump is used by a Uprobe to trigger an eBPF program
// that prints the first bytes of requested symbols or addresses
func (t *Tracker) triggerMemDump(event trace.Event) error {
	if _, ok := t.eventsState[events.PrintMemDump]; !ok {
		return nil
	}

	errArgFilter := make(map[int]error, 0)

	for p := range t.config.Policies.Map() {
		printMemDumpFilters := p.ArgFilter.GetEventFilters(events.PrintMemDump)
		if len(printMemDumpFilters) == 0 {
			errArgFilter[p.ID] = fmt.Errorf("policy %d: no address or symbols were provided to print_mem_dump event. "+
				"please provide it via -e print_mem_dump.args.address=<hex address>"+
				", -e print_mem_dump.args.symbol_name=<owner>:<symbol> or "+
				"-e print_mem_dump.args.symbol_name=<symbol> if specifying a system owned symbol", p.ID)

			continue
		}

		var length uint64
		var err error

		lengthFilter, ok := printMemDumpFilters["length"].(*filters.StringFilter)
		if lengthFilter == nil || !ok || len(lengthFilter.Equal()) == 0 {
			length = maxMemDumpLength // default mem dump length
		} else {
			field := lengthFilter.Equal()[0]
			length, err = strconv.ParseUint(field, 10, 64)
			if err != nil {
				return errfmt.WrapError(err)
			}
		}

		addressFilter, ok := printMemDumpFilters["address"].(*filters.StringFilter)
		if addressFilter != nil && ok {
			for _, field := range addressFilter.Equal() {
				address, err := strconv.ParseUint(field, 16, 64)
				if err != nil {
					return errfmt.WrapError(err)
				}
				eventHandle := t.triggerContexts.Store(event)
				_ = t.triggerMemDumpCall(address, length, eventHandle)
			}
		}

		symbolsFilter, ok := printMemDumpFilters["symbol_name"].(*filters.StringFilter)
		if symbolsFilter != nil && ok {
			for _, field := range symbolsFilter.Equal() {
				symbolSlice := strings.Split(field, ":")
				splittedLen := len(symbolSlice)
				var owner string
				var name string
				if splittedLen == 1 {
					owner = "system"
					name = symbolSlice[0]
				} else if splittedLen == 2 {
					owner = symbolSlice[0]
					name = symbolSlice[1]
				} else {
					return errfmt.Errorf("invalid symbols provided %s - more than one ':' provided", field)
				}
				symbol, err := t.kernelSymbols.GetSymbolByName(owner, name)
				if err != nil {
					// Checking if the user specified a syscall name
					if owner == "system" {
						for _, prefix := range []string{"sys_", "__x64_sys_", "__arm64_sys_"} {
							symbol, err = t.kernelSymbols.GetSymbolByName(owner, prefix+name)
							if err == nil {
								break
							}
						}
					}
					if err != nil {
						return errfmt.WrapError(err)
					}
				}
				eventHandle := t.triggerContexts.Store(event)
				_ = t.triggerMemDumpCall(symbol.Address, length, uint64(eventHandle))
			}
		}
	}

	for k, v := range errArgFilter {
		if v != nil {
			return errfmt.Errorf("error setting %v filter: %v", k, v)
		}
	}

	return nil
}

// AddReadyCallback sets a callback function to be called when the tracker started all its probes
// and is ready to receive events
func (t *Tracker) AddReadyCallback(f func(ctx gocontext.Context)) {
	t.readyCallback = f
}

// ready executes the ready callback if it was set.
// doesn't block the execution of the tracker
func (t *Tracker) ready(ctx gocontext.Context) {
	if t.readyCallback != nil {
		go t.readyCallback(ctx)
	}
}

//go:noinline
func (t *Tracker) triggerMemDumpCall(address uint64, length uint64, eventHandle uint64) error {
	return nil
}

// SubscribeAll returns a stream subscribed to all policies
func (t *Tracker) SubscribeAll() *streams.Stream {
	return t.subscribe(policy.AllPoliciesOn)
}

// Subscribe returns a stream subscribed to selected policies
func (t *Tracker) Subscribe(policyNames []string) (*streams.Stream, error) {
	var policyMask uint64

	for _, policyName := range policyNames {
		p, err := t.config.Policies.LookupByName(policyName)
		if err != nil {
			return nil, err
		}
		utils.SetBit(&policyMask, uint(p.ID))
	}

	return t.subscribe(policyMask), nil
}

func (t *Tracker) subscribe(policyMask uint64) *streams.Stream {
	// TODO: the channel size matches the pipeline channel size,
	// but we should make it configurable in the future.
	return t.streamsManager.Subscribe(policyMask, 10000)
}

// Unsubscribe unsubscribes stream
func (t *Tracker) Unsubscribe(s *streams.Stream) {
	t.streamsManager.Unsubscribe(s)
}
