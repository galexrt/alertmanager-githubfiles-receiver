FROM quay.io/prometheus/busybox:latest
LABEL maintainer="Alexander Trost <galexrt@googlemail.com>"

ADD .build/linux-amd64/alertmanager-githubfiles-receiver /bin/alertmanager-githubfiles-receiver

ENTRYPOINT ["/bin/alertmanager-githubfiles-receiver"]
