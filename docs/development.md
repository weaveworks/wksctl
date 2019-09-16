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
> Successfully tagged quay.io/wksctl/build:latest
> docker tag quay.io/wks/build quay.io/wksctl/build:master-XXXXXXX
> touch build/.uptodate
docker push quay.io/wksctl/build:$(tools/image-tag)
```

- Update `.circleci/config.yml` to use the newly pushed image.
- Push this change, get it reviewed, and merge it to `master`.
