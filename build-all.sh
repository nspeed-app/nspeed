#! /bin/bash
# local snapshot of the release build (all OS/arch) into dist/, nothing is published.
# real releases are made by .github/workflows/release.yml when a v* tag is pushed.
goreleaser release --snapshot --clean
