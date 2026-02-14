# syntax=docker/dockerfile:1

# Production Dockerfile for goreleaser (dockers_v2).
# The motus binary is pre-built by goreleaser with the frontend embedded
# via //go:embed, so no separate frontend build stage is needed.

FROM gcr.io/distroless/static-debian12:nonroot

ARG TARGETPLATFORM

LABEL org.opencontainers.image.source="https://github.com/tamcore/motus"
LABEL org.opencontainers.image.description="Motus GPS Tracking System"
LABEL org.opencontainers.image.licenses="MIT"

COPY ${TARGETPLATFORM}/motus /motus
COPY migrations /migrations

EXPOSE 8080 5013 5093

USER 65532:65532

ENTRYPOINT ["/motus"]
CMD ["serve"]
