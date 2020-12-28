#!/bin/sh

set -e

if [ ! -f "build/env.sh" ]; then
    echo "$0 must be run from the root of the repository."
    exit 2
fi

# Create fake Go workspace if it doesn't exist yet.
workspace="$PWD/build/_workspace"
root="$PWD"
kokdir="$workspace/src/github.com/kokprojects"
if [ ! -L "$kokdir/go-kok" ]; then
    mkdir -p "$kokdir"
    cd "$kokdir"
    ln -s ../../../../../. go-kok
    cd "$root"
fi

# Set up the environment to use the workspace.
GOPATH="$workspace"
export GOPATH

# Run the command inside the workspace.
cd "$kokdir/go-kok"
PWD="$kokdir/go-kok"

# Launch the arguments with the configured environment.
exec "$@"
