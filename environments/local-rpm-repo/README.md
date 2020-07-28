# local-docker-registry

## Instructions

1. Create the Docker image for the local YUM repository:

    ```console
    $ export IMAGE_TAG="$(cd ../.. ; ./tools/image-tag)"
    $ docker build -t "docker.io/weaveworks/local-yum-repo:${IMAGE_TAG}" .
    [...]
    Successfully tagged docker.io/weaveworks/local-yum-repo:${IMAGE_TAG}
    ```

2. Run it:

    ```console
    docker run -d -p 8080:80 --restart always --name yumrepo docker.io/weaveworks/local-yum-repo:${IMAGE_TAG}
    ```

3. Use it:

    ```console
    $ curl localhost:8080
    <html>
    <head><title>Index of /</title></head>
    <body bgcolor="white">
    <h1>Index of /</h1><hr><pre><a href="../">../</a>
    <a href="base/">base/</a>                                              13-May-2019 10:04                   -
    [...]
    <a href="yum-plugin-versionlock-1.1.31-50.el7.noarch.rpm">yum-plugin-versionlock-1.1.31-50.el7.noarch.rpm</a>        12-Nov-2018 15:27               36584
    </pre><hr></body>
    </html>
    ```

4. Create a repository configuration file pointing to your local YUM repository:

    ```ini
    [local]
    name=Local
    baseurl=http://localhost:8080
    enabled=1
    gpgcheck=0
    ```

5. Configure it in your `cluster.yaml`:

    ```yaml
    apiVersion: cluster.weave.works/v1alpha3
    kind: ExistingInfraCluster
    spec:
      os:
        files:
        - source:
            configmap: repo
            key: local.repo
          destination: /etc/yum.repos.d/local.repo
    ```

## Future work

The exact list of packages to install should be driven by the "plan" for nodes.
Currently, it is hardcoded in the `Dockerfile` file.
