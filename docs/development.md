# Development

## Build

```console
make
```

### Upgrading the build image

- Update `build/Dockerfile` as required.
- Test the build locally:

```console
rm build/.uptodate
make !$
```

- Push this change, get it reviewed, and merge it to `master`.
- Run:

```console
git checkout master ; git fetch origin master ; git merge --ff-only master
rm build/.uptodate
make !$
> [...]
> Successfully built deadbeefcafe
> Successfully tagged docker.io/weaveworks/wksctl-build:latest
> docker tag docker.io/weaveworks/wksctl-build docker.io/weaveworks/wksctl-build:master-XXXXXXX
> touch build/.uptodate
docker push docker.io/weaveworks/wksctl-build:$(tools/image-tag)
```

- Update `.circleci/config.yml` to use the newly pushed image.
- Push this change, get it reviewed, and merge it to `master`.

## Adding docs

Docs live in the [`docs` directory](https://github.com/weaveworks/wksctl/tree/master/docs)
and we use Markdown for everything. Every new commit will be published at
<https://wksctl.readthedocs.io/en/latest/>.

A few things to be aware of:

- Use `make serve-docs` to serve the docs locally and point a webbrowser to the URL in the output, e.g. `localhost:8000`, to check out if your changes worked out.
- Upon pushing a PR to this repository, links in the docs will be automatically tested.
