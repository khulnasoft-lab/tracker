
# setresgid16

## Intro 
setresgid16 - set real, effective and saved user or group ID

## Description
The setresgid16() system call sets the real GID, effective GID, and saved set-user-ID of the calling process. This call sets the three GIDs with a single syscall, instead of the sets the individual GID values with setregid(2). 

The three GIDs are set according to the following rules:

* If rgid is not (uid_t)-1, it is set as the real GID of the process;

* If egid is not (uid_t)-1, it is set as the effective GID of the process;

* If suid is not (uid_t)-1, it is set as the saved set-GID of the process.

It is permitted that the real GID, effective GID, and saved set-GID are all set to the same value. 

By convention, a set-user-ID or set-GID program should clear the saved set-user-ID or set-GID when it starts, and should do so early in its initialization before other things that might open files or create child processes. In an environment where file names are relied on to be predictable and unchanging, using setresgid16() to clear the saved set-GID avoids certain security problems (but also see getresgid16(2), below).

## Arguments
* `rgid`:`old_uid_t`[K] - real group ID of the calling process. 
* `euid`:`old_uid_t`[K] - effective group ID of the calling process. 
* `suid`:`old_uid_t`[K] - saved set-group-ID of the calling process.

### Available Tags
* K - Originated from kernel-space.

## Hooks
### sys_setresgid16
#### Type
Kprobe.
#### Purpose
To monitor execution of the setresgid16() system call and get the values of the given arguments.

## Example Use Case
This event can be used to monitor changes in the real GID, effective GID and saved set-GID of the running process. It can be used to identify abnormal behavior or unexpected modifications in these GIDs.

## Issues
N/A

## Related Events
* sys_setregid16() - sets the real GID and effective GID of the calling process.
* sys_setresuid16() - sets the real UID, effective UID, and saved set-user-ID of the calling process.

> This document was automatically generated by OpenAI and needs review. It might
> not be accurate and might contain errors. The authors of Tracker recommend that
> the user reads the "events.go" source file to understand the events and their
> arguments better.
