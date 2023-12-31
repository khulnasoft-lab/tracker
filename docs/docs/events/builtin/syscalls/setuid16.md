
# setuid16

## Intro
setuid16 - sets the effective user ID of the calling process

## Description
setuid16() sets the effective user ID of the calling process. The argument uid is used to set the  effective user ID of the caller.

If the calling process is privileged (i.e., owns the superuser id), the effective user ID can be set to any value.

Under normal conditions, an unprivileged process may change the real and saved user IDs to the value of uid only if they match its own real (or effective, if uid is not privileged) user ID. A privileged process (under Linux: one having the CAP_SETUID capability) may set the effective user ID to any value.

The related seteuid16(), setreuid16() and setresuid16() provide more fine-grain control over processes' privilege.

## Arguments
* `uid`:old_old_uid_t[KU] - user ID of the new set of the IDs.

### Available Tags
* K - Originated from kernel-space.
* U - Originated from user space (for example, pointer to user space memory used to get it)

## Hooks
### setuid16
#### Type
kprobe
#### Purpose
Monitoring of user identity changes.

## Example Use Case
One example of use case of setuid16 is to trace a malicious thread changing its user identity and running malicious code.

## Issues
There are no known issues with setuid16.

## Related Events
setreuid16, setresuid16, seteuid16.

> This document was automatically generated by OpenAI and needs review. It might
> not be accurate and might contain errors. The authors of Tracker recommend that
> the user reads the "events.go" source file to understand the events and their
> arguments better.
