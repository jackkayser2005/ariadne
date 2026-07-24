# Container images

Docker gives Ariadne reproducible tool environments. It does not define
personas or evidence semantics.

## Current image

`docs/Dockerfile` builds the Hugo documentation preview with a pinned Hugo
version and Hextra module.

## Boundary

Baseline and treatment personas are data consumed by the same runner image.
Creating one image per persona would introduce uncontrolled filesystem and
dependency differences.

Android device control remains outside Docker for Experiment 001. Running an
emulator inside Docker Desktop on Windows adds nested virtualization and device
access problems before they solve a demonstrated need.

Add another image only when a runnable component requires dependencies that
cannot be expressed cleanly in the Go binary or on the authorized Android test
device.

