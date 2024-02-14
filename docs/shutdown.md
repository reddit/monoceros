## Pod Shutdown Process

Monoceros is a process controller which handles multiple workers and occasionally a proxy. Since Monoceros is mostly used at scale it needs to shutdown the workers and the proxy in a such a way that:

1. It will minimize traffic disruption
1. It terminates eventually even if the workers are not responding.

To achieve that we have the following process:

1. Send a `SIGTERM` signal to all worker processes. At this point we expect the workers to start draining:

   1. Healthy: the process works as normal and it responds successfully to health checks. This is important because the proxy will still health check the workers even after the `SIGTERM` is sent.
   1. Not-Ready: the process advertises that it is not ready to accept new connections.

1. Wait for all worker process to return for a maximum duration of `signal_timeout`

   1. If `signal_timeout` elapses then we send a `SIGKILL` to the workers and increment `sigkill_total` counter. This part of the shutdown is potentially non graceful and thus should be avoided.

1. **(optional)** Terminate the proxy worker if operating in proxy mode by following the same process as worker process termination (step 2)

1. Exit Monoceros
