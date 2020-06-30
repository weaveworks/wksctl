#!/bin/sh -e

for f in "$@"; do
    echo "embedmd: generating $f"
    embedmd -w "$f"
done
