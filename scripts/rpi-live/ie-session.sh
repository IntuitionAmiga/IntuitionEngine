#!/bin/sh
set -eu

export IE_AUDIO_BACKEND=jack
export WLR_RENDERER=pixman
export WLR_NO_HARDWARE_CURSORS=1
# Ebitengine prefers GLES on real Raspberry Pi hardware. QEMU's generic virtio
# display exposes desktop GL through Mesa instead, so the visual harness marks
# its disposable overlay and selects that path without changing the appliance.
if [ -e /var/lib/ie-qemu-visual ]; then
    export SteamEnv=1
    export LIBGL_ALWAYS_SOFTWARE=1
fi
exec >>/var/ie/state/ie-session.log 2>&1
# Persist proof that greetd executed the appliance session. The unprivileged
# appliance account cannot write /dev/console on a hardened image.
printf '%s\n' 'Intuition Engine greetd session started'
# Do not exec Cage: the wrapper records an abnormal child exit, waits a bounded
# period, then exits for greetd. That prevents overlapping sessions and a rapid
# uncontrolled restart loop when JACK or the display stack keeps failing.
set +e
cage -s -- /opt/ie/ie-launch.sh
status=$?
set -e
if [ "$status" -eq 0 ]; then exit 0; fi
sleep 2
exit "$status"
