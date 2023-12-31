
# recvmsg

## Intro
recvmsg - read a message from a socket

## Description
recvmsg() is used to receive messages from a  socket,  and  may  be  used  to  receive  both  connection-based  and  connectionless messages. recvmsg() may also be used to receive file descriptors sent by means of the sendmsg() system call.

The flags argument provides further options and is constructed by giving the bitwise OR of one or more of the following:
* MSG_CMSG_CLOEXEC - Indicates that associated to each control message an close-on-exec flag must be set.
* MSG_DONTWAIT - Non-blocking operation - make the call fail if the socket is not available for receive.
* MSG_ERRQUEUE - Receive messages from the kernel error queue.
* MSG_OOB - Receive out-of-band data.

Advantage of using recvmsg is the possibility of merging several different types of input into one system call.
Drawback is that it does not provide any data ordering assurance.

## Arguments
* `sockfd`:`int`[K] - File descriptor of the socket from which to receive messages. 
* `msg`:`struct msghdr*`[K] - Pointer to a msghdr structure which will contain the message of up to size 65535 bytes.
* `flags`:`int`[K] - OR-ed bit flags of the possible flags: MSG_CMSG_CLOEXEC, MSG_DONTWAIT, MSG_ERRQUEUE and MSG_OOB.

### Available Tags
* K - Originated from kernel-space.
* U - Originated from user space (for example, pointer to user space memory used to get it)
* TOCTOU - Vulnerable to TOCTOU (time of check, time of use)
* OPT - Optional argument - might not always be available (passed with null value)

## Hooks
### syscall_recvmsg
#### Type
Kprobe
#### Purpose
To analyze and control the messages received from the given socket.

## Example Use Case
An example use of recvmsg is when using socket-level security for validating messages. By using recvmsg, messages delivery can be allowed or denied according to some criteria.

## Issues
No known issues with this event.

## Related Events
* sendmsg - opposite of recvmsg, used to send messages.

> This document was automatically generated by OpenAI and needs review. It might
> not be accurate and might contain errors. The authors of Tracker recommend that
> the user reads the "events.go" source file to understand the events and their
> arguments better.
