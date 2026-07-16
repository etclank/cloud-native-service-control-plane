FROM busybox:1.37.0

ARG BUILD_DATE=unknown
ARG GIT_SHA=unknown
ARG SOURCE_URL=unknown

LABEL org.opencontainers.image.title="Registry smoke image"
LABEL org.opencontainers.image.description="GHCR and Kubernetes image-pull validation image"
LABEL org.opencontainers.image.source="${SOURCE_URL}"
LABEL org.opencontainers.image.revision="${GIT_SHA}"
LABEL org.opencontainers.image.created="${BUILD_DATE}"

RUN mkdir -p /www \
    && printf \
      '{"service":"registry-smoke","revision":"%s","built_at":"%s"}\n' \
      "${GIT_SHA}" \
      "${BUILD_DATE}" \
      > /www/index.html \
    && chmod 0444 /www/index.html

USER 65534:65534

EXPOSE 8080

ENTRYPOINT ["httpd", "-f", "-p", "8080", "-h", "/www"]
