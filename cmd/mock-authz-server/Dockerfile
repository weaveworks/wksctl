FROM alpine:3.11

RUN apk --no-cache --update add ca-certificates

COPY server /bin/

ENTRYPOINT [ "/bin/server" ]

ARG revision
LABEL org.opencontainers.image.title="mock-authz-server" \
      org.opencontainers.image.source="https://github.com/weaveworks/wksctl" \
      org.opencontainers.image.revision="${revision}"
