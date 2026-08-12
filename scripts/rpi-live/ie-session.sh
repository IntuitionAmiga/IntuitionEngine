#!/bin/sh
set -eu

export IE_AUDIO_BACKEND=jack
# This reaches QEMU's serial console under -nographic and proves greetd has
# started the appliance session, rather than merely reached userspace.
printf '%s\n' 'Intuition Engine greetd session started' >/dev/console 2>/dev/null || true
# Do not exec Cage: the wrapper records an abnormal child exit, waits a bounded
# period, then exits for greetd. That prevents overlapping sessions and a rapid
# uncontrolled restart loop when JACK or the display stack keeps failing.
set +e
cage -s -- xwayland-run -- /opt/ie/ie-launch.sh
status=$?
set -e
if [ "$status" -eq 0 ]; then exit 0; fi
sleep 2
exit "$status"
