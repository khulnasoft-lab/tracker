
# vm86old

## Intro
vm86old - Invoke real-mode software on 80386 and above processors.

## Description
vm86old is used to run real-mode programs, i.e. DOS programs, on 80386 and above processors. It provides the necessary information for invoking real-mode programs, along with the protection of the operating system and any other applications running in protected mode.

Using this system call comes with certain advantages and drawbacks. On one hand it preserves the system integrity as it keeps DOS mode separate from protected mode; however, it also limits the program from performing certain tasks that would otherwise be possible if they ran in the same context.

## Arguments
* `info`: `struct vm86_struct*`[K] - Pointer to a `vm86_struct` structure that describes the real-mode program that the system call should run.

### Available Tags
* K - Originated from kernel-space.
* U - Originated from user space (for example, pointer to user space memory used to get it)
* TOCTOU - Vulnerable to TOCTOU (time of check, time of use)
* OPT - Optional argument - might not always be available (passed with null value)

## Hooks
### do_vm86_old
#### Type
Kprobe
#### Purpose
To monitor calls to the vm86old system call.

## Example Use Case
A driver could use this system call to run real-mode programs from kernel-space.

## Issues
Due to the nature of the system call, the security of the system is affected as it enables running code in unprotected mode, meaning that malicious code could be used to exploit the underlying system.

## Related Events
* `sigreturn` - Returns the state of the calling thread the way it was at the time of the syscall.

> This document was automatically generated by OpenAI and needs review. It might
> not be accurate and might contain errors. The authors of Tracker recommend that
> the user reads the "events.go" source file to understand the events and their
> arguments better.
