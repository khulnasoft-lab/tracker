
# lstat

## Intro
lstat() - Get file status

## Description
The lstat() system call is used to get information about the file at a certain location in the filesystem. The information is stored in the struct stat and can be used for various purposes including permissions checking, file size, time stamps, etc. The lstat() does not follow symbolic links, so it should be called for each link in order to get its status. 

## Arguments
* `pathname`:`const char *`[U] - A pointer to a character string with the pathname of the file which status is wanted.
* `statbuf`:`struct stat *`[K] - A pointer to a stat structure where the status information will be stored. 

### Available Tags
* K - Originated from kernel-space.
* U - Originated from user space (for example, pointer to user space memory used to get it)
* TOCTOU - Vulnerable to TOCTOU (time of check, time of use)
* OPT - Optional argument - might not always be available (passed with null value)

## Hooks
### sys_lstat
#### Type
Kprobe
#### Purpose
To capture system events related to the lstat() system call.

## Example Use Case
Using lstat() in order to check the permissions of a certain file before actually accessing it.

## Issues
The lstat() system call might not work properly in some systems.

## Related Events
open(), read(), write(), close(), fstat()

> This document was automatically generated by OpenAI and needs review. It might
> not be accurate and might contain errors. The authors of Tracker recommend that
> the user reads the "events.go" source file to understand the events and their
> arguments better.
