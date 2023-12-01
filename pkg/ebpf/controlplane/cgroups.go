package controlplane

import (
	"github.com/khulnasoft-lab/tracker/pkg/capabilities"
	"github.com/khulnasoft-lab/tracker/pkg/errfmt"
	"github.com/khulnasoft-lab/tracker/pkg/events/parse"
	"github.com/khulnasoft-lab/tracker/pkg/logger"
	"github.com/khulnasoft-lab/tracker/types/trace"
)

//
// Containers Lifecycle
//

// processCgroupMkdir handles the cgroup_mkdir signal.
func (ctrl *Controller) processCgroupMkdir(args []trace.Argument) error {
	cgroupId, err := parse.ArgVal[uint64](args, "cgroup_id")
	if err != nil {
		return errfmt.Errorf("error parsing cgroup_mkdir signal args: %v", err)
	}
	path, err := parse.ArgVal[string](args, "cgroup_path")
	if err != nil {
		return errfmt.Errorf("error parsing cgroup_mkdir signal args: %v", err)
	}
	hId, err := parse.ArgVal[uint32](args, "hierarchy_id")
	if err != nil {
		return errfmt.Errorf("error parsing cgroup_mkdir signal args: %v", err)
	}
	info, err := ctrl.cgroupManager.CgroupMkdir(cgroupId, path, hId)
	if err != nil {
		return errfmt.WrapError(err)
	}
	if info.Container.ContainerId == "" && !info.Dead {
		// If cgroupId is from a regular cgroup directory, and not the container base directory
		// (from known runtimes), it should be removed from the containers bpf map.
		err := capabilities.GetInstance().EBPF(
			func() error {
				return ctrl.cgroupManager.RemoveFromBPFMap(ctrl.bpfModule, cgroupId, hId)
			},
		)
		if err != nil {
			// If the cgroupId was not found in bpf map, this could mean that it is not a container
			// cgroup and, as a systemd cgroup, could have been created and removed very quickly. In
			// this case, we don't want to return an error.
			logger.Debugw("Failed to remove entry from containers bpf map", "error", err)
		}
		return errfmt.WrapError(err)
	}

	if !ctrl.enrichDisabled {
		// If cgroupId belongs to a container, enrich now (in a goroutine)
		go func() {
			_, err := ctrl.cgroupManager.EnrichCgroupInfo(cgroupId)
			if err != nil {
				logger.Errorw("error triggering container enrich in control plane", "error", err)
			}
		}()
	}

	return nil
}

// processCgroupRmdir handles the cgroup_rmdir signal.
func (ctrl *Controller) processCgroupRmdir(args []trace.Argument) error {
	cgroupId, err := parse.ArgVal[uint64](args, "cgroup_id")
	if err != nil {
		return errfmt.Errorf("error parsing cgroup_rmdir args: %v", err)
	}

	hId, err := parse.ArgVal[uint32](args, "hierarchy_id")
	if err != nil {
		return errfmt.Errorf("error parsing cgroup_rmdir args: %v", err)
	}
	ctrl.cgroupManager.CgroupRemove(cgroupId, hId)
	return nil
}
