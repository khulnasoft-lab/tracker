
# fchownat

## Intro

fchownat - change ownership of a file or directory relative to a directory file descriptor.

## Description

The `fchownat()` system call provides a mechanism to modify the ownership (both
user and group) of a specified file or directory.

Unlike the `chown()` system call, `fchownat()` allows operations relative to a
directory referenced by a given file descriptor. This is particularly useful
when working within specific directory contexts or when the exact path to a file
or directory might not be directly accessible or known.

## Arguments

* `dirfd`:`int`[K] - File descriptor pointing to the directory relative to which the pathname is interpreted.
* `pathname`:`const char *`[U] - The path to the file or directory whose ownership is to be changed.
* `owner`:`uid_t`[K] - The user ID to be set. If set to `-1`, the user ID isn't changed.
* `group`:`gid_t`[K] - The group ID to be set. If set to `-1`, the group ID isn't changed.
* `flags`:`int`[K] - Flags to modify function behavior (e.g., `AT_SYMLINK_NOFOLLOW` to not follow symbolic links).

### Available Tags

* K - Originated from kernel-space.
* U - Originated from user space.
* TOCTOU - Vulnerable to TOCTOU (time of check, time of use).
* OPT - Optional argument - might not always be available (passed with null value).

## Hooks

### sys_fchownat

#### Type

Tracepoint (through `sys_enter`).

#### Purpose

To observe and record instances when the `fchownat()` system call is invoked,
capturing details about the target file or directory, as well as the new
ownership details.

## Example Use Case

In environments with strict access controls, monitoring changes in file or
directory ownership can be crucial to maintain security and data integrity.

## Issues

Inappropriate use or vulnerabilities linked to the `fchownat()` system call can
potentially expose files or directories to unauthorized users, posing data
integrity and security risks.

## Related Events

* `chown()` - Change ownership of a file.
* `lchown()` - Change ownership of a symbolic link.
* `fchown()` - Change ownership of a file via its file descriptor.

> This document was automatically generated by OpenAI and reviewed by a Human.
