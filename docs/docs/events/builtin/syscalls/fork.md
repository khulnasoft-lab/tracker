
# fork

## Intro
fork - create a copy of the process in the same address space.

## Description
The Linux `fork()` system call creates a new process by duplicating the calling process. The new process is referred to as the child process and the calling process is referred to as the parent process. The child process created from the fork() system call has a copy of the parent process's entire address space. Therefore, when the parent process makes changes to any of its memory, these changes are visible to the child process - unlike `execve()` which completely overlays the address space of the creating process with the contents of the specified executable.

The `fork()` system call returns twice; once in the parent process and once in the child process. In the parent process, the `fork()` system call returns the process ID (PID) of the newly-created child process. In the child process, the `fork()` system call returns 0.

The `fork()` system call is synchronous, meaning the parent process waits for the child process to complete before proceeding. This is necessary for the parent and child processes to establish communication with each other.

There are some drawbacks to using the `fork()` system call. First, the parent process should not change any of its memory before the child process exists, otherwise the child will inherit these changes which could lead to undefined behavior. Secondly, the overhead of switching between processes may adversely affect the performance of the system. Finally, the Linux `fork()` system call is limited to a maximum of 32 processes.

## Arguments
* `flags`:`int`[OPT] - Flags used to modify the behaviour of the `fork()` system call.

### Available Tags
1. K - Originated from kernel-space.
2. U - Originated from user space (for example, pointer to user space memory used to get it)
3. TOCTOU - Vulnerable to TOCTOU (time of check, time of use)
4. OPT - Optional argument - might not always be available (passed with null value)

## Hooks
### do_fork
#### Type
Tracepoint + Kprobe.
#### Purpose
For usage and profiling analysis.

## Example Use Case
The `fork()` system call can be used to create a child process so that the parent process can execute a separate task in parallel and communicate with the child process to return a result. In this way, the parent process does not have to wait for the child process to complete before continuing execution.

## Issues
Linux imposes a hard limit of 32 process creations via fork.

## Related Events
* `execve` - execute a program.
* `clone` - create a child processes or threads.

> This document was automatically generated by OpenAI and needs review. It might
> not be accurate and might contain errors. The authors of Tracker recommend that
> the user reads the "events.go" source file to understand the events and their
> arguments better.
