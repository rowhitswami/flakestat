# Consumed by GoReleaser, which places the prebuilt static binaries in a
# temporary build context laid out by platform:
#
#   context/linux/amd64/flakestat
#   context/linux/arm64/flakestat
#
# There is no compile step here on purpose: the binary is CGO-free, so the
# image is just a filesystem around it.
FROM alpine:3.21

# Populated automatically by buildx; selects the right prebuilt binary so one
# Dockerfile serves every architecture.
ARG TARGETPLATFORM

# ca-certificates for any HTTPS use; git because flakestat reads the commit SHA
# to tell genuine flakiness (same code, different result) from a regression that
# was later fixed. Without git the tool still runs, just with weaker scoring.
RUN apk add --no-cache ca-certificates git \
    # Mounted repos are owned by a different UID than the container user, and
    # git refuses to read them without this. Every CI use mounts a repo.
    && git config --global --add safe.directory '*'

COPY $TARGETPLATFORM/flakestat /usr/local/bin/flakestat

WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/flakestat"]
CMD ["--help"]
