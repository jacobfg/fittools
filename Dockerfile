# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM node:24-alpine AS ui
WORKDIR /build/web
COPY web/package.json web/package-lock.json web/.npmrc ./
RUN npm ci
COPY web/index.html web/tsconfig.json web/vite.config.ts web/device-presets.json ./
COPY web/src ./src
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine AS build
ARG TARGETOS=linux
ARG TARGETARCH
WORKDIR /build
COPY go.mod ./
COPY internal ./internal
COPY cmd ./cmd
COPY web/embed.go ./web/embed.go
COPY web/device-presets.json ./web/device-presets.json
COPY --from=ui /build/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /fittools ./cmd/fittools
RUN mkdir -p /runtime/tmp && chmod 1777 /runtime/tmp

FROM scratch
ENV FIT_UI_HOST=0.0.0.0 FIT_MAX_CONCURRENT=1
COPY --from=build /fittools /fittools
COPY --from=build /runtime/ /
COPY THIRD_PARTY_NOTICES.md /THIRD_PARTY_NOTICES.md
USER 10001:10001
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD ["/fittools", "healthcheck"]
ENTRYPOINT ["/fittools"]
CMD ["serve"]
