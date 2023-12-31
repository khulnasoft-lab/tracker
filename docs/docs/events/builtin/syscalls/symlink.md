
# symlink

## Intro
symlink - creates a symbolic link to a file in the file system.

## Description
The `symlink` event creates a symbolic link at the given path to the target file. This link is similar to a regular file, but it points to the actual file rather than containing a copy of the contents. This is useful for creating multiple references to the same file without taking up more space. The main drawback is that if the target file is changed, so are all the links pointing to it.

## Arguments
* `target`:`const char*`[K] - The path to the file that will be pointed to.
* `linkpath`:`const char*`[K] - The path of the symbolic link that will be created.

### Available Tags
* K - Originated from kernel-space.

## Hooks
### sys_symlink
#### Type
kretprobe
#### Purpose
Hooked to trace the execution of the `sys_symlink` kernel function, which is the entrypoint for the `symlink` syscall.

## Example Use Case
A use case for the `symlink` event could be analyzing the behavior of different system processes when creating symbolic links. This could help identify potential malicious actors and vulnerabilities in the system, or analyze how processes interact with files and other processes.

## Issues
The main issue with the `symlink` event is that it is vulnerable to TOCTOU (time of check, time of use) attacks. This means that the target file could be changed before the `symlink` syscall is triggered, leading to a change of the reference for the symbolic link. 

## Related Events
* `unlink` - deletes the file referenced by a given path.
* `link` - creates a hard link to a file in the file system.

> This document was automatically generated by OpenAI and needs review. It might
> not be accurate and might contain errors. The authors of Tracker recommend that
> the user reads the "events.go" source file to understand the events and their
> arguments better.
