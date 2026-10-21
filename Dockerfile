FROM ubuntu:24.04 AS bundle
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates xz-utils && rm -rf /var/lib/apt/lists/*
WORKDIR /source
COPY . .
RUN test "$TARGETARCH" = amd64 -o "$TARGETARCH" = arm64 && bash scripts/verify-runtime.sh "$TARGETARCH" && mkdir /bundle && tar --no-same-owner -xJf "runtime/linux-$TARGETARCH.tar.xz" -C /bundle && mv "/bundle/bin/linux-$TARGETARCH" /bundle/bin/native

FROM ubuntu:24.04 AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl iproute2 jq tzdata && rm -rf /var/lib/apt/lists/* && mkdir -p /var/lib/veloray-agent /run/veloray-proxy && chown 10003:10001 /run/veloray-proxy && chmod 2770 /run/veloray-proxy && chown 10002:10002 /var/lib/veloray-agent && chmod 0700 /var/lib/veloray-agent
COPY --from=bundle /bundle/bin/native/veloray /bundle/bin/native/veloray-agent /usr/local/bin/
COPY --from=bundle /bundle/xray/xray /bundle/singbox/sing-box /usr/local/bin/
COPY --from=bundle /bundle/xray /opt/veloray/xray
COPY --from=bundle /bundle/singbox /opt/veloray/singbox
COPY --from=bundle /bundle/licenses /usr/share/licenses/veloray
COPY --from=bundle /bundle/frontend/dist /opt/veloray/frontend
COPY deploy/docker/entrypoint.sh /usr/local/bin/veloray-container
ENV VELORAY_FRONTEND_DIR=/opt/veloray/frontend XRAY_LOCATION_ASSET=/opt/veloray/xray VELORAY_TEMPLATE_ROOT=/etc/veloray/templates
USER 10001:10001
ENTRYPOINT ["/bin/bash", "/usr/local/bin/veloray-container"]
CMD ["panel"]

FROM caddy:2.11.7-alpine AS proxy
USER root
RUN mkdir -p /data /config /run/veloray-proxy && chown -R 10003:10001 /data /config /run/veloray-proxy && chmod 2770 /run/veloray-proxy
COPY --from=bundle /bundle/bin/native/veloray /usr/local/bin/veloray
RUN setcap cap_net_bind_service=+ep /usr/local/bin/veloray
USER 10003:10001
ENTRYPOINT ["/usr/local/bin/veloray", "proxy-serve"]
CMD []
