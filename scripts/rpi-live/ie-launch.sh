#!/bin/sh
set -eu

export IE_LIVE_IMAGE=1
export IE_AUDIO_BACKEND=jack
export IE_JACK_ALSA_DEVICE="${IE_JACK_ALSA_DEVICE:-hw:${IE_JACK_ALSA_CARD:-0}}"
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
mkdir -p "$XDG_RUNTIME_DIR" /var/ie/state /var/ie/runtime
chmod 700 "$XDG_RUNTIME_DIR"
export ALSA_CONFIG_PATH=/var/ie/runtime/asound.conf
cat >"$ALSA_CONFIG_PATH" <<EOF
pcm.!default { type hw; card ${IE_JACK_ALSA_CARD:-0}; }
ctl.!default { type hw; card ${IE_JACK_ALSA_CARD:-0}; }
EOF
chmod 0600 "$ALSA_CONFIG_PATH"

cd /var/ie/share 2>/dev/null || cd /opt/ie
set -- -ehbasic-host -ehbasic-host-appliance
for system in EmuTOS AROS IntuitionOS; do
    [ -d "/var/ie/share/Systems/$system" ] || continue
    case "$system" in
        EmuTOS) set -- "$@" -emutos-drive "/var/ie/share/Systems/$system" ;;
        AROS) set -- "$@" -aros-drive "/var/ie/share/Systems/$system" ;;
        IntuitionOS) set -- "$@" -intuitionos-root "/var/ie/share/Systems/$system" ;;
    esac
done
if [ -f /var/ie/share/Systems/IntuitionOS/Boot/iexec.ie64 ]; then
    set -- "$@" -intuitionos-image /var/ie/share/Systems/IntuitionOS/Boot/iexec.ie64
fi
set +e
/opt/ie/IntuitionEngine "$@" >>/var/ie/state/intuition-engine.log 2>&1
status=$?
set -e
printf 'IntuitionEngine exited with status %s\n' "$status" >>/var/ie/state/intuition-engine.log
exit "$status"
