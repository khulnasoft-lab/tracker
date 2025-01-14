# Custom Events

Tracker comes with many built-in events, but you can extend its capabilities by creating custom events tailored to your specific needs.

Currently, custom events can be implemented using Go.  Refer to the [Go](./golang.md) documentation for detailed instructions on how to define and integrate your custom events into Tracker.

Once you've created your custom event, load it using the `signatures-dir` flag. For example, if your custom event is defined in the directory `/tmp/myevents`, start Tracker with the following command:

```
tracker --signatures-dir=/tmp/myevents
```

!!! Tip
    Tracker also uses the custom events to add a few events, if you pass your own directory
    for `signatures-dir` you will not load the tracker [signatures](../builtin/signatures/index.md),
    to avoid such problems, you can either place your own events under the same directory of the tracker custom events,
    or pass multiple directories for example:
    ```
    tracker --signatures-dir=/tmp/myevents --signatures-dir=./dist/signatures
    ```

👈 Please use the side-navigation on the left in order to browse the different topics.
