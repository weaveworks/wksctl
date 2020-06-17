FROM quay.io/footloose/centos7:0.6.3

RUN yum -y install sudo kubernetes-client

# Needed for TestService
RUN yum -y install httpd

# Fetch gosu
ENV GOSU_VERSION 1.11
ENV ARCH amd64
RUN curl -fsSLo /usr/local/bin/gosu "https://github.com/tianon/gosu/releases/download/$GOSU_VERSION/gosu-$ARCH" \
    && chmod +x /usr/local/bin/gosu

# Install golang so we can run our tests on macs
ENV GO_VERSION 1.12
RUN curl -fsSLo /tmp/gotar.tgz "https://dl.google.com/go/go$GO_VERSION.linux-$ARCH.tar.gz" \
    && tar -C /usr/local -xzf /tmp/gotar.tgz && rm /tmp/gotar.tgz

ENV SRC_PATH /go/src/github.com/weaveworks/wksctl
WORKDIR $SRC_PATH
CMD ["/bin/bash"]
