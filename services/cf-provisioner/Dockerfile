FROM golang:1.26-alpine AS builder
WORKDIR /build

# COPY go.work go.work.sum ./
COPY go.work ./
COPY libs/ libs/
COPY services/cf-provisioner/ services/cf-provisioner/

WORKDIR /build/services/cf-provisioner
RUN GOWORK=off CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /cf-provisioner .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /cf-provisioner /cf-provisioner
USER nonroot:nonroot
EXPOSE 8082
ENTRYPOINT ["/cf-provisioner"]
