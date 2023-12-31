
# delete_module

## Intro
delete_module - system call to delete a module from the running kernel

## Description
The delete_module() system call deletes the specified module from the running kernel. It does not delete the associated files from the disk; for this, use rmmod(8). It takes two arguments - name, which is a pointer to the module name, and flags, an integer.

The flags argument serves to specify the behavior of delete_module(). If the module has any active users, the call can either block until all users have freed the module (flags set to 0 or O_NONBLOCK) or just return with an EAGAIN error.

Advantage of using delete_module() system call is that it allows to dynamically manage the behavior of the running kernel configuration.

Disadvantage is that when the call is blocked, other processes might be delayed because they were not able to wait until the delete_module() system call was completed. This may lead to race conditions, which might cause issues with system stability.

## Arguments
* `name`: const char*[K|U] - pointer to the module name
* `flags`: int[K] - integer to specify the behavior of delete_module()

### Available Tags
* K - Originated from kernel-space.
* U - Originated from user space (for example, pointer to user space memory used to get it)
* TOCTOU - Vulnerable to TOCTOU (time of check, time of use)
* OPT - Optional argument - might not always be available (passed with null value)

## Hooks
### do_delete_module
#### Type
Kprobes + ftrace
#### Purpose
Hooks do_delete_module to detect when the delete_module() system call is invoked and to monitor the behavior of delete_module()

## Example Use Case
delete_module() system call can be used to dynamically manage the running kernel configuration. In a production environment, for instance, it can be used to disable certain features or to enable debug mechanisms without restarting the machine.

## Issues
Race conditions can arise from blocking delete_module() system calls, delaying other processes.

## Related Events
The use of delete_module() system call is connected with kprobe_event_handler and rmmod system calls.

> This document was automatically generated by OpenAI and needs review. It might
> not be accurate and might contain errors. The authors of Tracker recommend that
> the user reads the "events.go" source file to understand the events and their
> arguments better.
