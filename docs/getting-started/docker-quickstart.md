# Docker Quickstart

This section details how you can run Tracker through a container image.

## Prerequisites

Please ensure that Docker or another container runtime is working on your machine. 
## Run the Tracker container images

All of the Tracker container images are stored in a public registry on [Docker Hub.](https://hub.docker.com/r/khulnasoft/tracker)
You can easily start experimenting with Tracker using the Docker image.

**On x86 architecture, please run the following command:**

```console
docker run \
  --name tracker --rm -it \
  --pid=host --cgroupns=host --privileged \
  -v /etc/os-release:/etc/os-release-host:ro \
  khulnasoft/tracker:latest
```

**If you are on arm64 architecture, you will need to replace the container image tag to `aarch64`:**

```console
docker run \
  --name tracker --rm -it \
  --pid=host --cgroupns=host --privileged \
  -v /etc/os-release:/etc/os-release-host:ro \
  khulnasoft/tracker:aarch64
```

To learn how to install Tracker in a production environment, [check out the Kubernetes guide](./kubernetes-quickstart).
