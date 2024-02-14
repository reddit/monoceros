## Monoceros - A Multi-process Application Controller

Monoceros makes it easy to run, keep alive and terminate multiple copies of single server. Monoceros is designed to support advanced load balancing between the copies, offer observability and debugging tools.

![monoceros architecture graph illustrating a single controller managing the lifetime of multiple workers and a proxy load balancing requests.](assets/monoceros.drawio.png?raw=true "Monoceros Architecture")

See [design doc](https://docs.google.com/document/d/1rXq4xosKG4KKvQXQL51l1i8IUTiudUBi7O0epB8z-ik/edit) for more details.

### Modes of operation

1. **Monoceros in Supervisor Mode**: In this mode Monoceros will only keep your workers alive and observe them. Your workers should open a socket with `SO_REUSEPORT` option set and let the kernel load balance connections among the listening sockets.
1. **Monoceros in Proxy Mode**: In this mode Monoceros will configure an Envoy proxy process that will load balance incoming requests among the workers. Currently only HTTP servers are supported in proxy mode.

If you are onboarding to Monoceros we recommend you to:

1. Start with **Monoceros in Supervisor Mode** which doesnt alter the datapath and provides you with data insights.
1. Monitor the values of `monoceros_workerpool_process_cpu_seconds_total` within the same computation unit (e.g. a kubernetes pod). If you see a large spread between the workers CPU this means that you could use advanced load balancing.
1. Rollout  **Monoceros in Proxy Mode**

### Features

Currently we support:

- Running, restarting and gracefully terminating a number of workers
- Prometheus endpoint exposed through the `/metrics` path in the admin endpoint
- Restart workers after a specific time
- Prometheus stats for the process lifetime
- Advanced load-balancing using a reverse proxy

Features in the roadmap:

- Advanced debugging tooling for workers such as pprof
- Health checking for workers and restart workers when they can no longer answer to health cheks
- Restart workers if they exceed a memory limit

We set a unique environment variable to every worker `MULTIPROCESS_WORKER_ID`.

## Getting Started

### Build

- `make build`: will build just the monoceros binary
- `make processbin`: will build a useful cli app that can be managed via Monoceros

### Run

After you build from source you can run `./dist/monoceros -c ./examples/basic.yaml -d` which will run `ping` locally.

### Test

- `make test`: will run all the tests (unit and integration)
- `test-unit`: will run all the tests with in-memory workers

### Questions

If you want help or support reach out to the devs at #infra-transport
