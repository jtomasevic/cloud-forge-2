FROM golang:1.26-alpine AS builder
WORKDIR /build

# COPY go.work go.work.sum ./
COPY go.work ./
COPY libs/ libs/
COPY services/cf-accounts/ services/cf-accounts/

WORKDIR /build/services/cf-accounts
RUN GOWORK=off CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /cf-accounts .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /cf-accounts /cf-accounts
USER nonroot:nonroot
EXPOSE 8081
ENTRYPOINT ["/cf-accounts"]
