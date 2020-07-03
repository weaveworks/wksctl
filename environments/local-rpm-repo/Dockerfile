FROM centos:7
RUN yum install -y epel-release && \
    yum install -y \
    createrepo \
    nginx \
    && \
    yum clean all

RUN mkdir -p /var/www/html/repos/{base,centosplus,extras,updates}
COPY docker-ce.repo /etc/yum.repos.d/docker-ce.repo
COPY kubernetes.repo /etc/yum.repos.d/kubernetes.repo
RUN yum --downloadonly --downloaddir /var/www/html/repos install -y --disableexcludes=kubernetes \
    device-mapper-persistent-data \
    docker-ce-19.03.8 \
    kubeadm-1.16.11 \
    kubectl-1.16.11 \
    kubelet-1.16.11 \
    lvm2 \
    yum-utils \
    yum-versionlock
RUN createrepo /var/www/html/repos

COPY nginx.conf /etc/nginx/nginx.conf
EXPOSE 80
CMD ["nginx"]
