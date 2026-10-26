# Build on the runner's native platform and cross-compile, instead of emulating arm64.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /audiolibrix-abs .

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /audiolibrix-abs /audiolibrix-abs
USER 65534:65534
EXPOSE 3001
ENTRYPOINT ["/audiolibrix-abs"]
