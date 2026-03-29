# MiniDevOpsHub Control Plane Dockerfile
FROM golang:1.21-alpine as builder
WORKDIR /app
COPY . .
RUN go build -o minidevopshub ./cmd/controlplane

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/minidevopshub .
EXPOSE 8080
CMD ["./minidevopshub"]
