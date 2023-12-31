
# utimensat

## Intro
utimensat - Change file timestamps with nanosecond precision

## Description
The utimensat system call creates, checks, and/or modifies the timestamps of a file relative to a directory file descriptor. It provides nanosecond precision and some additional flags that the utime and utimes system calls do not provide.

The 'times' argument is a pointer to a two-element array that specifies the access and modification times respectively. If either timestamp is set to the special value UTIME_OMIT, then that particular timestamp is not set. If 'times' is set to NULL, then the access and modification times of the 'pathname' are set to the current time.

The flags argument is a bitfield that is used to indicate whether the times are relative to the directory referenced by the file descriptor 'dirfd' (AT_SYMLINK_NOFOLLOW), or whether the operation should be applied to the symlink itself (AT_SYMLINK_NOFOLLOW).

The utimensat system call is similar to utime and utimes, but with more precise control over timestamp behavior.

## Arguments
* `dirfd`:`int`[K] - A file descriptor referring to a directory, or the special value AT_FDCWD, which can be used to indicate the current working directory.
* `pathname`:`const char`* `[K]` - A pointer to a string containing the pathname of a file relative to the directory referred to by the file descriptor 'dirfd'.
* `times`:`struct timespec`* `[K]` - A pointer to a two-element array specifying the access and modification times respectively. The values are measured as nanoseconds since the Epoch (Jan 1 1970). If either timestamp is set to the special value UTIME_OMIT, then that particular timestamp is not set. If 'times' is set to NULL, then the access and modification times of the 'pathname' are set to the current time.
* `flags`:`int`[K] - A bit-field indicating whether the times are relative to the directory referred to by the file descriptor 'dirfd' (AT_SYMLINK_NOFOLLOW), or whether the operation should be applied to the symlink itself (AT_SYMLINK_NOFOLLOW).

### Available Tags
* K - Originated from kernel-space. 
* U - Originated from user space (for example, pointer to user space memory used to get it) 
* TOCTOU - Vulnerable to TOCTOU (time of check, time of use) 
* OPT - Optional argument - might not always be available (passed with null value) 

## Hooks
### do_utimes
#### Type
Tracepoint
#### Purpose
To analyze the arguments and return values of the utimensat system call.

## Example Use Case
Utimensat could be used to set timestamps to a target file while allowing a user to avoid issues with Time Of Check To Time Of Use (TOCTOU) race condition by therefore providing a more secure setting of timestamps.

## Issues
Utimensat system call may fail in certain situations due to filesystem type that do not support nanosecond timestamp updates.

## Related Events
* `utime`,`utimes` - These related system calls can modify timestamps for files, but with less precision. 
* `futimesat` - Like utimensat, but operates on a file instead of a pathname.

> This document was automatically generated by OpenAI and needs review. It might
> not be accurate and might contain errors. The authors of Tracker recommend that
> the user reads the "events.go" source file to understand the events and their
> arguments better.
