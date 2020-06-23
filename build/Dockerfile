FROM weaveworks/build-golang:1.13.5-stretch

RUN apt-get update && \
    apt-get install -y \
      sudo \
      libffi-dev awscli \
      && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# Install Terraform
RUN curl -fsSLo terraform.zip https://releases.hashicorp.com/terraform/0.11.10/terraform_0.11.10_linux_amd64.zip && \
    echo "43543a0e56e31b0952ea3623521917e060f2718ab06fe2b2d506cfaa14d54527 terraform.zip" | sha256sum -c && \
    unzip terraform.zip && \
    chmod +x terraform && \
    mv terraform /usr/bin && \
    rm -f terraform.zip

# Install Google Cloud SDK - borrowed from https://hub.docker.com/r/google/cloud-sdk/~/dockerfile/
ENV CLOUD_SDK_VERSION 206.0.0
ENV PATH /go/google-cloud-sdk/bin:$PATH
RUN curl -O https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-sdk-${CLOUD_SDK_VERSION}-linux-x86_64.tar.gz && \
    echo "d39293914b2e969bfe18dd19eb77ba96d283995f8cf1e5d7ba6ac712a3c9479a  google-cloud-sdk-${CLOUD_SDK_VERSION}-linux-x86_64.tar.gz" | sha256sum -c && \
    tar xzf google-cloud-sdk-${CLOUD_SDK_VERSION}-linux-x86_64.tar.gz && \
    rm google-cloud-sdk-${CLOUD_SDK_VERSION}-linux-x86_64.tar.gz && \
    ln -s /lib /lib64 && \
    gcloud config set core/disable_usage_reporting true && \
    gcloud config set component_manager/disable_update_check true && \
    gcloud config set metrics/environment github_docker_image && \
    gcloud --version

# Install Docker (client only)
ENV DOCKERVERSION=19.03.8
RUN curl -fsSLo docker.tgz https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKERVERSION}.tgz && \
    echo "7f4115dc6a3c19c917f8b9664d7b51c904def1c984e082c4600097433323cf6f  docker.tgz" | sha256sum -c && \
    tar xzvf docker.tgz --strip 1 -C /usr/local/bin docker/docker && \
    rm docker.tgz;

# Install golangci-lint
ENV GOLANGCILINTVERSION=1.17.1
RUN curl -fsSLo golangci-lint.tgz https://github.com/golangci/golangci-lint/releases/download/v1.17.1/golangci-lint-${GOLANGCILINTVERSION}-linux-amd64.tar.gz && \
    echo "62693d3351858206af0bfe975f7f79a8ac9124502e8de7906f377c231f37f7d3  golangci-lint.tgz" | sha256sum -c && \
    tar xzvf golangci-lint.tgz --strip 1 -C /usr/local/bin golangci-lint-${GOLANGCILINTVERSION}-linux-amd64/golangci-lint && \
    rm golangci-lint.tgz;

ARG revision
LABEL maintainer="Weaveworks <help@weave.works>" \
      org.opencontainers.image.title="golang" \
      org.opencontainers.image.source="https://github.com/weaveworks/wksctl/tree/master/build" \
      org.opencontainers.image.revision="${revision}" \
      org.opencontainers.image.vendor="Weaveworks"
