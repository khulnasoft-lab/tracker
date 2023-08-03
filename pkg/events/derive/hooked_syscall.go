package derive

import (
	"fmt"

	"github.com/khulnasoft-labs/libbpfgo/helpers"

	"github.com/khulnasoft-labs/tracker/pkg/errfmt"
	"github.com/khulnasoft-labs/tracker/pkg/events"
	"github.com/khulnasoft-labs/tracker/pkg/events/parse"
	"github.com/khulnasoft-labs/tracker/pkg/utils"
	"github.com/khulnasoft-labs/tracker/types/trace"
)

var SyscallsToCheck = make([]string, 0)
var MaxSupportedSyscallID = events.IoPgetevents // Was the last syscall introduced in the minimum version supported 4.18

func DetectHookedSyscall(kernelSymbols helpers.KernelSymbolTable) DeriveFunction {
	return deriveSingleEvent(events.HookedSyscalls, deriveDetectHookedSyscallArgs(kernelSymbols))
}

func deriveDetectHookedSyscallArgs(kernelSymbols helpers.KernelSymbolTable) deriveArgsFunction {
	return func(event trace.Event) ([]interface{}, error) {
		syscallAddresses, err := parse.ArgVal[[]uint64](event.Args, "syscalls_addresses")
		if err != nil {
			return nil, errfmt.Errorf("error parsing syscalls_numbers arg: %v", err)
		}

		hookedSyscall, err := analyzeHookedAddresses(syscallAddresses, kernelSymbols)
		if err != nil {
			return nil, errfmt.Errorf("error parsing analyzing hooked syscalls addresses arg: %v", err)
		}

		return []interface{}{SyscallsToCheck, hookedSyscall}, nil
	}
}

func analyzeHookedAddresses(addresses []uint64, kernelSymbols helpers.KernelSymbolTable) ([]trace.HookedSymbolData, error) {
	hookedSyscalls := make([]trace.HookedSymbolData, 0)

	for _, syscall := range SyscallsToCheck {
		eventNamesToIDs := events.Core.NamesToIDs()
		syscallID, ok := eventNamesToIDs[syscall]
		if !ok {
			return hookedSyscalls, errfmt.Errorf("%s - no such syscall", syscall)
		}

		syscallAddress := addresses[syscallID]
		if syscallAddress == 0 { // syscall pointer is null or in kernel bounds
			continue
		}
		if inText, err := kernelSymbols.TextSegmentContains(syscallAddress); err != nil || inText {
			continue
		}

		hookingFunction := utils.ParseSymbol(syscallAddress, kernelSymbols)

		var hookedSyscallName string

		if events.Core.IsDefined(syscallID) {
			hookedSyscallName = events.Core.GetDefinitionByID(syscallID).GetName()
		} else {
			hookedSyscallName = fmt.Sprint(syscallID)
		}

		hookedSyscalls = append(hookedSyscalls, trace.HookedSymbolData{SymbolName: hookedSyscallName, ModuleOwner: hookingFunction.Owner})
	}

	return hookedSyscalls, nil
}
