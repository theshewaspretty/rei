# syntax=docker/dockerfile:1

# ---- 빌드: 서버 + 배포용 에이전트(Linux amd64/arm64, Windows amd64) ----
FROM golang:1.27-alpine AS build
WORKDIR /src

# 사내망에서 TLS 검사 장비가 인증서를 바꿔 끼우면 모듈을 받을 때 검증에 실패한다.
# 그 장비의 루트 CA(PEM, 확장자 .crt)를 docker/certs/ 에 넣으면 빌드 단계에서만 신뢰 목록에 더한다.
COPY docker/certs/ /tmp/extra-certs/
RUN for c in /tmp/extra-certs/*.crt; do [ -f "$c" ] && cat "$c" >> /etc/ssl/certs/ca-certificates.crt; done; true

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY web ./web

ARG VERSION=dev
ENV CGO_ENABLED=0
RUN set -e; \
    LDFLAGS="-s -w -X infrascanner/internal/model.Version=${VERSION}"; \
    go build -trimpath -ldflags "$LDFLAGS" -o /out/infra-server ./cmd/server; \
    for target in linux/amd64 linux/arm64 windows/amd64; do \
      os="${target%/*}"; arch="${target#*/}"; ext=""; \
      if [ "$os" = "windows" ]; then ext=".exe"; fi; \
      GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "$LDFLAGS" -o "/out/downloads/infra-agent-$os-$arch$ext" ./cmd/agent; \
    done; \
    mkdir -p /out/data

# ---- 실행: 셸 없는 최소 이미지, 일반 사용자 권한 ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/infra-server /app/infra-server
COPY --from=build /out/downloads /app/downloads
# 이름 있는 볼륨은 처음 만들어질 때 이 폴더의 소유자를 물려받는다 (nonroot 가 쓸 수 있어야 한다)
COPY --from=build --chown=nonroot:nonroot /out/data /data

ENV LISTEN_ADDR=:8443 \
    DATA_DIR=/data \
    DOWNLOADS_DIR=/app/downloads
EXPOSE 8443
VOLUME /data
USER nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/app/infra-server", "healthcheck"]
ENTRYPOINT ["/app/infra-server"]
