
# pwritev

## Intro
pwritev - write data from multiple buffers to a file descriptor at a given offset.

## Description
The `pwritev` system call is used to write data from multiple buffers to a file descriptor at a given offset. It is similar to `readv` in the way that it can write data from multiple non-contiguous memory blocks, however, the data is written to a specific location instead of just to a file descriptor. This can be useful for writing data at a specific location, regardless of the current file offset.

The `pwritev` system call is useful when writing data to a specific location in a file, but due to its reliance on a single file descriptor (FD) it can be vulnerable to race conditions when writing to multiple files, since the FD value might change between different calls.

## Arguments
* `fd`: `int` - a valid file descriptor, for a file previously opened for writing.
* `iov`: `const struct iovec*` - a pointer to a struct iovec* array of read buffers. The size of the array is specified in iovcnt.
* `iovcnt`: `unsigned long` - the size of the read buffer array passed in iov.
* `pos_l`: `unsigned long` - the low bits of the position in the file to start writing to.
* `pos_h`: `unsigned long` - the high bits of the position in the file to start writing to.

### Available Tags
* K - Originated from kernel-space.
* U - Originated from user space (for example, pointer to user space memory used to get it)
* TOCTOU - Vulnerable to TOCTOU (time of check, time of use)
* OPT - Optional argument - might not always be available (passed with null value)

## Hooks
### sys_pwritev
#### Type
Kprobes
#### Purpose
To instrument the pwritev syscall in order to get more insight into data being written.

## Example Use Case
In a system where data needs to be written from buffers scattered throughout a wide memory range, `pwritev` can be used to write data directly to a specific file offset. This can eliminate the need to read the entire file in order to write data at a certain location.

## Issues
`pwritev` is vulnerable to race conditions when the same file descriptor is used to write data to multiple files. If the FD value changes between multiple calls, data can be written in the wrong file.

## Related Events
* preadv
* readv
* writev

> This document was automatically generated by OpenAI and needs review. It might
> not be accurate and might contain errors. The authors of Tracker recommend that
> the user reads the "events.go" source file to understand the events and their
> arguments better.
