
# setgroups

## Intro
setgroups - sets the list of supplementary groups of the calling process

## Description
The setgroups() system call allows the calling process to set its supplementary group IDs directly, without manipulating the supplementary group IDs with  the  initgroups() system call.  The setgroups() system call is limited to  processes  with  the  CAP_SETGID  capability  and with an effective user ID of 0. 

This system call is the complementary system call to the getgroups() system call, which gets the supplementary group IDs of the calling process.

## Arguments
* `size`:`int` - The  number of supplementary group IDs in the list. 
* `list`:`gid_t*`[K] - A pointer to the list of group IDs.

### Available Tags
* K - Originated from kernel-space.

## Hooks
### sys_setgroups
#### Type
kprobe+kretprobe
#### Purpose
Observe setgroups() calls and the return values.

## Example Use Case
For example, an administrator could use the setgroups() system call to change the supplementary group IDs associated with a process in order to temporarily assign it additional privileges.

## Issues
One possible issue with setgroups is that it may be vulnerable to time of check, time of use (TOCTOU) race conditions.

## Related Events
getgroups - gets the list of supplementary groups of the calling process

> This document was automatically generated by OpenAI and needs review. It might
> not be accurate and might contain errors. The authors of Tracker recommend that
> the user reads the "events.go" source file to understand the events and their
> arguments better.
