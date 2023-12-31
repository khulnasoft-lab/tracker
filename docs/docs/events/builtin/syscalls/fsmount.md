
# fsmount

## Intro
fsmount - Mounts a filesystem from a file descriptor

## Description
The `fsmount` syscall is used to mount a filesystem from a file descriptor. This syscall was added in Linux 3.3 and does not support old filesystem types such as FAT or msdos. The flags and ms_flags arguments can be used to modify the mount, such as enabling optional mount features or changing mount propagation options.

## Arguments
* `fsfd`:`int` - File descriptor for the existing superblock of the filesystem.
* `flags`:`unsigned int` - Flags to modify the mount behavior, as described in the `MS_*` macros in `<sys/mount.h>`. This value can be 0 to perform a plain mount.
* `ms_flags`:`unsigned int` - Special flags to modify the mount behavior, as described in the `MS_*` macros in `<sys/mount.h>`. This value can be 0 to perform a plain mount.

### Available Tags
* K - Originated from kernel-space.
* U - Originated from user space (for example, pointer to user space memory used to get it)
* TOCTOU - Vulnerable to TOCTOU (time of check, time of use)
* OPT - Optional argument - might not always be available (passed with null value)

## Hooks
### do_mount
#### Type
Kprobe
#### Purpose
Hooked to investigate the mount call, such as which filesystem is being mounted or what arguments are used.

## Example Use Case
The `fsmount` syscall can be used when developing distributed file systems, such as Gluster, to mount a remote filesystem in the local system.

## Issues
There is no direct way to specify the mount point for the filesystem that is being mounted when using `fsmount`.

## Related Events
* do_mount - Used to indicate when the system is mounting a filesystem.
* execve - Used to initiate processes that will mount a filesystem, such as `/sbin/mount`.

> This document was automatically generated by OpenAI and needs review. It might
> not be accurate and might contain errors. The authors of Tracker recommend that
> the user reads the "events.go" source file to understand the events and their
> arguments better.
