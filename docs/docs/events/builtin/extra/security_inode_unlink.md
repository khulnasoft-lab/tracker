# security_inode_unlink

## Intro

security_inode_unlink - An event that captures details when an inode is unlinked.

## Description

This event is triggered when an inode is unlinked, representing file or
directory deletion. The eBPF program attached to this event extracts various
attributes related to the unlinked inode, primarily focusing on the path, inode
number, device number, and creation time of the file or directory.

The main purpose of this event is to monitor and log file or directory
deletions, which can be critical for security, monitoring, or auditing use
cases, especially when tracking changes in critical directories or files.

## Arguments

1. **pathname** (`const char*`): The path to the file or directory being unlinked.
2. **inode** (`unsigned long`): Inode number of the file or directory.
3. **dev** (`dev_t`): Device number associated with the inode.
4. **ctime** (`u64`): Creation time of the file or directory.

## Hooks

### trace_security_inode_unlink

#### Type

Kprobe (using `kprobe/security_inode_unlink`).

#### Purpose

To capture and extract detailed information whenever an inode gets unlinked.
This hook provides a set of attributes that can be used to understand the
context and specifics of the unlinked file or directory.

## Example Use Case

Security applications might use this event to track and monitor deletions of
sensitive files or directories. It can also be employed for auditing purposes,
ensuring that no unexpected file removal operations are conducted, especially in
critical system directories.

## Issues

This eBPF program captures details on each unlinked inode, which might introduce
some overhead, especially in systems where files are frequently created and
deleted. It's essential to consider the potential performance implications and
adjust the monitoring frequency or scope if needed.

## Related Events

* security_inode_unlink
* security_inode_mknod
* security_inode_symlink
* security_inode_rename

> This document was automatically generated by OpenAI and reviewed by a Human.
