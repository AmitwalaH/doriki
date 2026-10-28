# The doriki image on ghcr.io (DESIGN.md section 10.1), built by GoReleaser's dockers_v2 from the
# binary it already compiled; nothing is compiled here. distroless static gives CA roots, tzdata
# and a non-root user, and nothing else. Pinned by the multi-arch index digest; bump it by hand.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/doriki /usr/bin/doriki
ENTRYPOINT ["/usr/bin/doriki"]
