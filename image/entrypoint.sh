#!/bin/sh
set -eu

if [ "${HOME:-}" != /home/moltis ]; then
    echo 'HOME must be /home/moltis. Mount the instance directory there.' >&2
    exit 1
fi
if [ "$(id -u)" = 0 ]; then
    if [ -L "$HOME" ]; then
        echo 'The mounted home must not be a symbolic link.' >&2
        exit 1
    fi
    mkdir -p "$HOME"
    chown --no-dereference moltis:moltis "$HOME"
    exec gosu moltis /usr/local/bin/moltis-entrypoint "$@"
fi
if [ ! -w "$HOME" ]; then
    echo 'The mounted home must be writable by the container user.' >&2
    exit 1
fi
umask 077
mkdir -p "$HOME/.config/moltis" "$HOME/.moltis" "$TMPDIR"
cd "$HOME"

# Apple VirtioFS uses the runtime process UID for its mount mapping. Run as the
# Mac owner, and supply a libc user record when that UID is absent in the image.
# No root access or ownership change on the host home is needed.
if ! getent passwd "$(id -u)" >/dev/null; then
    passwd_file="$TMPDIR/passwd"
    group_file="$TMPDIR/group"
    awk -F: -v uid="$(id -u)" -v gid="$(id -g)" '
        BEGIN { OFS=":" }
        $1 == "moltis" { $3=uid; $4=gid }
        { print }
    ' /etc/passwd > "$passwd_file"
    cp /etc/group "$group_file"
    if ! getent group "$(id -g)" >/dev/null; then
        printf 'moltis-host:x:%s:\n' "$(id -g)" >> "$group_file"
    fi
    for wrapper in /usr/lib/*/libnss_wrapper.so; do
        if [ -f "$wrapper" ]; then
            export NSS_WRAPPER_PASSWD="$passwd_file" NSS_WRAPPER_GROUP="$group_file"
            export LD_PRELOAD="$wrapper${LD_PRELOAD:+:$LD_PRELOAD}"
            break
        fi
    done
    if ! getent passwd "$(id -u)" >/dev/null; then
        echo 'Cannot initialize the container user identity.' >&2
        exit 1
    fi
fi

# Do not expose a host runtime socket. Tool commands stay in this instance.
config="$HOME/.config/moltis/moltis.toml"
if [ ! -e "$config" ]; then
    # Noclobber prevents a second initializer from replacing user configuration.
    (set -C; cat > "$config" <<'EOF'
[tools.exec.sandbox]
backend = "restricted-host"
EOF
    )
fi

# Permit native diagnostics using the same identity and home initialization.
if [ "$#" -gt 0 ]; then
    exec "$@"
fi
exec moltis --config-dir "$HOME/.config/moltis" --data-dir "$HOME/.moltis" \
    --bind 0.0.0.0 --port 9000 --no-tls
