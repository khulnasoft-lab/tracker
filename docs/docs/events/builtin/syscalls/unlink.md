
# unlink

## Intro
unlink - Removes a file name for the file system.

## Description
unlink removes the specified pathname from the file system. If the specified
pathname is the last link to a file and no processes have the file open, then the
file is deleted and the space it was using is made available. This system call
fails if the pathname specified is a directory or if the pathname has more than
one link.

unevent is used to unlink a file name and may also be used to remove directories,
provided the directory is empty. However, unlink is not secure against race
conditions and can be impacted by the TOCTOU (Time of Check, Time of Use) bug.

## Arguments
* `pathname`:`const char*`[K, U] - Pathname of the file to unlink. Must be a file, and not a directory.

### Available Tags
* K - Originated from kernel-space.
* U - Originated from user space (for example, pointer to user space memory used to get it)
* TOCTOU - Vulnerable to TOCTOU (time of check, time of use)
* OPT - Optional argument - might not always be available (passed with null value)

## Hooks
### do_unlinkat
#### Type
Kprobe + Kreteprobe
#### Purpose
To log the syscall arguments and the syscall return value upon execution, as well as determine the origin of the syscall.

## Example Use Case
unlink is used to remove a file or directory from the file system, with one important caveat - in order to unlink a directory, the directory must be empty, otherwise, unlink fails. This system call is often used to clean up unused files that have been created during the course of running a program.

## Issues
The C language doesn’t require the system calls to detect errors on their own, so the errors returned by unlink can be unpredictable. Additionally, it is vulnerable to TOCTOU bug, where a potential attacker might be able to exploit the race condition to the system's disadvantage.

## Related Events
The unlink event is related to other system calls that can be used to manipulate files and directories, such as open, close, read, and write. It may also be used in combination with link and symlink to create, rename, and delete links to files in the system.

> This document was automatically generated by OpenAI and needs review. It might
> not be accurate and might contain errors. The authors of Tracker recommend that
> the user reads the "events.go" source file to understand the events and their
> arguments better.
