### Internal Development Docs

These docs will help with development and code organization

### Architecture

The internal architecture of Monoceros contains 2 main components -- the Supervisor and the Admin interface.

#### Supervisor

![](assets/monoceros_internal.drawio.png?raw=true "Monoceros Architecture")

The Supervisor is the core of Monoceros. It is configured to create worker pools which contain many replicas of a worker process. Each worker process is spawned with the same command and arguments but is a separate process. After all worker pools are created, the Supervisor enters a control loop where it listens for events and reacts accordingly. Events can come from the Limiters or from the operating system. For instance, if the main Monoceros process receives a SIGTERM, the Supervisor is responsible for terminating the all the worker processes gracefully. Restarts are another common Supervisor reaction.

Each worker pool is configured with a restart budget, `MaxRestarts`. If the Supervisor exceeds the restart budget, the pool of workers is terminated.

##### Limiters

Limiters can be attached to individual worker processes. They can monitor attributes of a Worker's runtime such as duration and memory usage. When configured limits are exceeded, the limiter notifies the Supervisor who may try to restart the worker.

#### The Admin Interface

The HTTP admin interface listens on a configured port. Currently, it only provides a `/metrics` endpoint for Prometheus scraping.

### Testing

Monoceros testing makes extensive use of a binary called `processbin`. The source for `processbin` can be found in the `cmd` subdirectoy. This binary allows test cases to control aspects of a worker process to simulate edge cases and failure modes. `WorkerPools` are tested with a combination of `realProcessWorker` and `InMemoryWorker` instances. `InMemoryWorker` instances serve as a sort of mock worker that can simulate different exit codes, restart counts and misbehavior.

### Metrics

Monoceros makes Prometheus metrics available on the admin interface at `/metrics`.

#### Limiters

| Metric Name                               | Description                                 | Unit         | Type    |
| ----------------------------------------- | ------------------------------------------- | ------------ | ------- |
| monocerous.workerpool.restarts_total      | Number of restarts performed by a pool      | restarts     | counter |
| monocerous.workerpool.limits_events_total | Number of limiting events applied to a pool | limit events | counter |

Why they're important: Restarts and limit events should be reasonably predictable. For instance, a worker that is configured to run for a certain amount of time before restarting. If these counters increases in ways not anticipated or if memory limits are hit, it may be indicative of a misconfigured or misbehaving worker.
