
# ftime

## Intro
ftime - Get and set the current system time

## Description
The ftime() function gets the current time of day, expressed in seconds and milliseconds since the Epoch (00:00:00 UTC, January 1, 1970). It has the following parameters:

* `buf`:`struct timeb *`[K] - a pointer to a `struct timeb` which will be filled in with the current time and date.
* `tz`:`struct timezone *`[K] - an optional pointer to a `struct timezone`, which if supplied, is filled in with information about the local timezone.

For both of these parameters, passing `NULL` will simply indicate that you do not want to receive the corresponding information.

Using ftime() is not recommended for obtaining the current date and time, as there are more accurate methods, but it is most commonly used for calculating the execution time of a program.

### Available Tags
* K - Originated from kernel-space.

## Hooks
### ftime
#### Type
Kprobes
#### Purpose
To observe the execution of ftime and observe the arguments passed to it.

## Example Use Case
One use case for ftime would be to measure the time taken for a certain process or program to be completed. This can be done by obtaining the current time with ftime() before and after an operation, then subtracting the two to get the execution time.

## Issues
The resolution of ftime() is limited at milliseconds, so it is not suitable for performance tuning operations for which higher accuracy is required.

## Related Events
clock_gettime

> This document was automatically generated by OpenAI and needs review. It might
> not be accurate and might contain errors. The authors of Tracker recommend that
> the user reads the "events.go" source file to understand the events and their
> arguments better.
