FROM quay.io/prometheus/busybox:latest

LABEL org.opencontainers.image.authors="Alexander Trost <galexrt@googlemail.com>" \
    org.opencontainers.image.created="${BUILD_DATE}" \
    org.opencontainers.image.title="galexrt/alertmanager-githubfiles-receiver" \
    org.opencontainers.image.description="An Alertmanager webhook receiver which creates files in a GitHub repository based on the received alert(s)." \
    org.opencontainers.image.documentation="https://github.com/galexrt/alertmanager-githubfiles-receiver/blob/main/README.md" \
    org.opencontainers.image.url="https://github.com/galexrt/alertmanager-githubfiles-receiver" \
    org.opencontainers.image.source="https://github.com/galexrt/alertmanager-githubfiles-receiver" \
    org.opencontainers.image.revision="${REVISION}" \
    org.opencontainers.image.vendor="galexrt" \
    org.opencontainers.image.version="N/A"

ADD .build/linux-amd64/alertmanager-githubfiles-receiver /bin/alertmanager-githubfiles-receiver

ENTRYPOINT ["/bin/alertmanager-githubfiles-receiver"]
