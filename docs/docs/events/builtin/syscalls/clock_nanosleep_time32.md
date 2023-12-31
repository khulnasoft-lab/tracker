
# clock_nanosleep_time32

## Intro
clock_nanosleep_time32 - suspend the execution of the calling thread until the time pointed to by rqtp is reached

## Description
The clock_nanosleep_time32() function is used to suspend the execution of the calling thread until the time pointed to by rqtp is reached. If flags is 0, the thread will be suspended until the time pointed by rqtp is reached. The which_clock argument specifies the clock to use for measuring this timeout, the value can be either CLOCK_REALTIME or CLOCK_MONOTONIC. If rmtp (remaining time pointer) is a non-null pointer then the remaining time is stored in the old_timespec32(include <linux/time.h> for details) structure referenced by rmtp. If rmtp is non-null and the clock_nanosleep_time32() time is interrupted by a signal while the thread is suspended, the remaining time is written to rmtp and -1 is returned with errno set to EINTR. This is the most reliable and portable way to implement timeout guards.

## Arguments
* `which_clock`:`clockid_t`[K] - The clock to use for measuring this timeout. The value can be either CLOCK_REALTIME or CLOCK_MONOTONIC.
* `flags`:`int`[K] - If flags is 0, the thread will be suspended until the time pointed by rqtp is reached.
* `rqtp`:`struct old_timespec32`*[K] - Pointer to the address of structure containing the requested time value.
* `rmtp`:`struct old_timespec32`*[K, OPT] - Pointer to an old_timespec32 structure, if it is non-null, the remaining time will be stored in the old_timespec32 structure referenced by it. If it is null the remaining time will be ignored.

### Available Tags
* K - Originated from kernel-space.
* U - Originated from user space (for example, pointer to user space memory used to get it)
* TOCTOU - Vulnerable to TOCTOU (time of check, time of use)
* OPT - Optional argument - might not always be available (passed with null value)

## Hooks
### do_nanosleep_time32
#### Type
Kprobe
#### Purpose
To trace Nanosleep calls made from user-space.

## Example Use Case
The clock_nanosleep_time32() function can be used by applications to easily manage their timeouts while they are waiting for a given event or condition to be satisfied.

## Issues
The clock_nanosleep_time32() call relies on the kernel's scheduling policy, so the system may lag if there is heavy activity and the thread may not wake up exactly when it was supposed to.

## Related Events
* `do_nanosleep_time32` - kernel entry point.
* `clock_nanosleep` - Older syscall which can still be used for the same purpose.

> This document was automatically generated by OpenAI and needs review. It might
> not be accurate and might contain errors. The authors of Tracker recommend that
> the user reads the "events.go" source file to understand the events and their
> arguments better.
