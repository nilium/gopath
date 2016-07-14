#!/usr/bin/env bash

# Place in PATH somewhere with higher precedence than a go binary, such
# as the go tool, goimports, etc. to have this act as a gopath shim of
# sorts. Can break if you pass an incorrect first argument, since it
# uses arg 0 to determine the name of the thing to exec to.
#
# This shim must be in the PATH in order for it to function as expected,
# since it uses which(1) to look-up the next-in-line executables of the
# same basename as its arg 0.

export GOPATH="$(gopath)"

base="$(basename "$0")"
if ! next="$(which -a "$base" | sed -n 2p)" || [[ -z "$next" ]]
then
        echo "${base} not found" 2>&1
        exit 1
fi

exec "$next" "$@"
