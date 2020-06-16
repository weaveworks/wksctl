#!/bin/sh -e

for f in "$@"; do
    echo "embedmd: checking $f"
    embedmd -d "$f"
done
