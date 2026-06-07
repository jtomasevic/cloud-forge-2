FROM golang:1.26-alpine AS builder
WORKDIR /build

COPY go.work go.work.sum ./
COPY libs/ libs/
COPY services/cf-router/ services/cf-router/

WORKDIR /build/services/cf-router
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /cf-router .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /cf-router /cf-router
COPY api/ /api/
USER nonroot:nonroot
EXPOSE 8083
EXPOSE 8090
ENTRYPOINT ["/cf-router"]
