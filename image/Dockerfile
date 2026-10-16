# syntax=docker/dockerfile:1
# Moltis 20260913.02, multi-platform OCI index from ghcr.io/moltis-org/moltis.
FROM ghcr.io/moltis-org/moltis@sha256:01fdd3e8b824d6eb6856692941346a0fcd33d5e2b82a8690a554726bd1a7bc81
USER root
# Keep runtime utilities eligible for security updates at rebuild time.
# Do not retain upstream's passwordless privilege escalation.
RUN rm -f /etc/sudoers.d/moltis \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
        bash ffmpeg git gosu jq libnss-wrapper openssh-client ripgrep tini \
    && SUDO_FORCE_REMOVE=yes apt-get purge -y sudo \
    && rm -rf /var/lib/apt/lists/*
ENV HOME=/home/moltis \
    USER=moltis \
    LOGNAME=moltis \
    XDG_CONFIG_HOME=/home/moltis/.config \
    XDG_CACHE_HOME=/home/moltis/.cache \
    TMPDIR=/home/moltis/.cache/tmp \
    MOLTIS_BEHIND_PROXY=true
COPY --chmod=0755 entrypoint.sh /usr/local/bin/moltis-entrypoint
WORKDIR /home/moltis
EXPOSE 9000
# The runtime supplies the non-root UID. Root is only a bind-mount initializer
# for explicit native image use, and drops privileges before starting Moltis.
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/moltis-entrypoint"]
CMD []
