FROM alpine:3.11

RUN apk --no-cache --update add ca-certificates

COPY mock-https-authz-server /bin/server

ENTRYPOINT [ "/bin/server" ]

ARG revision
LABEL org.opencontainers.image.title="mock-https-authz-server" \
      org.opencontainers.image.source="https://github.com/weaveworks/wksctl" \
      org.opencontainers.image.revision="${revision}"
