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

## Adding docs

Docs live in the [`docs` directory](https://github.com/weaveworks/wksctl/tree/master/docs)
and we use Markdown for everything. Every new commit will be published at
<https://wksctl.readthedocs.io/en/latest/>.

A few things to be aware of:

- The landing page (`docs/index.rst`) is written in reStructuredText, because readthedocs uses sphinx for generating HTML/PDF/etc. Make sure your new doc is listed there - it's how the index is built. (`recommonmark` is used as the bridge between Markdown and reStructuredText).
- Use `make serve-docs` to generate the docs locally and point a webbrowser to the URL in the output, e.g. `localhost:8000/_build/html`, to check out if your changes worked out.
- Links in the docs will be automatically tested.
- Gotcha 1: links in markdown tables are problematic: <https://github.com/ryanfox/sphinx-markdown-tables/issues/18>
- Gotcha 2: cross-referencing using anchors is problematic: <https://github.com/readthedocs/recommonmark/issues/8>
- Gotcha 3: Make sure your use of headings, e.g. `#`, `##`, `###` makes sense. The table of contents will be a bit upset if you don't.
