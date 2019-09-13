# Frequently asked questions

## Checkpoint

`wksctl` contacts Weaveworks servers for available versions. When a new version is available, `wksctl` will print out a message along with a URL to download it.

The information sent in this check is:

- wksctl version
- Machine Architecture
- Operating System
- Host UUID hash

To disable this check, run the following before executing `wksctl`:

-```console
export CHECKPOINT_DISABLE=1
```
