FROM alpine:3.11

RUN apk --no-cache --update add ca-certificates

COPY controller /bin/

ENTRYPOINT [ "/bin/controller" ]

ARG revision
LABEL org.opencontainers.image.title="controller" \
    org.opencontainers.image.description="The WKSCTL controller is responsible for ensuring that the Kubernetes cluster it is deployed on always converges towards its description." \
    org.opencontainers.image.revision="${revision}" \
    org.opencontainers.image.url="https://github.com/weaveworks/wksctl" \
    org.opencontainers.image.source="git@github.com:weaveworks/wksctl.git" \
    org.opencontainers.image.vendor="Weaveworks"
