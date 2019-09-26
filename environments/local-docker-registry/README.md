# Local Docker Registry

This directory contains the resources required to run your own Docker registry.
This can be useful when working offline, when setting up air-gapped cluster, or
just to speed up integration tests, among other scenarii.

## Instructions

1. Install Docker.
2. Start the [Docker registry](https://hub.docker.com/_/registry):

    ```console
    $ docker run -d \
      -p 5000:5000 \
      --restart always \
      -v /tmp/registry:/var/lib/registry \
      --name registry \
      registry:2
    ```

3. Push the container images used by WKS to it:

    ```console
    ./retag_push.sh
    ```
